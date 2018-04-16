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
	"math"
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
	. "../lib/session"
	. "../lib/types"
	"github.com/DistributedClocks/GoVector/govec"
	"github.com/gorilla/websocket"
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
	modifiedSessions map[string]*Session
	clientSessions   map[string][]string
	logs             map[string]map[string]Log
	localElements    []Element
	elementsToAck    []Element
	cache            *Cache
	golog            *govec.GoLog
	mux              sync.Mutex
}

type LogSettings struct {
	JobID  string `json:"JobID"`
	Output string `json:"Output"`
}

type SessionAndLog struct {
	SessionRecord *Session
	LogRecord     []Log
}

type ClientRecovery struct {
	Session   []Element
	LogRecord []Log
}

type NoCRDTError string

func (e NoCRDTError) Error() string {
	return fmt.Sprintf("Worker doesn't have sessionID [%s]", string(e))
}

// Used to send heartbeat to the server just shy of 1 second each beat
const TIME_BUFFER int = 500
const ELEMENT_DELAY int = 2
const CHUNK_SIZE int = 30

// Turn off AUTO_SAVE to disable auto saving of sessions to the file
// system. This can be helpful for improving the comprehensibility of
// ShiViz logs during certain workflows.
const AUTO_SAVE bool = true

const EXEC_DIR = "./execute"

func main() {
	if len(os.Args) != 3 {
		usage()
	}
	gob.Register(map[string]*Element{})
	gob.Register(map[string]Log{})
	gob.Register(&net.TCPAddr{})
	gob.Register([]Element{})
	gob.Register([]*Element{})
	gob.Register(&Element{})
	gob.Register(Session{})
	gob.Register(Job{})
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
	w.modifiedSessions = make(map[string]*Session)
	w.logs = make(map[string]map[string]Log)

	w.cache = new(Cache)
	w.cache.Init()
	if _, err := os.Stat(EXEC_DIR); os.IsNotExist(err) {
		os.Mkdir(EXEC_DIR, 0755)
	}
}

func (w *Worker) connectToFS() {
	fsServerConn, err := rpc.Dial("tcp", w.fserverAddr)
	w.checkError(err)
	w.fsServerConn = fsServerConn
}

// Send all of the ops made locally on this worker to all other connected workers
// After sending, wipe all localElements from the worker
func (w *Worker) sendLocalElements() error {
	for {
		time.Sleep(time.Second * time.Duration(ELEMENT_DELAY))

		if AUTO_SAVE {
			w.saveModifiedSessionsToFS()
		}

		numLocalElements := len(w.localElements)
		numAckElements := len(w.elementsToAck)
		if numLocalElements > 0 || numAckElements > 0 {
			numSuccess := 0

			elementQueue := append(w.localElements, w.elementsToAck...)
			numChunks := int(math.Ceil(float64(len(elementQueue)) / float64(CHUNK_SIZE)))
			numElements := len(elementQueue)

			w.logger.Println("Sending local elements -- Map of connected workers:", w.workers)

			request := new(WorkerRequest)
			request.Payload = make([]interface{}, 1)
			response := new(WorkerResponse)
			for workerAddr, workerCon := range w.workers {
				isConnected := false

				// Check if worker is connected
				workerCon.Call("Worker.PingWorker", "", &isConnected)
				if !isConnected {
					w.logger.Println("Lost worker: ", workerAddr)

					delete(w.workers, workerAddr)
					if len(w.workers) < w.settings.MinNumWorkerConnections {
						w.getWorkers()
					}
				}

				// Break elements into chunks and send to worker
				sentSuccessfully := true
				chunkNum := 0
				for chunkNum < numChunks {
					from := chunkNum * CHUNK_SIZE
					to := from + CHUNK_SIZE
					if to > numElements {
						to = numElements
					}

					request.Payload[0] = elementQueue[from:to]
					err := workerCon.Call("Worker.ApplyIncomingElements", request, response)
					if err != nil {
						w.logger.Println("Received error when trying to send chunk of elements to worker ", workerAddr, ": \n", err)

						sentSuccessfully = false
						break
					}

					chunkNum++
				}

				// If all elements were sent successfully, increment
				// number of successes
				if sentSuccessfully {
					numSuccess++
				}
			}

			w.ackElements(numAckElements, numSuccess)

			w.localElements = w.localElements[numLocalElements:]
		}
	}
	return nil
}

