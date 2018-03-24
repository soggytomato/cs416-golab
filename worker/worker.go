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

	. "../lib/types"
	"github.com/gorilla/websocket"
	// POC(CLI) relevant
	// "bufio"
	// "bytes"
	// "math/rand"
	// "strings"
)

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
	sessions         map[string]*Session
	clientSessions   map[string][]string
	localElements    []*Element
}

type WorkerResponse struct {
	Error   error
	Payload []interface{}
}

type WorkerRequest struct {
	Payload []interface{}
}

type NoCRDTError string

func (e NoCRDTError) Error() string {
	return fmt.Sprintf("Worker doesn't have sessionID [%s]", string(e))
}

// Used to send heartbeat to the server just shy of 1 second each beat
const TIME_BUFFER int = 500

// Since we are adding a character to the right of another character, we need
// a fake INITIAL_ID to use to place the first character in an empty message
const INITIAL_ID string = "12345"

func main() {
	gob.Register(map[string]*Element{})
	gob.Register(&net.TCPAddr{})
	gob.Register([]*Element{})
	gob.Register(&Element{})
	gob.Register(&Session{})
	worker := new(Worker)
	worker.logger = log.New(os.Stdout, "[Initializing] ", log.Lshortfile)
	worker.init()
	worker.listenRPC()
	worker.listenHTTP()
	worker.registerWithLB()
	worker.getWorkers()
	go worker.sendlocalElements()
	// worker.workerPrompt() //POC(CLI)
	for {

	}
}

func (w *Worker) init() {
	args := os.Args[1:]
	w.serverAddr = args[0]
	w.workers = make(map[string]*rpc.Client)
	w.sessions = make(map[string]*Session)
	w.clients = make(map[string]*websocket.Conn)
	w.clientSessions = make(map[string][]string)
}

//****POC(CLI) CODE***//

// func (w *Worker) workerPrompt() {
// 	reader := bufio.NewReader(os.Stdin)
// 	for {
// 		fmt.Print("Worker> ")
// 		cmd, _ := reader.ReadString('\n')
// 		if w.handleIntroCommand(cmd) == 1 {
// 			return
// 		}
// 	}
// }
//
// func (w *Worker) handleIntroCommand(cmd string) int {
// 	args := strings.Split(strings.TrimSpace(cmd), ",")
//
// 	switch args[0] {
// 	case "newSession":
// 		w.newSession()
// 	case "getSession":
// 		w.getSession(args[1])
// 	default:
// 		fmt.Println(" Invalid command.")
// 	}
//
// 	return 0
// }
//
// func (w *Worker) newSession() {
// 	sessionID := String(5)
// 	w.sessions[sessionID] = &Session{sessionID, make(map[string]*Element), "", 1}
// 	w.crdtPrompt(sessionID)
// }
//
// func (w *Worker) crdtPrompt(sessionID string) {
// 	reader := bufio.NewReader(os.Stdin)
// 	for {
// 		message := w.getMessage(w.sessions[sessionID])
// 		fmt.Println("SessionID:", sessionID)
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
// func (w *Worker) getMessage(session *Session) string {
// 	var buffer bytes.Buffer
// 	crdt := session.CRDT
// 	firstElement := crdt[session.Head]
// 	for firstElement != nil {
// 		fmt.Println(firstElement.ID, "->", firstElement.Text)
// 		buffer.WriteString(firstElement.Text)
// 		firstElement = crdt[firstElement.NextID]
// 	}
// 	return buffer.String()
// }
//
// func (w *Worker) handleCommand(cmd string) int {
// 	args := strings.Split(strings.TrimSpace(cmd), ",")
//
// 	switch args[0] {
// 	case "addRight":
// 		err := w.addRight(args[1], args[2], args[3])
// 		if checkError(err) != nil {
// 			return 0
// 		}
// 	case "exit":
// 		return 1
// 	default:
// 		fmt.Println(" Invalid command.")
// 	}
//
// 	return 0
// }

//**CRDT CODE**//

// Adds a character to the right of the prevID specified in the args
func (w *Worker) addRight(prevID, content, sessionID string) error {
	if !w.prevIDExists(prevID, sessionID) {
		return nil
	}
	session := w.sessions[sessionID]
	elementID := strconv.Itoa(session.Next) + strconv.Itoa(w.workerID)
	newElement := &Element{sessionID, strconv.Itoa(w.workerID), elementID, prevID, "", content, false}
	w.addToCRDT(newElement, session)
	return nil
}

