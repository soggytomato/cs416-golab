/*

Port is dynamic

Usage:

$ go run worker.go [loadbalancer ip:port]

*/
package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	. "../lib/cache"
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
	fserverAddr      string
	fsServerConn     *rpc.Client
	localRPCAddr     net.Addr
	localHTTPAddr    net.Addr
	externalIP       string
	clients          map[string]*websocket.Conn
	workers          map[string]*rpc.Client
	logger           *log.Logger
	sessions         map[string]*Session
	clientSessions   map[string][]string
	localElements    []*Element
	cache            *Cache
}

type LogSettings struct {
	JobID  string `json:"JobID"`
	Output string `json:"Output"`
}

type NoCRDTError string

func (e NoCRDTError) Error() string {
	return fmt.Sprintf("Worker doesn't have sessionID [%s]", string(e))
}

// Used to send heartbeat to the server just shy of 1 second each beat
const TIME_BUFFER int = 500
const ELEMENT_DELAY int = 2

// Since we are adding a character to the right of another character, we need
// a fake INITIAL_ID to use to place the first character in an empty message
const INITIAL_ID string = "12345"

const EXEC_DIR = "./execute"

func main() {
	gob.Register(map[string]*Element{})
	gob.Register(&net.TCPAddr{})
	gob.Register([]*Element{})
	gob.Register(&Element{})
	gob.Register(&Session{})
	gob.Register(Log{})
	gob.Register([]Log{})
	worker := new(Worker)
	worker.logger = log.New(os.Stdout, "[Initializing] ", log.Lshortfile)
	worker.init()
	worker.listenRPC()
	worker.listenHTTP()
	worker.registerWithLB()
	worker.connectToFS()
	worker.getWorkers()
	go worker.sendLocalElements()
	go worker.cache.Maintain()
	// worker.workerPrompt() //POC(CLI)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	wg.Wait()
}

func (w *Worker) init() {
	args := os.Args[1:]
	w.serverAddr = args[0]
	w.fserverAddr = args[1]
	w.workers = make(map[string]*rpc.Client)
	w.sessions = make(map[string]*Session)
	w.clients = make(map[string]*websocket.Conn)
	w.clientSessions = make(map[string][]string)

	w.cache = new(Cache)
	w.cache.Init()

	if _, err := os.Stat(EXEC_DIR); os.IsNotExist(err) {
		os.Mkdir(EXEC_DIR, 0755)
	}
}

func (w *Worker) connectToFS() {
	fsServerConn, err := rpc.Dial("tcp", w.fserverAddr)
	checkError(err)
	w.fsServerConn = fsServerConn
}