// If the worker has the session in it's CRDT map, apply the op
// If it doesn't, skip over applying the op
// If it has applied these ops already, skip over applying the op
func (w *Worker) ApplyIncomingElements(request *WorkerRequest, response *WorkerResponse) error {
	elements := request.Payload[0].([]Element)
	for _, element := range elements {
		w.cache.Add(element)

		// Send to clients if we actually added to the CRDT
		// if not, we already had it...
		if w.addToSession(element) {
			w.sendToClients(element)
		}
	}

	return nil
}

func (w *Worker) saveModifiedSessionsToFS() {
	for sessionID, session := range w.modifiedSessions {
		logMsg := "Saving session [" + sessionID + "] to file system"
		w.logger.Println(logMsg)

		request := new(FSRequest)
		request.Payload = make([]interface{}, 2)
		request.Payload[0] = session
		request.Payload[1] = w.golog.PrepareSend(logMsg, []byte{})
		response := new(FSResponse)

		err := w.fsServerConn.Call("Server.SaveSession", request, response)
		if err == nil && len(response.Payload) > 0 {
			logMsg = "Session [" + sessionID + "] sent"
			delete(w.modifiedSessions, sessionID)
			w.logger.Println(logMsg)
			var recbuf []byte
			w.golog.UnpackReceive(logMsg, response.Payload[1].([]byte), &recbuf)
		} else {
			w.logger.Println("saveModifiedSessionsToFS:", err)
			logMsg = "Session [" + sessionID + "] could not be sent"
			w.logger.Println(logMsg)
			w.golog.LogLocalEvent(logMsg)
		}
	}
}

// Load balancer calls CreateNewSession when it receives a request from a client
// using an ID it has not seen before. Worker stores a new Session locally and saves
// it to the FS
func (w *Worker) CreateNewSession(sessionID string, _ *bool) error {
	logMsg := "Saving session [" + sessionID + "] to file system"
	w.logger.Println(logMsg)

	request := new(FSRequest)
	session := &Session{ID: sessionID, CRDT: make(map[string]*Element)}

	w.sessions[sessionID] = session
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = w.sessions[sessionID]
	request.Payload[1] = w.golog.PrepareSend(logMsg, []byte{})
	response := new(FSResponse)

	err := w.fsServerConn.Call("Server.SaveSession", request, response)
	if err == nil && len(response.Payload) > 0 {
		logMsg = "Session [" + sessionID + "] sent"
		w.logger.Println(logMsg)
		var recbuf []byte
		w.golog.UnpackReceive(logMsg, response.Payload[1].([]byte), &recbuf)
	} else {
		w.logger.Println("CreateNewSession:", err)
		logMsg = "Session [" + sessionID + "] could not be sent"
		w.logger.Println(logMsg)
		w.golog.LogLocalEvent(logMsg)
	}

	return nil
}

// If worker doesn't have Session, contact other workers/FS to load the Session
// Once stored, worker will actively update Session as Elements arrive
func (w *Worker) LoadSession(sessionID string, response *bool) error {
	if w.sessions[sessionID] == nil {
		w.getSessionAndLogs(sessionID)
	}

	return nil
}

