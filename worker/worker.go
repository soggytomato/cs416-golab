/*

Port is dynamic

Usage:

$ go run worker.go [loadbalancer ip:port]

*/
package main

import (
	// "bufio"
	// "bytes"
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
	"strconv"
	// "strings"
	"time"

	"github.com/gorilla/websocket"
)

type WorkerNetSettings struct {
	WorkerID                int `json:"workerID"`
	HeartBeat               int `json:"heartbeat"`
	MinNumWorkerConnections int `json:"min-num-worker-connections"`
}

type WorkerInfo struct {
	RPCAddress  net.Addr
	HTTPAddress net.Addr
}

type Worker struct {
	workerID         int
	loadBalancerConn *rpc.Client
	settings         *WorkerNetSettings
	serverAddr       string
	localRPCAddr     net.Addr
	localHTTPAddr    net.Addr
	externalIP       string
	clients          map[string]*websocket.Conn
	workers          map[string]*rpc.Client
	logger           *log.Logger
	crdt             map[string]*Operation
	localOps         []Operation
	nextOpNumber     int
	crdtFirstID      string
}

type WorkerResponse struct {
	Error   error
	Payload []interface{}
}

type WorkerRequest struct {
	Payload []interface{}
}

type OpType int

const (
	INSERT OpType = iota
	DELETE
)

type Operation struct {
	ClientID string
	Type     OpType
	ID       string
	PrevID   string
	NextID   string
	Text     string
}

type browserMsg struct {
	SessionID  string
	Username   string
	Command    string
	Operations string
	Payload    string
}

// Used to send heartbeat to the server just shy of 1 second each beat
const TIME_BUFFER int = 500

// Since we are adding a character to the right of another character, we need
// a fake INITIAL_ID to use to place the first character in an empty message
const INITIAL_ID string = "12345"

func main() {
	gob.Register(map[string]*Operation{})
	gob.Register(&net.TCPAddr{})
	gob.Register([]Operation{})
	gob.Register(&Operation{})
	worker := new(Worker)
	worker.logger = log.New(os.Stdout, "[Initializing] ", log.Lshortfile)
	worker.init()
	worker.listenRPC()
	worker.listenHTTP()
	worker.registerWithLB()
	worker.getWorkers()
	worker.getCRDT()
	go worker.sendLocalOps()
	for {

	}
}

func (w *Worker) init() {
	args := os.Args[1:]
	w.serverAddr = args[0]
	w.workers = make(map[string]*rpc.Client)
	w.crdt = make(map[string]*Operation)
	w.nextOpNumber = 1
	w.clients = make(map[string]*websocket.Conn)
}

func (w *Worker) listenRPC() {
	addrs, _ := net.InterfaceAddrs()
	var externalIP string
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				externalIP = ipnet.IP.String()
			}
		}
	}
	externalIP = externalIP + ":0"
	tcpAddr, err := net.ResolveTCPAddr("tcp", externalIP)
	checkError(err)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError(err)
	rpc.Register(w)
	w.localRPCAddr = listener.Addr()
	rpc.Register(w)
	w.externalIP = externalIP
	w.logger.Println("listening for RPC on: ", listener.Addr().String())
	go func() {
		for {
			conn, _ := listener.Accept()
			go rpc.ServeConn(conn)
		}
	}()
}

func (w *Worker) listenHTTP() {
	http.HandleFunc("/ws", w.wsHandler)
	httpAddr, err := net.ResolveTCPAddr("tcp", w.externalIP)
	checkError(err)
	listener, err := net.ListenTCP("tcp", httpAddr)
	checkError(err)
	w.localHTTPAddr = listener.Addr()
	go http.Serve(listener, nil)
	w.logger.Println("listening for HTTP on: ", listener.Addr().String())
}

func (w *Worker) registerWithLB() {
	loadBalancerConn, err := rpc.Dial("tcp", w.serverAddr)
	checkError(err)
	settings := new(WorkerNetSettings)
	err = loadBalancerConn.Call("LBServer.RegisterNewWorker", &WorkerInfo{w.localRPCAddr, w.localHTTPAddr}, settings)
	checkError(err)
	w.settings = settings
	w.workerID = settings.WorkerID
	go w.startHeartBeat()
	w.logger.SetPrefix("[Worker: " + strconv.Itoa(w.workerID) + "] ")
	w.loadBalancerConn = loadBalancerConn
}