//****POC CODE***//

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
	newElement := &Element{sessionID, strconv.Itoa(w.workerID), elementID, prevID, "", content, false, time.Now().Unix()}
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
// the new firstElement so we can iterate through the CRDT properly
func (w *Worker) replacingFirstElement(newElement *Element, prevID, elementID string, session *Session) bool {
	if prevID == "" {
		firstElement := session.CRDT[session.Head]
		newElement.NextID = session.Head
		firstElement.PrevID = elementID
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
	prevElement := session.CRDT[newPrevID]
	newElement.NextID = prevElement.NextID
	prevElement.NextID = elementID
}

// Checks if any other clients have made inserts to the same prevID. The algorithm
// compares the prevElement's nextID to the incomingOp ID - if nextID is greater, incomingOp
// will move further down the message until it is greater than the nextID
func (w *Worker) samePlaceInsertCheck(newElement *Element, prevID, elementID string, session *Session) string {
	prevElement := session.CRDT[prevID]
	if prevElement.NextID != "" {
		nextElementID := prevElement.NextID
		for strings.Compare(nextElementID, elementID) == 1 && newElement.ClientID != session.CRDT[nextElementID].ClientID {
			prevElement = session.CRDT[nextElementID]
			nextElementID = prevElement.NextID
		}
		return prevElement.ID
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
func (w *Worker) sendLocalElements() error {
	for {
		time.Sleep(time.Second * time.Duration(ELEMENT_DELAY))
		//w.getWorkers() // checks all workers, connects to more if needed

		if len(w.localElements) > 0 {
			request := new(WorkerRequest)
			request.Payload = make([]interface{}, 1)
			request.Payload[0] = w.localElements
			response := new(WorkerResponse)
			w.logger.Println("Map of connceted workers:", w.workers)
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
	}
	return nil
}

// If the worker has the session in it's CRDT map, apply the op
// If it doesn't, skip over applying the op
// If it has applied these ops already, skip over applying the op
func (w *Worker) ApplyIncomingElements(request *WorkerRequest, response *WorkerResponse) error {
	elements := request.Payload[0].([]*Element)
	for _, element := range elements {
		w.cache.Add(element)

		session := w.sessions[element.SessionID]
		if session != nil {
			if session.CRDT[element.ID] == nil {
				w.addToCRDT(element, session)
			}
			if element.Deleted == true {
				w.deleteFromCRDT(element, session)
			}

			w.sendToClients(element)
		}
	}
	return nil
}

func (w *Worker) newSession(sessionID string) {
	w.sessions[sessionID] = &Session{sessionID, make(map[string]*Element), "", 1}
}

// Client can provide the sessionID to get session from another worker
func (w *Worker) getSession(sessionID string) bool {
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
			return true
		}
	}
	return false
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
	http.HandleFunc("/session", w.sessionHandler)
	http.HandleFunc("/recover", w.recoveryHandler)
	http.HandleFunc("/execute", w.executeJob)

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
				err = workerCon.Call("Worker.BidirectionalSetup", request, response)
				if err != nil {
					w.logger.Println("Error calling BidrectionalSetup:", err)
				}
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

func (w *Worker) sessionHandler(wr http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		_sessionID, _ := r.URL.Query()["sessionID"]
		if len(_sessionID) == 0 {
			http.Error(wr, "Missing sessionID in URL parameter", http.StatusBadRequest)
		}

		sessionID := _sessionID[0]

		wr.Header().Set("Content-Type", "application/json; charset=UTF-8")
		wr.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(wr).Encode(w.sessions[sessionID])
	} else if r.Method == "POST" {
		_sessionID, _ := r.URL.Query()["sessionID"]
		if len(_sessionID) == 0 {
			http.Error(wr, "Missing sessionID in URL parameter", http.StatusBadRequest)
		}

		_userID, _ := r.URL.Query()["userID"]
		if len(_userID) == 0 {
			http.Error(wr, "Missing userID in URL parameter", http.StatusBadRequest)
		}

		sessionID := _sessionID[0]
		userID := _userID[0]

		fmt.Println("Got delete request", sessionID, userID)

		w.deleteClients(sessionID, []string{userID})
	}
}

func (w *Worker) recoveryHandler(wr http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		_sessionID, _ := r.URL.Query()["sessionID"]
		if len(_sessionID) == 0 {
			http.Error(wr, "Missing sessionID in URL parameter", http.StatusBadRequest)
		}

		sessionID := _sessionID[0]

		wr.Header().Set("Content-Type", "application/json; charset=UTF-8")
		wr.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(wr).Encode(w.cache.Get(sessionID))
	}
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

	if _, ok := w.sessions[sessionID]; !ok && !w.getSession(sessionID) {
		w.newSession(sessionID)
	}

	w.clients[clientID] = conn
	w.clientSessions[sessionID] = append(w.clientSessions[sessionID], clientID)

	go w.onElement(conn, clientID)
}

// HTTP point to handle an execute job from client
// Assumption is the client will send a JSON object with the sessionID and code snippet in string form
// Returns a log ID for the browser to store
// Steps:
//		- Construct and save the log to the file system
//		- call Load Balancer with jobID
//		- return to client with jobID

type test_struct struct {
	SessionID string `json:"SessionID"`
	Snippet   string `json:"Snippet"`
}

func (w *Worker) executeJob(wr http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {
		w.logger.Println("Got a /execute POST Request")
		err := r.ParseForm()
		checkError(err)
		sessionID := r.FormValue("sessionID")
		snippet := r.FormValue("snippet")
		log := new(Log)
		log.Job = *new(Job)
		log.Job.SessionID = sessionID
		log.Job.Snippet = snippet
		t := time.Now()
		jobID := sessionID + t.Format("20060102150405")
		log.Job.JobID = jobID

		// Save to FileSystem
		request := new(FSRequest)
		request.Payload = make([]interface{}, 1)
		request.Payload[0] = log
		var ignored bool
		err = w.fsServerConn.Call("Server.SaveLog", request, &ignored)
		checkError(err)
		// Sending back jobID
		logSettings := *new(LogSettings)
		logSettings.JobID = jobID
		wr.Header().Set("Content-Type", "application/json; charset=UTF-8")
		wr.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(wr).Encode(logSettings)

		// Sending with go routine to not wait for return value
		go w.loadBalancerConn.Call("LBServer.NewJob", jobID, &ignored)
	}

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

		w.cache.Add(element)

		// TODO remove because we will buffer the sends
		w.sendToClients(element)
	}
}