func (w *Worker) addToCRDT(newElement *Element, session *Session) error {
	if w.firstCRDTEntry(newElement.ID, session) {
		w.addElementAndIncrementCounter(newElement, session)
		return nil
	}
	if w.replacingFirstElement(newElement, newElement.PrevID, newElement.ID, session) {
		w.addElementAndIncrementCounter(newElement, session)
		return nil
	}
	w.normalInsert(newElement, newElement.PrevID, newElement.ID, session)
	w.addElementAndIncrementCounter(newElement, session)
	return nil
}

func (w *Worker) deleteFromCRDT(element *Element, session *Session) error {
	session.CRDT[element.ID].Deleted = true

	return nil
}

// Check if the prevID actually exists; if true, continue with addRight
func (w *Worker) prevIDExists(prevID, sessionID string) bool {
	session := w.sessions[sessionID]
	if session != nil {
		if _, ok := session.CRDT[prevID]; ok || prevID == INITIAL_ID {
			return true
		} else {
			return false
		}
	} else {
		return false
	}
}

// The case where the first content is entered into a CRDT
func (w *Worker) firstCRDTEntry(elementID string, session *Session) bool {
	if len(session.CRDT) <= 0 {
		session.Head = elementID
		return true
	} else {
		return false
	}
}

// If your character is placed at the beginning of the message, it needs to become
// the new firstOp so we can iterate through the CRDT properly
func (w *Worker) replacingFirstElement(newElement *Element, prevID, elementID string, session *Session) bool {
	if prevID == "" {
		firstOp := session.CRDT[session.Head]
		newElement.NextID = session.Head
		firstOp.PrevID = elementID
		session.Head = elementID
		return true
	} else {
		return false
	}
}

// Any other insert that doesn't take place at the beginning or end is handled here
func (w *Worker) normalInsert(newElement *Element, prevID, elementID string, session *Session) {
	fmt.Println("prevID:", prevID)
	newPrevID := w.samePlaceInsertCheck(newElement, prevID, elementID, session)
	prevOp := session.CRDT[newPrevID]
	newElement.NextID = prevOp.NextID
	prevOp.NextID = elementID
}

// Checks if any other clients have made inserts to the same prevID. The algorithm
// compares the prevOp's nextID to the incomingOp ID - if nextID is greater, incomingOp
// will move further down the message until it is greater than the nextID
func (w *Worker) samePlaceInsertCheck(newElement *Element, prevID, elementID string, session *Session) string {
	var nextOpID int
	prevOp := session.CRDT[prevID]
	if prevOp.NextID != "" {
		nextOpID, _ = strconv.Atoi(prevOp.NextID)
		newOpID, _ := strconv.Atoi(elementID)
		for nextOpID >= newOpID && newElement.ClientID != session.CRDT[prevOp.NextID].ClientID {
			prevOp = session.CRDT[strconv.Itoa(nextOpID)]
			nextOpID, _ = strconv.Atoi(prevOp.NextID)
		}
		return prevOp.ID
	} else {
		return prevID
	}

}

// Once all the CRDT pointers are updated, the op can be added to the CRDT and the op
// number can be incremented
func (w *Worker) addElementAndIncrementCounter(newElement *Element, session *Session) {
	id := newElement.ID
	session.CRDT[id] = newElement
	w.localElements = append(w.localElements, newElement)
	session.Next++
}

// Send all of the ops made locally on this worker to all other connected workers
// After sending, wipe all localElements from the worker
func (w *Worker) sendlocalElements() error {
	for {
		time.Sleep(time.Second * 10)
		//w.getWorkers() // checks all workers, connects to more if needed
		request := new(WorkerRequest)
		request.Payload = make([]interface{}, 1)
		request.Payload[0] = w.localElements
		response := new(WorkerResponse)
		for workerAddr, workerCon := range w.workers {
			isConnected := false
			workerCon.Call("Worker.PingWorker", "", &isConnected)
			if isConnected {
				workerCon.Call("Worker.ApplyIncomingElements", request, response)
			} else {
				delete(w.workers, workerAddr)
			}
		}
		w.localElements = nil
	}
	return nil
}

// If the worker has the session in it's CRDT map, apply the op
// If it doesn't, skip over applying the op
// If it has applied these ops already, skip over applying the op
func (w *Worker) ApplyIncomingElements(request *WorkerRequest, response *WorkerResponse) error {
	elements := request.Payload[0].([]*Element)
	for _, element := range elements {
		session := w.sessions[element.SessionID]
		if session != nil {
			if session.CRDT[element.ID] == nil {
				w.addToCRDT(element, session)
			}
		}
	}
	return nil
}