func (w *Worker) startHeartBeat() {
	var ignored bool
	w.loadBalancerConn.Call("LBServer.HeartBeat", w.workerID, &ignored)
	for {
		time.Sleep(time.Duration(w.settings.HeartBeat-TIME_BUFFER) * time.Millisecond)
		w.loadBalancerConn.Call("LBServer.HeartBeat", w.workerID, &ignored)
	}
}

// Gets miners from server if below MinNumMinerConnections
func (w *Worker) getWorkers() {
	var addrSet []net.Addr
	for workerAddr, workerCon := range w.workers {
		isConnected := false
		workerCon.Call("Worker.PingWorker", "", &isConnected)
		if !isConnected {
			delete(w.workers, workerAddr)
		}
	}
	if len(w.workers) < int(w.settings.MinNumWorkerConnections) {
		w.loadBalancerConn.Call("LBServer.GetNodes", w.workerID, &addrSet)
		w.connectToWorkers(addrSet)
	}
}

func (w *Worker) getCRDT() {
	response := new(WorkerResponse)
	for _, workerCon := range w.workers {
		err := workerCon.Call("Worker.SendCRDT", "", response)
		if err != nil {
			fmt.Println(err)
		} else {
			w.crdt = response.Payload[0].(map[string]*Operation)
			w.crdtFirstID = response.Payload[1].(string)
			return
		}
	}
}

func (w *Worker) SendCRDT(payload string, response *WorkerResponse) error {
	response.Payload = make([]interface{}, 2)
	response.Payload[0] = w.crdt
	response.Payload[1] = w.crdtFirstID
	return nil
}

//****POC CODE***//

// func (w *Worker) workerPrompt() {
// 	reader := bufio.NewReader(os.Stdin)
// 	for {
// 		message := w.getMessage()
// 		fmt.Println("Message:", message)
// 		fmt.Print("Worker> ")
// 		cmd, _ := reader.ReadString('\n')
// 		if w.handleCommand(cmd) == 1 {
// 			return
// 		}
// 	}
// }
//
// // Iterate through the beginning of the CRDT to the end to show the message and
// // specify the mapping of each character
// func (w *Worker) getMessage() string {
// 	var buffer bytes.Buffer
// 	firstOp := w.crdt[w.crdtFirstID]
// 	for firstOp != nil {
// 		fmt.Println(firstOp.ID, "->", firstOp.Text)
// 		buffer.WriteString(firstOp.Text)
// 		firstOp = w.crdt[firstOp.NextID]
// 	}
// 	return buffer.String()
// }
//
// func (w *Worker) handleCommand(cmd string) int {
// 	args := strings.Split(strings.TrimSpace(cmd), ",")
//
// 	switch args[0] {
// 	case "addRight":
// 		err := w.addRight(args[1], args[2])
// 		if checkError(err) != nil {
// 			return 0
// 		}
// 	case "refresh":
// 		return 0
// 	default:
// 		fmt.Println(" Invalid command.")
// 	}
//
// 	return 0
// }

// Adds a character to the right of the prevID specified in the args
func (w *Worker) addRight(prevID, content string) error {
	if !w.prevIDExists(prevID) {
		return nil
	}
	opID := strconv.Itoa(w.nextOpNumber) + strconv.Itoa(w.workerID)
	newOperation := &Operation{strconv.Itoa(w.workerID), INSERT, opID, prevID, "", content}
	w.addToCRDT(newOperation)
	return nil
}

func (w *Worker) addToCRDT(newOperation *Operation) error {
	if w.firstCRDTEntry(newOperation.ID) {
		w.addOpAndIncrementCounter(newOperation, newOperation.ID)
		return nil
	}
	if w.replacingFirstOp(newOperation, newOperation.PrevID, newOperation.ID) {
		w.addOpAndIncrementCounter(newOperation, newOperation.ID)
		return nil
	}
	w.normalInsert(newOperation, newOperation.PrevID, newOperation.ID)
	w.addOpAndIncrementCounter(newOperation, newOperation.ID)
	return nil
}

