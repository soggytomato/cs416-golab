/*

Port is dynamic

Usage:

$ go run worker.go [loadbalancer ip:port]

*/
package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"time"
)

type WorkerNetSettings struct {
	WorkerID                int `json:"workerID"`
	HeartBeat               int `json:"heartbeat"`
	MinNumWorkerConnections int `json:"min-num-worker-connections"`
}

type WorkerInfo struct {
	Address net.Addr
}

type Worker struct {
	workerID         int
	loadBalancerConn *rpc.Client
	settings         *WorkerNetSettings
	serverAddr       string
	localRPCAddr     net.Addr
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

// Used to send heartbeat to the server just shy of 1 second each beat
const TIME_BUFFER int = 500
// Since we are adding a character to the right of another character, we need
// a fake INITIAL_ID to use to place the first character in an empty message
const INITIAL_ID string = "12345"

func main() {
	gob.Register(&net.TCPAddr{})
	gob.Register([]Operation{})
	gob.Register(&Operation{})
	worker := new(Worker)
	worker.logger = log.New(os.Stdout, "[Initializing] ", log.Lshortfile)
	worker.init()
	worker.listenRPC()
	worker.registerWithLB()
	worker.getWorkers()
	go worker.sendLocalOps()
	worker.workerPrompt()
}

func (w *Worker) init() {
	args := os.Args[1:]
	w.serverAddr = args[0]
	w.workers = make(map[string]*rpc.Client)
	w.crdt = make(map[string]*Operation)
	w.nextOpNumber = 1
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
	w.logger.Println("listening on: ", listener.Addr().String())
	go func() {
		for {
			conn, _ := listener.Accept()
			go rpc.ServeConn(conn)
		}
	}()
}

func (w *Worker) registerWithLB() {
	loadBalancerConn, err := rpc.Dial("tcp", w.serverAddr)
	checkError(err)
	settings := new(WorkerNetSettings)
	err = loadBalancerConn.Call("LBServer.RegisterNewWorker", &WorkerInfo{w.localRPCAddr}, settings)
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

func (w *Worker) workerPrompt() {
	reader := bufio.NewReader(os.Stdin)
	for {
		message := w.getMessage()
		fmt.Println("Message:", message)
		fmt.Print("Worker> ")
		cmd, _ := reader.ReadString('\n')
		if w.handleCommand(cmd) == 1 {
			return
		}
	}
}

// Iterate through the beginning of the CRDT to the end to show the message and
// specify the mapping of each character
func (w *Worker) getMessage() string {
	var buffer bytes.Buffer
	firstOp := w.crdt[w.crdtFirstID]
	for firstOp != nil {
		fmt.Println(firstOp.ID, "->", firstOp.Text)
		buffer.WriteString(firstOp.Text)
		firstOp = w.crdt[firstOp.NextID]
	}
	return buffer.String()
}

func (w *Worker) handleCommand(cmd string) int {
	args := strings.Split(strings.TrimSpace(cmd), ",")

	switch args[0] {
	case "addRight":
		err := w.addRight(args[1], args[2])
		if checkError(err) != nil {
			return 0
		}
	case "refresh":
		return 0
	default:
		fmt.Println(" Invalid command.")
	}

	return 0
}

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

func checkError(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}