// Get the Session from a connected worker or get it from the FS
func (w *Worker) getSessionAndLogs(sessionID string) bool {
	// Mark the session as pending, so the cache doesn't flush
	w.cache.AddPending(sessionID)

	response := new(WorkerResponse)
	for _, workerCon := range w.workers {
		var isConnected bool
		workerCon.Call("Worker.PingWorker", "", &isConnected)
		err := workerCon.Call("Worker.GetSession", sessionID, response)
		if err != nil {
			w.logger.Println("Failed to retrieve session and logs for session "+sessionID+"\n", err)
		} else {
			session := response.Payload[0].(Session)
			logs := response.Payload[1].(map[string]Log)
			if len(logs) > 0 {
				if _, exists := w.logs[sessionID]; exists {
					for _, log := range logs {
						w.logs[sessionID][log.Job.JobID] = log
					}
				} else {
					w.logs[sessionID] = logs
				}
			}
			w.sessions[sessionID] = &session
			return true
		}
	}

	// If worker's neighbours cannot provide the session, contact the file server for the session
	if w.sessions[sessionID] == nil {
		logMsg := "Retrieving session [" + sessionID + "] from file system"
		w.logger.Println(logMsg)

		fsRequest := new(FSRequest)
		fsResponse := new(FSResponse)
		fsRequest.Payload = make([]interface{}, 2)
		fsRequest.Payload[0] = sessionID
		fsRequest.Payload[1] = w.golog.PrepareSend(logMsg, []byte{})

		err := w.fsServerConn.Call("Server.GetSession", fsRequest, fsResponse)
		if err != nil {
			w.logger.Println("getSessionAndLogs:", err)
			logMsg = "Session [" + sessionID + "] could not be retrieved"
			w.logger.Println(logMsg)
			w.golog.LogLocalEvent(logMsg)
		} else {
			logMsg = "Session [" + sessionID + "] retrieved"
			w.logger.Println(logMsg)
			session := fsResponse.Payload[0].(Session)
			logs := fsResponse.Payload[1].([]Log)
			var recbuf []byte
			w.golog.UnpackReceive(logMsg, fsResponse.Payload[2].([]byte), &recbuf)

			if _, exists := w.logs[sessionID]; exists {
				for _, log := range logs {
					w.logs[sessionID][log.Job.JobID] = log
				}
			} else {
				tempLogMap := make(map[string]Log)
				for _, log := range logs {
					tempLogMap[log.Job.JobID] = log
				}
				w.logs[sessionID] = tempLogMap
			}
			w.sessions[sessionID] = &session
			// TODO handle cached elements
			return true
		}
	}

	// Apply the cached elements to the session
	if w.sessions[sessionID] != nil {
		cachedElements := w.cache.Get(sessionID)
		for _, element := range cachedElements {
			w.addToSession(element)
		}
	}
	// Remove pending status on session
	w.cache.RemovePending(sessionID)

	return false
}

// If client tries to get a session, this function can be used to get that session
// if the worker has it in its CRDT map
func (w *Worker) GetSession(sessionID string, response *WorkerResponse) error {
	if w.sessions[sessionID] == nil {
		return NoCRDTError(sessionID)
	}
	response.Payload = make([]interface{}, 2)
	response.Payload[0] = w.sessions[sessionID]
	response.Payload[1] = w.logs[sessionID]
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
	w.checkError(err)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	w.checkError(err)
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
	http.HandleFunc("/execute", w.executeHandler)

	http.HandleFunc("/ws", w.wsHandler)
	httpAddr, err := net.ResolveTCPAddr("tcp", w.externalIP)
	w.checkError(err)
	listener, err := net.ListenTCP("tcp", httpAddr)
	w.checkError(err)
	w.localHTTPAddr = listener.Addr()
	go http.Serve(listener, nil)
	w.logger.Println("listening for HTTP on: ", listener.Addr().String())
}

func (w *Worker) registerWithLB() {
	loadBalancerConn, err := rpc.Dial("tcp", w.serverAddr)
	w.checkError(err)
	settings := new(WorkerNetSettings)
	info := &WorkerInfo{w.localRPCAddr, w.localHTTPAddr}
	err = loadBalancerConn.Call("LBServer.RegisterNewWorker", info, settings)
	w.checkError(err)
	w.settings = settings
	w.workerID = settings.WorkerID
	logID := "Worker_" + strconv.Itoa(w.workerID)
	w.golog = govec.InitGoVector(logID, logID)

	go w.startHeartBeat()
	w.logger.SetPrefix("[Worker: " + strconv.Itoa(w.workerID) + "] ")
	w.loadBalancerConn = loadBalancerConn
}