// Check if the prevID actually exists; if true, continue with addRight
func (w *Worker) prevIDExists(prevID string) bool {
	if _, ok := w.crdt[prevID]; ok || prevID == INITIAL_ID {
		return true
	} else {
		return false
	}
}

// The case where the first content is entered into a CRDT
func (w *Worker) firstCRDTEntry(opID string) bool {
	if len(w.crdt) <= 0 {
		w.crdtFirstID = opID
		return true
	} else {
		return false
	}
}

// If your character is placed at the beginning of the message, it needs to become
// the new firstOp so we can iterate through the CRDT properly
func (w *Worker) replacingFirstOp(newOperation *Operation, prevID, opID string) bool {
	if prevID == INITIAL_ID {
		firstOp := w.crdt[w.crdtFirstID]
		newOperation.NextID = w.crdtFirstID
		firstOp.PrevID = opID
		w.crdtFirstID = opID
		return true
	} else {
		return false
	}
}

// Any other insert that doesn't take place at the beginning or end is handled here
func (w *Worker) normalInsert(newOperation *Operation, prevID, opID string) {
	newPrevID := w.samePlaceInsertCheck(newOperation, prevID, opID)
	prevOp := w.crdt[newPrevID]
	newOperation.NextID = prevOp.NextID
	prevOp.NextID = opID
}

// Checks if any other clients have made inserts to the same prevID. The algorithm
// compares the prevOp's nextID to the incomingOp ID - if nextID is greater, incomingOp
// will move further down the message until it is greater than the nextID
func (w *Worker) samePlaceInsertCheck(newOperation *Operation, prevID, opID string) string {
	var nextOpID int
	prevOp := w.crdt[prevID]
	if prevOp.NextID != "" {
		nextOpID, _ = strconv.Atoi(prevOp.NextID)
		newOpID, _ := strconv.Atoi(opID)
		for nextOpID >= newOpID && newOperation.ClientID != w.crdt[prevOp.NextID].ClientID {
			prevOp = w.crdt[strconv.Itoa(nextOpID)]
			nextOpID, _ = strconv.Atoi(prevOp.NextID)
		}
		return prevOp.ID
	} else {
		return prevID
	}

}

// Once all the CRDT pointers are updated, the op can be added to the CRDT and the op
// number can be incremented
func (w *Worker) addOpAndIncrementCounter(newOperation *Operation, opID string) {
	deepCopyOp := &Operation{newOperation.ClientID, newOperation.Type, newOperation.ID, newOperation.PrevID, newOperation.NextID, newOperation.Text}
	w.crdt[opID] = deepCopyOp
	w.localOps = append(w.localOps, *deepCopyOp)
	w.nextOpNumber++
}

// Establishes RPC connections with workers in addrs array
func (w *Worker) connectToWorkers(addrs []net.Addr) {
	for _, workerAddr := range addrs {
		if w.workers[workerAddr.String()] == nil {
			workerCon, err := rpc.Dial("tcp", workerAddr.String())
			if err != nil {
				w.logger.Println(err)
				delete(w.workers, workerAddr.String())
			} else {
				w.workers[workerAddr.String()] = workerCon
				response := new(WorkerResponse)
				request := new(WorkerRequest)
				request.Payload = make([]interface{}, 1)
				request.Payload[0] = w.localRPCAddr.String()
				workerCon.Call("Worker.BidirectionalSetup", request, response)
			}
		}
	}
}

func (w *Worker) sendLocalOps() error {
	for {
		time.Sleep(time.Second * 10)
		// w.getWorkers() // checks all workers, connects to more if needed
		request := new(WorkerRequest)
		request.Payload = make([]interface{}, 1)
		request.Payload[0] = w.localOps
		response := new(WorkerResponse)
		for workerAddr, workerCon := range w.workers {
			isConnected := false
			workerCon.Call("Worker.PingWorker", "", &isConnected)
			if isConnected {
				workerCon.Call("Worker.ApplyIncomingOps", request, response)
			} else {
				delete(w.workers, workerAddr)
			}
		}
		w.localOps = nil
	}
	return nil
}

func (w *Worker) ApplyIncomingOps(request *WorkerRequest, response *WorkerResponse) error {
	incomingOps := request.Payload[0].([]Operation)
	for _, op := range incomingOps {
		if w.crdt[op.ID] == nil {
			w.addToCRDT(&op)
		}
	}
	return nil
}