// Client can provide the sessionID to get session from another worker
func (w *Worker) getSession(sessionID string) {
	response := new(WorkerResponse)
	for _, workerCon := range w.workers {
		var isConnected bool
		workerCon.Call("Worker.PingWorker", "", &isConnected)
		err := workerCon.Call("Worker.SendSession", sessionID, response)
		if err != nil {
			fmt.Println(err)
		} else {
			w.sessions[sessionID] = response.Payload[0].(*Session)
			// w.crdtPrompt(sessionID) // Used in POC(CLI)
			return
		}
	}
}

// If client tries to get a session, this function can be used to get that session
// if the worker has it in its CRDT map
func (w *Worker) SendSession(sessionID string, response *WorkerResponse) error {
	if w.sessions[sessionID] == nil {
		return NoCRDTError(sessionID)
	}
	response.Payload = make([]interface{}, 1)
	response.Payload[0] = w.sessions[sessionID]
	return nil
}

//**RPC SETUP CODE**//

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
	info := &WorkerInfo{w.localRPCAddr, w.localHTTPAddr}
	err = loadBalancerConn.Call("LBServer.RegisterNewWorker", info, settings)
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

// Gets workers from server if below MinNumMinerConnections
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
	}
	return nil
}

// Pings all workers currently listed in the worker map
// If a connected worker fails to reply, that worker should be removed from the map
func (w *Worker) PingWorker(payload string, reply *bool) error {
	*reply = true
	return nil
}

//**WEBSOCKET CODE**//

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

	_clientID, _ := r.URL.Query()["userID"]
	if len(_clientID) == 0 {
		http.Error(wr, "Missing clientID in URL parameter", http.StatusBadRequest)
	}

	_sessionID, _ := r.URL.Query()["sessionID"]
	if len(_sessionID) == 0 {
		http.Error(wr, "Missing sessionID in URL parameter", http.StatusBadRequest)
	}

	clientID := _clientID[0]
	sessionID := _sessionID[0]

	w.logger.Println("New socket connection from: ", clientID, sessionID)

	w.clients[clientID] = conn
	w.clientSessions[sessionID] = append(w.clientSessions[sessionID], clientID)

	go w.onElement(conn, clientID)
}

// Read function to always listen for messages from the browser
// If read fails, the websocket will be closed.
// Different commands should be handled here.
func (w *Worker) onElement(conn *websocket.Conn, userID string) {
	for {
		element := &Element{}
		err := conn.ReadJSON(element)
		if err != nil {
			w.logger.Println("Error reading from websocket: ", err)
			delete(w.clients, userID)
			return
		} else {
			w.logger.Println("Got element from "+userID+": ", element)
		}

		// Push to local elements
		w.localElements = append(w.localElements, element)

		// Update session CRDT accordingly
		if element.Deleted == true {
			w.deleteFromCRDT(element, w.sessions[element.SessionID])
		} else {
			w.addToCRDT(element, w.sessions[element.SessionID])
		}

		w.sendToClients(element)
	}
}

func (w *Worker) sendToClients(element *Element) {
	sessionID := element.SessionID

	var clientsToDelete []string
	for _, clientID := range w.clientSessions[sessionID] {
		conn := w.clients[clientID]

		err := conn.WriteJSON(element)
		if err != nil {
			w.logger.Println("Failed to send message to client '"+clientID+"':", err)

			clientsToDelete = append(clientsToDelete, clientID)
		}
	}

	w.deleteClients(sessionID, clientsToDelete)
}

//**UTIL CODE**//

func (w *Worker) deleteClients(sessionID string, clients []string) {
	for _, clientID := range clients {
		delete(w.clients, clientID)

		for i, id := range w.clientSessions[sessionID] {
			if id == clientID {
				w.clientSessions[sessionID] = append(w.clientSessions[sessionID][:i], w.clientSessions[sessionID][i+1:]...)
				break
			}
		}
	}
}

func checkError(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}

	return nil
}

// Code for creating random strings: only for POC(CLI)
// const charset = "abcdefghijklmnopqrstuvwxyz" +
// 	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
//
// var seededRand *rand.Rand = rand.New(
// 	rand.NewSource(time.Now().UnixNano()))
//
// func StringWithCharset(length int, charset string) string {
// 	b := make([]byte, length)
// 	for i := range b {
// 		b[i] = charset[seededRand.Intn(len(charset))]
// 	}
// 	return string(b)
// }
//
// func String(length int) string {
// 	return StringWithCharset(length, charset)
// }