func (w *Worker) sendToClients(element *Element) {
	sessionID := element.SessionID

	var clientsToDelete []string
	for _, clientID := range w.clientSessions[sessionID] {
		fmt.Println("Sending " + element.ClientID + " to user " + clientID)

		conn := w.clients[clientID]
		err := conn.WriteJSON(element)
		if err != nil {
			w.logger.Println("Failed to send message to client '"+clientID+"':", err)

			clientsToDelete = append(clientsToDelete, clientID)
		}
	}

	w.deleteClients(sessionID, clientsToDelete)
}

// Runs a job called by the load balancer
//  Steps:
//		- Gets log from File System
// 		- saves and compiles the file locally
//		- Runs the job
//		- saves the log to File system
//		- Acks back to Load Balancer that it is done
func (w *Worker) RunJob(request *WorkerRequest, response *WorkerResponse) error {
	w.logger.Println("RunJob Request")
	jobID := request.Payload[0].(string)
	fsRequest := new(FSRequest)
	fsRequest.Payload = make([]interface{}, 1)
	fsRequest.Payload[0] = jobID
	fsResponse := new(FSResponse)

	// Gets log from File System
	err := w.fsServerConn.Call("Server.GetLog", fsRequest, fsResponse)
	checkError(err)
	log := fsResponse.Payload[0].(Log)

	if !log.Job.Done { // Check if log has been executed yet already
		// 		- saves and compiles the file locally
		//		- Runs the job
		fileName := "runSnippet_" + jobID + ".go"
		filePath := path.Join(EXEC_DIR, fileName)
		file, _ := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0755)
		defer file.Close()
		file.Write([]byte(log.Job.Snippet))
		file.Sync()

		cmd := exec.Command("go", "run", filePath)
		var output, stderr bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &stderr
		var timedout bool
		timeoutCh := make(chan error, 1)
		go func() {
			timeoutCh <- cmd.Run()
		}()
		select {
		case <-timeoutCh:
			timedout = false
		case <-time.After(5 * time.Second):
			timedout = true
		}

		// Write the proper output to the log file
		if timedout {
			log.Output = "program timed out"
		} else if len(stderr.String()) == 0 {
			// No errors case
			log.Output = output.String()
		} else {
			// There was a compile or runtime error
			log.Output = sliceOutput(stderr.String(), fileName)
		}
		log.Job.Done = true
		var ignored bool
		fsRequest.Payload[0] = log

		// saves the log to File system
		w.fsServerConn.Call("Server.SaveLog", fsRequest, &ignored)
		os.Remove(filePath)
	}

	// Acks back to Load Balancer that it is done
	response.Payload = make([]interface{}, 1)
	response.Payload[0] = log
	return nil
}

func (w *Worker) SendLog(request *WorkerRequest, _ignored *bool) error {
	log := request.Payload[0].(Log)
	for _, clientConn := range w.clients {
		err := clientConn.WriteJSON(log)
		checkError(err)
	}
	return nil
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

// Function gets rid of weird command line outputs from errors
func sliceOutput(output string, fileName string) string {
	arr := strings.Split(output, "\n")
	var logOutput string
	for _, str := range arr {
		if str != "# command-line-arguments" {
			s := html.EscapeString(str)
			index := strings.Index(s, fileName)
			if index >= 0 {
				s = html.UnescapeString(s[len(fileName)+index+1:])
			} else {
				s = html.UnescapeString(s)
			}
			if len(logOutput) > 0 {
				logOutput += "\n" + s
			} else {
				logOutput += s
			}
		}
	}
	return logOutput
}

func checkError(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}

	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: go run worker.go [LBServer ip:port] [FSServer ip:port]\n")
	os.Exit(1)
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