func (w *Worker) BidirectionalSetup(request *WorkerRequest, response *WorkerResponse) error {
	workerAddr := request.Payload[0].(string)
	workerConn, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		delete(w.workers, workerAddr)
	} else {
		w.workers[workerAddr] = workerConn
	}
	return nil
}

// Pings all workers currently listed in the worker map
// If a connected worker fails to reply, that worker should be removed from the map
func (w *Worker) PingWorker(payload string, reply *bool) error {
	*reply = true
	return nil
}

// HTTP point to bootstrap websocket connection between client and worker
// Client should send their userID in a Get Request URL Parameter
// After establishing connection, worker will add the connection to worker.clients to write messages to later
// w.reader is called in a go routine to always listen for messages from the client
// Assumption:
//			- UserID is unique, if another client with the same userID connects, their connection will override the older one.
func (w *Worker) wsHandler(wr http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Upgrade(wr, r, wr.Header(), 1024, 1024)
	if err != nil {
		http.Error(wr, "Could not open websocket connection", http.StatusBadRequest)
	}
	userID, _ := r.URL.Query()["userID"]
	if len(userID) == 0 {
		http.Error(wr, "Missing userID in URL parameter", http.StatusBadRequest)
	}
	w.logger.Println("New socket connection from: ", userID)
	w.clients[userID[0]] = conn
	go w.reader(conn, userID[0])
}

// Read function to always listen for messages from the browser
// If read fails, the websocket will be closed.
// Different commands should be handled here.
func (w *Worker) reader(conn *websocket.Conn, userID string) {
	for {
		m := browserMsg{}
		err := conn.ReadJSON(&m)
		if err != nil {
			w.logger.Println("Error reading from websocket: ", err)
			delete(w.clients, userID)
			return
		}
		w.logger.Println("Got message from "+userID+": ", m)

		// Handle different commands here
		if m.Command == "GetSessCRDT" {
			w.getSessCRDT(m)
		} else if m.Command == "RunJob" {
			w.runJob(m)
		}
	}
}

// Write function, it is only called when the worker needs to write to worker
// If a write fails, the websocket will be closed.
// Assumes an already constructed msg when called as an argument.
func (w *Worker) writer(msg browserMsg) {
	// Write to Socket
	conn := w.clients[msg.Username]
	err := conn.WriteJSON(msg)
	if err != nil {
		w.logger.Println("Error writing to websocket: ", err)
		delete(w.clients, msg.Username)
		return
	}
}

// Handles when a client issues a run command for a session and snippet
func (w *Worker) handleRun(msg browserMsg) {
	// TODO Steps:
	//		- Save to file system for jobID
	//		- call Load Balancer with jobID
	//		- return to client with jobID
	var jobID int
	var ignored bool
	w.loadBalancerConn.Call("LBServer.newJob", jobID, &ignored)
	msg.Payload = strconv.Itoa(jobID)
	w.writer(msg)
}

// Runs a job called by the load balancer
func (w *Worker) runJob(request *WorkerRequest, response *WorkerResponse) error {
	// TODO Steps:
	//		- Gets log from File System
	// 		- saves and compiles the file locally
	//		- Runs the job
	//		- saves the log to File system
	//		- Acks back to Load Balancer that it is done

	var fsResponse string
	t := time.Now()
	fileName := "runSnippet_" + t.Format("20060102150405") + ".go"
	err := ioutil.WriteFile(fileName, []byte(fsResponse), 0777)
	checkError(err)

	cmd := exec.Command("go", "run", fileName)
	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr
	err = cmd.Run()
	checkError(err)

	// Write the proper output to the log file
	if len(stderr.String()) == 0 {
		// No errors case
	} else {
		// There was a compile or runtime error
	}
	return nil
}

// Gets the Session CRDT from File System to send to client
// Constructs the msg and calls w.writer(msg) to write to client
func (w *Worker) getSessCRDT(msg browserMsg) {
	// TODO:
	//	File System RPC Call to get CRDT
	msg.Payload = "This is suppose to be the CRDT"
	w.writer(msg)
}

func checkError(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}
