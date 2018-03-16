/*

Port is dynamic

Usage:

$ go run worker.go [loadbalancer ip:port]

*/
package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"strconv"
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
}

type WorkerResponse struct {
	Error   error
	Payload []interface{}
}

type WorkerRequest struct {
	Payload []interface{}
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

func main() {
	gob.Register(&net.TCPAddr{})
	worker := new(Worker)
	worker.logger = log.New(os.Stdout, "[Initializing] ", log.Lshortfile)
	worker.init()
	worker.listenRPC()
	worker.listenHTTP()
	worker.registerWithLB()
	worker.getWorkers()
	for {

	}
}

func (w *Worker) init() {
	args := os.Args[1:]
	w.serverAddr = args[0]
	w.workers = make(map[string]*rpc.Client)
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
	w.localRPCAddr = listener.Addr()
	w.externalIP = externalIP
	w.logger.Println("listening for RPC on: ", listener.Addr().String())
	go func() {
		for {
			conn, _ := listener.Accept()
			w.logger.Println("New connection!")
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
		workerCon.Call("Worker.PingMiner", "", &isConnected)
		if !isConnected {
			delete(w.workers, workerAddr)
		}
	}
	if len(w.workers) < int(w.settings.MinNumWorkerConnections) {
		w.loadBalancerConn.Call("LBServer.GetNodes", w.workerID, &addrSet)
		w.connectToWorkers(addrSet)
	}
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

func (w *Worker) BidirectionalSetup(request *WorkerRequest, response *WorkerResponse) error {

	workerAddr := request.Payload[0].(string)
	workerConn, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		delete(w.workers, workerAddr)
	} else {
		w.workers[workerAddr] = workerConn
		w.logger.Println("birectional setup complete")
	}
	return nil
}

// Pings all workers currently listed in the worker map
// If a connected worker fails to reply, that worker should be removed from the map
func (w *Worker) PingMiner(payload string, reply *bool) error {
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