func (w *Worker) startHeartBeat() {
	var ignored bool
	request := new(WorkerRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = w.workerID
	request.Payload[1] = len(w.clients)
	w.loadBalancerConn.Call("LBServer.HeartBeat", request, &ignored)
	for {
		time.Sleep(time.Duration(w.settings.HeartBeat-TIME_BUFFER) * time.Millisecond)
		request.Payload[1] = len(w.clients)
		w.loadBalancerConn.Call("LBServer.HeartBeat", request, &ignored)
	}
}

// Gets workers from server if below MinNumMinerConnections
func (w *Worker) getWorkers() {
	var addrSet []net.Addr
	for workerAddr, workerCon := range w.workers {
		isConnected := false
		workerCon.Call("Worker.PingWorker", "", &isConnected)
		if !isConnected {
			w.logger.Println("Lost worker: ", workerAddr)

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
				w.checkError(err)
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
		var sessionAndLog SessionAndLog
		sessionAndLog.SessionRecord = w.sessions[sessionID]
		for _, log := range w.logs[sessionID] {
			sessionAndLog.LogRecord = append(sessionAndLog.LogRecord, log)
		}
		json.NewEncoder(wr).Encode(sessionAndLog)
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

		w.deleteClients(sessionID, []string{userID})

		w.logger.Println("User " + userID + " has closed session " + sessionID)
	}
}

func (w *Worker) recoveryHandler(wr http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		_sessionID, _ := r.URL.Query()["sessionID"]
		if len(_sessionID) == 0 {
			http.Error(wr, "Missing sessionID in URL parameter", http.StatusBadRequest)
		}

		sessionID := _sessionID[0]

		if w.sessions[sessionID] == nil {
			w.getSessionAndLogs(sessionID)
		}

		wr.Header().Set("Content-Type", "application/json; charset=UTF-8")
		wr.Header().Set("Access-Control-Allow-Origin", "*")
		var clientRec ClientRecovery
		clientRec.Session = w.cache.Get(sessionID)
		for _, log := range w.logs[sessionID] {
			clientRec.LogRecord = append(clientRec.LogRecord, log)
		}
		json.NewEncoder(wr).Encode(clientRec)
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

func (w *Worker) executeHandler(wr http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		w.logger.Println("Got a /execute POST Request")
		err := r.ParseForm()
		w.checkError(err)
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
		logMsg := "Saving log [" + jobID + "] to file system"
		w.logger.Println(logMsg)

		request := new(FSRequest)
		request.Payload = make([]interface{}, 2)
		request.Payload[0] = log
		request.Payload[1] = w.golog.PrepareSend(logMsg, []byte{})
		response := new(FSResponse)

		err = w.fsServerConn.Call("Server.SaveLog", request, response)
		if err == nil && len(response.Payload) > 0 {
			logMsg = "Log [" + jobID + "] sent"
			w.logger.Println(logMsg)
			var recbuf []byte
			w.golog.UnpackReceive(logMsg, response.Payload[1].([]byte), &recbuf)
		} else {
			w.logger.Println("executeHandler:", err)
			logMsg = "Log [" + jobID + "] could not be sent"
			w.logger.Println(logMsg)
			w.golog.LogLocalEvent(logMsg)
		}

		// Sending back jobID
		logSettings := *new(LogSettings)
		logSettings.JobID = jobID
		wr.Header().Set("Content-Type", "application/json; charset=UTF-8")
		wr.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(wr).Encode(logSettings)

		// Sending with go routine to not wait for return value
		go func() {
			logMsg = "Sending job [" + jobID + "] to load balancer"
			w.logger.Println(logMsg)

			wrequest := new(WorkerRequest)
			wrequest.Payload = make([]interface{}, 3)
			wrequest.Payload[0] = jobID
			wrequest.Payload[1] = strconv.Itoa(w.workerID)
			wrequest.Payload[2] = w.golog.PrepareSend(logMsg, []byte{})
			wresponse := new(WorkerResponse)

			err = w.loadBalancerConn.Call("LBServer.NewJob", wrequest, wresponse)
			w.checkError(err)
			if err == nil && len(wresponse.Payload) > 0 {
				logMsg = "Job [" + jobID + "] sent and finished"
				var recbuf []byte
				w.golog.UnpackReceive(logMsg, wresponse.Payload[0].([]byte), &recbuf)
			} else {
				logMsg = "Job [" + jobID + "] could not be finished"
				w.golog.LogLocalEvent(logMsg)
			}
			w.logger.Println(logMsg)
		}()
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

		if w.addToSession(*element) {
			w.sendToClients(*element)
		}

		w.elementsToAck = append(w.elementsToAck, *element)
	}
}

func cleanAcks(numAcks int) int {
	if numAcks > len(w.elementsToAck) {
		numAcks = len(w.elementsToAck)
	}
	_numAcks := numAcks

	for i := 0; i < _numAcks; i++ {
		element := w.elementsToAck[i]
		clientID := element.ClientID

		conn := w.clients[clientID]
		if conn == nil {
			w.elementsToAck = append(w.elementsToAck[:i], w.elementsToAck[i+1:]...)

			_numAcks--
		}
	}

	return _numAcks
}

func (w *Worker) ackElements(numAcks int, numSuccess int) {
	// Clean for non-existent clients
	_numAcks := w.cleanAcks(numAcks)

	// Exit if the original number of acks is too high
	// something wasn't right...
	// Otherwise update numAcks for new num
	if numAcks > len(w.elementsToAck) {
		return
	} else {
		numAcks = _numAcks
	}

	// If we sent to minimum number of workers, ack all elements
	if numSuccess >= w.settings.MinNumWorkerConnections {
		for i := 0; i < numAcks; i++ {
			element := w.elementsToAck[i]
			clientID := element.ClientID

			w.sendToClient(clientID, element)
		}

		w.elementsToAck = w.elementsToAck[numAcks:]
	}
}

func (w *Worker) sendToClients(element Element) {
	sessionID := element.SessionID
	clientID := element.ClientID

	for _, _clientID := range w.clientSessions[sessionID] {
		if _clientID == clientID {
			continue
		}

		w.sendToClient(_clientID, element)
	}
}

func (w *Worker) sendToClient(clientID string, element Element) (sent bool, err error) {
	sent = true

	conn := w.clients[clientID]
	if conn != nil {
		w.mux.Lock()
		err = conn.WriteJSON(element)
		w.mux.Unlock()
		if err != nil {
			w.logger.Println("Failed to send message to client '"+clientID+"':", err)
			sent = false
		}
	} else {
		sent = false
	}

	if !sent || err != nil {
		w.deleteClients(element.SessionID, []string{clientID})
	}

	return
}

// Runs a job called by the load balancer
//  Steps:
//		- Gets log from File System
// 		- saves and compiles the file locally
//		- Runs the job
//		- saves the log to File system
//		- Acks back to Load Balancer that it is done
func (w *Worker) RunJob(request *WorkerRequest, response *WorkerResponse) error {
	jobID := request.Payload[0].(string)
	logMsg := "Running job [" + jobID + "]"

	w.logger.Println(logMsg)
	var recbuf []byte
	w.golog.UnpackReceive(logMsg, request.Payload[1].([]byte), &recbuf)

	// Gets log from File System
	var log Log
	for {
		logMsg := "Retrieving log [" + jobID + "] from file system"
		w.logger.Println(logMsg)

		fsRequest := new(FSRequest)
		fsRequest.Payload = make([]interface{}, 2)
		fsRequest.Payload[0] = jobID
		fsRequest.Payload[1] = w.golog.PrepareSend(logMsg, []byte{})
		fsResponse := new(FSResponse)

		err := w.fsServerConn.Call("Server.GetLog", fsRequest, fsResponse)
		w.checkError(err)
		if err == nil && len(fsResponse.Payload) > 0 {
			logMsg = "Log [" + jobID + "] retrieved"
			log = fsResponse.Payload[0].(Log)
			var recbuf []byte
			w.golog.UnpackReceive(logMsg, fsResponse.Payload[1].([]byte), &recbuf)
			break
		} else {
			logMsg = "Log [" + jobID + "] could not be retrieved"
			w.golog.LogLocalEvent(logMsg)
		}
		w.logger.Println(logMsg)
		time.Sleep(250 * time.Millisecond)
	}

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

		logMsg := "Saving log [" + jobID + "] to file system"
		w.logger.Println(logMsg)

		fsRequest := new(FSRequest)
		fsRequest.Payload = make([]interface{}, 2)
		fsRequest.Payload[0] = log
		fsRequest.Payload[1] = w.golog.PrepareSend(logMsg, []byte{})
		fsResponse := new(FSResponse)

		// saves the log to File system
		err := w.fsServerConn.Call("Server.SaveLog", fsRequest, fsResponse)
		if err == nil && len(fsResponse.Payload) > 0 {
			logMsg = "Log [" + jobID + "] sent"
			var recbuf []byte
			w.golog.UnpackReceive(logMsg, fsResponse.Payload[1].([]byte), &recbuf)
		} else {
			w.logger.Println("executeHandler:", err)
			logMsg = "Log [" + jobID + "] could not be sent"
			w.golog.LogLocalEvent(logMsg)
		}
		w.logger.Println(logMsg)

		os.Remove(filePath)
	}

	logMsg = "Job [" + jobID + "] finished"
	w.logger.Println(logMsg)

	// Acks back to Load Balancer that it is done
	response.Payload = make([]interface{}, 2)
	response.Payload[0] = log
	response.Payload[1] = w.golog.PrepareSend(logMsg, []byte{})

	return nil
}

func (w *Worker) SendLog(request *WorkerRequest, response *WorkerResponse) error {
	log := request.Payload[0].(Log)
	logMsg := "Received log [" + log.Job.JobID + "] from load balancer"

	w.logger.Println(logMsg)
	var recbuf []byte
	w.golog.UnpackReceive(logMsg, request.Payload[1].([]byte), &recbuf)

	if _, exists := w.logs[log.Job.SessionID]; exists {
		w.logs[log.Job.SessionID][log.Job.JobID] = log
	} else {
		tempLogMap := make(map[string]Log)
		tempLogMap[log.Job.JobID] = log
		w.logs[log.Job.SessionID] = tempLogMap
	}
	//w.logs[log.Job.SessionID][log.Job.JobID] = log
	//w.logs[log.Job.SessionID] = append(w.logs[log.Job.SessionID], log)
	var clientsToDelete []string
	for clientID, clientConn := range w.clients {
		if clientConn != nil {
			err := clientConn.WriteJSON(log)
			w.checkError(err)
		} else {
			clientsToDelete = append(clientsToDelete, clientID)
		}
	}
	w.deleteClients(log.Job.SessionID, clientsToDelete)

	logMsg = "Log [" + log.Job.JobID + "] sent to clients"
	w.logger.Println(logMsg)
	response.Payload = make([]interface{}, 1)
	response.Payload[0] = w.golog.PrepareSend(logMsg, []byte{})

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

func (w *Worker) checkError(err error) error {
	if err != nil {
		w.logger.Println("Error:", err)
		return err
	}

	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: go run worker.go [LBServer ip:port] [FSServer ip:port]\n")
	os.Exit(1)
}

//**CRDT CODE**//

// Adds a character to the right of the prevID specified in the args
func (w *Worker) addRight(prevID, content, sessionID string) error {
	session := w.sessions[sessionID]
	elementID := strconv.Itoa(session.Next) + strconv.Itoa(w.workerID)
	newElement := &Element{sessionID, strconv.Itoa(w.workerID), elementID, prevID, "", content, false, time.Now().Unix()}
	w.addToSession(*newElement)

	return nil
}

func (w *Worker) addToSession(element Element) (processed bool) {
	_element := element

	sessionID := element.SessionID
	session := w.sessions[sessionID]
	if session == nil {
		return false
	}

	if element.Deleted == true {
		processed = session.Delete(element)
	} else {
		processed = session.Add(element)
	}

	if processed {
		w.modifiedSessions[sessionID] = session
		w.localElements = append(w.localElements, _element)
	}

	return
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
// 		if w.checkError(err) != nil {
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
