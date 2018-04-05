package main

// Usage: go run server.go [server port]

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"sync"
	"sync/atomic"
	"time"

	. "../lib/session"
	. "../lib/types"

	"github.com/DistributedClocks/GoVector/govec"
)

// The maximum time in milliseconds since the most recent heartbeat
// for a node to be considered disconnected.
//
const HEARTBEAT_INTERVAL = 2000

// nodes:    All known FS nodes, connected or not
// sessions: All known sessions
// logs:     All known logs
// index:    A structure mapping sessionIDs to logs
//
type Server struct {
	logger   *log.Logger
	nodes    *FSNodes
	sessions *Sessions
	logs     *Logs
	index    *Index
	golog    *govec.GoLog
}

// A map of node IDs to file system nodes.
//
type FSNodes struct {
	sync.RWMutex
	all map[string]*FSNode
}

// A map of session IDs to collections of nodes which are known to
// have saved the session.
//
type Sessions struct {
	sync.RWMutex
	all map[string]map[string]*FSNode
}

// A map of job IDs to collections of nodes which are known to have
// saved the log (containing the job).
//
type Logs struct {
	sync.RWMutex
	all map[string]map[string]*FSNode
}

// A map of session IDs to collections of job IDs associated with the
// session.
//
type Index struct {
	sync.RWMutex
	logs map[string]map[string]bool
}

// We will use exclusively atomic operations on lastHeartbeat
//
type FSNode struct {
	nodeID        string
	nodeAddr      string
	nodeConn      *rpc.Client
	lastHeartbeat *int64
}

func main() {
	if len(os.Args) != 2 {
		usage()
	}

	RegisterGob()

	server := new(Server)
	rpc.Register(server)

	server.init()
	server.listenRPC()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	wg.Wait()
}

////////////////////////////////////////////////////////////////////////////////////////////
// <PRIVATE METHODS>

func (s *Server) init() {
	s.logger = log.New(os.Stdout, "[Initializing] ", log.Lshortfile)
	s.nodes = &FSNodes{all: make(map[string]*FSNode)}
	s.sessions = &Sessions{all: make(map[string]map[string]*FSNode)}
	s.logs = &Logs{all: make(map[string]map[string]*FSNode)}
	s.index = &Index{logs: make(map[string]map[string]bool)}
	s.golog = govec.InitGoVector("FSServer", "FSServer")

	rand.Seed(time.Now().Unix())
}

func (s *Server) listenRPC() {
	var externalIP string

	// Use external IP (uncomment below) when deployed on Azure,
	// because this doesn't work on my home network

	// addrs, _ := net.InterfaceAddrs()
	// for _, a := range addrs {
	// 	if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
	// 		if ipnet.IP.To4() != nil {
	// 			externalIP = ipnet.IP.String()
	// 		}
	// 	}
	// }
	externalIP = "localhost:" + os.Args[1]
	tcpAddr, _ := net.ResolveTCPAddr("tcp", externalIP)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError(err)

	s.logger.Println("Now listening at " + fmt.Sprint(listener.Addr()))
	s.logger.SetPrefix("[Ready] ")

	go func() {
		for {
			conn, err := listener.Accept()
			checkError(err)
			s.logger.Println("New connection from " + conn.RemoteAddr().String())
			go rpc.ServeConn(conn)
		}
	}()
}

// Saves a session to a specified node. If the session is saved
// successfully, the node will be added to the map (s.sessions) so
// that the session can be retrieved from this node at a later time.
// If the session cannot be saved, then the node is removed from that
// map (since, if it was previously known to contain that session, it
// now has an outdated version).
//
func (s *Server) saveSessionToNode(session *Session, node *FSNode) {
	logMsg := "Saving session [" + session.ID + "] to node [" + node.nodeID + "]"
	s.logger.Println(logMsg)

	request := new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = *session
	request.Payload[1] = s.golog.PrepareSend(logMsg, []byte{})
	response := new(FSResponse)
	err := node.nodeConn.Call("FSNode.SaveSession", request, response)
	checkError(err)

	if len(response.Payload) > 0 && response.Payload[0].(bool) {
		s.sessions.addNode(session.ID, node)
		logMsg = "Session [" + session.ID + "] saved"
		s.logger.Println(logMsg)
		var recbuf []byte
		s.golog.UnpackReceive(logMsg, response.Payload[1].([]byte), &recbuf)
	} else {
		s.sessions.removeNode(session.ID, node.nodeID)
		logMsg = "Session [" + session.ID + "] could not be saved"
		s.logger.Println(logMsg)
		s.golog.LogLocalEvent(logMsg)
	}
}

// Attempts to retrieve a session from a specified node.
//
func (s *Server) getSessionFromNode(sessionID string, node *FSNode) *Session {
	logMsg := "Retrieving session [" + sessionID + "] from node [" + node.nodeID + "]"
	s.logger.Println(logMsg)

	request := new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = sessionID
	request.Payload[1] = s.golog.PrepareSend(logMsg, []byte{})
	response := new(FSResponse)
	err := node.nodeConn.Call("FSNode.GetSession", request, response)
	checkError(err)

	if len(response.Payload) == 0 {
		logMsg = "Session [" + sessionID + "] could not be retrieved"
		s.logger.Println(logMsg)
		s.golog.LogLocalEvent(logMsg)
		return nil
	} else {
		logMsg = "Session [" + sessionID + "] retrieved"
		s.logger.Println(logMsg)
		session := response.Payload[0].(Session)
		var recbuf []byte
		s.golog.UnpackReceive(logMsg, response.Payload[1].([]byte), &recbuf)

		return &session
	}
}

// A helper function for retrieving a single log specified by the
// given job ID. An attempt will be made to retrieve the log from any
// node which is known to have it, and if any retrieval fails, that
// node will be removed from the log map (s.logs).
//
func (s *Server) getLog(jobID string) *Log {
	nodes := s.logs.get(jobID)

	for _, node := range nodes {
		if isConnected(node) {
			_log := s.getLogFromNode(jobID, node)
			if _log != nil {
				return _log
			} else {
				s.logs.removeNode(jobID, node.nodeID)
			}
		}
	}

	return nil
}

// A helper function for retrieving a set of logs specified by a given
// session ID.
//
func (s *Server) getLogs(sessionID string) []Log {
	jobIDs := s.index.get(sessionID)
	if jobIDs == nil {
		return nil
	}

	logsMap := make(map[string]*Log)
	for jobID, _ := range jobIDs {
		_log := s.getLog(jobID)
		if _log != nil {
			logsMap[jobID] = _log
		}
	}
	if len(logsMap) == 0 {
		return nil
	}

	logs := make([]Log, len(logsMap))
	i := 0
	for _, _log := range logsMap {
		logs[i] = *_log
		i++
	}

	return logs
}

// Saves a log to a specified node. If the log is saved successfully,
// the node will be added to the map (s.logs) so that the log can be
// retrieved from this node at a later time. If the log cannot be
// saved, then the node is removed from that map (since, if it was
// previously known to contain that log, it now has an outdated
// version).
//
func (s *Server) saveLogToNode(_log *Log, node *FSNode) {
	logMsg := "Saving log [" + _log.Job.JobID + "] to node [" + node.nodeID + "]"
	s.logger.Println(logMsg)

	request := new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = *_log
	request.Payload[1] = s.golog.PrepareSend(logMsg, []byte{})
	response := new(FSResponse)
	err := node.nodeConn.Call("FSNode.SaveLog", request, response)
	checkError(err)

	if len(response.Payload) > 0 && response.Payload[0].(bool) {
		s.logs.addNode(_log.Job.JobID, node)
		logMsg = "Log [" + _log.Job.JobID + "] saved"
		s.logger.Println(logMsg)
		var recbuf []byte
		s.golog.UnpackReceive(logMsg, response.Payload[1].([]byte), &recbuf)
	} else {
		s.logs.removeNode(_log.Job.JobID, node.nodeID)
		logMsg = "Log [" + _log.Job.JobID + "] could not be saved"
		s.logger.Println(logMsg)
		s.golog.LogLocalEvent(logMsg)
	}
}

// Attempts to retrieve a log from a specified node.
//
func (s *Server) getLogFromNode(jobID string, node *FSNode) *Log {
	logMsg := "Retrieving log [" + jobID + "] from node [" + node.nodeID + "]"
	s.logger.Println(logMsg)

	request := new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = jobID
	request.Payload[1] = s.golog.PrepareSend(logMsg, []byte{})
	response := new(FSResponse)
	err := node.nodeConn.Call("FSNode.GetLog", request, response)
	checkError(err)

	if len(response.Payload) == 0 {
		logMsg = "Log [" + jobID + "] could not be retrieved"
		s.logger.Println(logMsg)
		s.golog.LogLocalEvent(logMsg)
		return nil
	} else {
		logMsg = "Log [" + jobID + "] retrieved"
		s.logger.Println(logMsg)
		_log := response.Payload[0].(Log)
		var recbuf []byte
		s.golog.UnpackReceive(logMsg, response.Payload[1].([]byte), &recbuf)
		return &_log
	}
}

// </PRIVATE METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <RPC METHODS>

// Hearbeat function - updates the node's most recent heartbeat.
//
func (s *Server) Heartbeat(nodeID string, _ *bool) (_ error) {
	s.nodes.heartbeat(nodeID)
	return
}

// Register a new or existing node. New nodes will be assigned a node
// ID, and existing nodes will have their node ID checked in the nodes
// map. Invalid nodes will be rejected.
//
func (s *Server) RegisterNode(request *FSRequest, response *FSResponse) (_ error) {
	nodeID := request.Payload[0].(string)
	nodeAddr := request.Payload[1].(string)
	response.Payload = make([]interface{}, 2)
	response.Payload[0] = false
	accepted := false

	if len(nodeID) == 0 {
		now := time.Now().UnixNano()
		nodeID = generateNodeID(5)
		node := &FSNode{
			nodeID: nodeID,
			nodeAddr: nodeAddr,
			lastHeartbeat: &now}
		s.nodes.add(node)

		response.Payload[0] = true
		response.Payload[1] = node.nodeID

		s.logger.Println("New node [" + nodeID + "] registered")
		accepted = true
	} else {
		if s.nodes.get(nodeID) != nil {
			response.Payload[0] = true
			s.logger.Println("Existing node [" + nodeID + "] registered")
			accepted = true
		} else {
			s.logger.Println("Node [" + nodeID + "] rejected")
			return
		}
	}

	if accepted {
		s.nodes.dial(nodeID, nodeAddr)
	}

	return
}

// Save a session to the file system. The file server will attempt to
// save the session to all connected file system nodes.
//
func (s *Server) SaveSession(request *FSRequest, response *FSResponse) (_ error) {
	session := request.Payload[0].(Session)
	logMsg := "Saving session [" + session.ID + "] to file system"

	s.logger.Println(logMsg)
	var recbuf []byte
	s.golog.UnpackReceive(logMsg, request.Payload[1].([]byte), &recbuf)

	nodes := s.nodes.getAll()
	for _, node := range nodes {
		if isConnected(node) {
			go s.saveSessionToNode(&session, node)
		} else {
			s.sessions.removeNode(session.ID, node.nodeID)
		}
	}

	logMsg = "Session [" + session.ID + "] save started"
	s.logger.Println(logMsg)

	response.Payload = make([]interface{}, 2)
	response.Payload[0] = true
	response.Payload[1] = s.golog.PrepareSend(logMsg, []byte{})

	return
}

// Get a session from the file system, given a session ID.
// If the session exists, the response payload will also include all
// saved logs associated with that session.
//
func (s *Server) GetSession(request *FSRequest, response *FSResponse) (_ error) {
	sessionID := request.Payload[0].(string)
	logMsg := "Retrieving session [" + sessionID + "] from file system"

	s.logger.Println(logMsg)
	var recbuf []byte
	s.golog.UnpackReceive(logMsg, request.Payload[1].([]byte), &recbuf)

	nodes := s.sessions.get(sessionID)

	for _, node := range nodes {
		if isConnected(node) {
			session := s.getSessionFromNode(sessionID, node)
			if session != nil {
				logMsg = "Sending session [" + sessionID + "] to worker"
				s.logger.Println(logMsg)

				response.Payload = make([]interface{}, 3)
				response.Payload[0] = *session
				response.Payload[1] = s.getLogs(sessionID)
				response.Payload[2] = s.golog.PrepareSend(logMsg, []byte{})

				break
			} else {
				s.sessions.removeNode(sessionID, node.nodeID)
			}
		}
	}

	return
}

// Save a log to the file system. The file server will attempt to save
// the log to all connected file system nodes.
//
func (s *Server) SaveLog(request *FSRequest, response *FSResponse) (_ error) {
	_log := request.Payload[0].(Log)
	logMsg := "Saving log [" + _log.Job.JobID + "] to file system"

	s.logger.Println(logMsg)
	var recbuf []byte
	s.golog.UnpackReceive(logMsg, request.Payload[1].([]byte), &recbuf)

	nodes := s.nodes.getAll()
	for _, node := range nodes {
		if isConnected(node) {
			go s.saveLogToNode(&_log, node)
		} else if s.logs.all[_log.Job.JobID] != nil {
			s.logs.removeNode(_log.Job.JobID, node.nodeID)
		}
	}

	s.index.addLog(_log.Job.SessionID, _log.Job.JobID)

	logMsg = "Log [" + _log.Job.SessionID + "] save started"
	s.logger.Println(logMsg)

	response.Payload = make([]interface{}, 2)
	response.Payload[0] = true
	response.Payload[1] = s.golog.PrepareSend(logMsg, []byte{})

	return
}

// Get a log from the file system, given a job ID.
//
func (s *Server) GetLog(request *FSRequest, response *FSResponse) (_ error) {
	jobID := request.Payload[0].(string)
	logMsg := "Retrieving log [" + jobID + "] from file system"

	s.logger.Println(logMsg)
	var recbuf []byte
	s.golog.UnpackReceive(logMsg, request.Payload[1].([]byte), &recbuf)

	_log := s.getLog(jobID)
	if _log != nil {
		logMsg = "Sending log [" + jobID + "] to worker"
		s.logger.Println(logMsg)

		response.Payload = make([]interface{}, 2)
		response.Payload[0] = *_log
		response.Payload[1] = s.golog.PrepareSend(logMsg, []byte{})
	}

	return
}

// </RPC METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <HELPER METHODS>

func isConnected(node *FSNode) bool {
	since := time.Now().UnixNano() - atomic.LoadInt64(node.lastHeartbeat)
	return since <= int64(HEARTBEAT_INTERVAL * time.Millisecond)
}

var ALPHABET = []rune("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

func generateNodeID(length int) string {
	id := make([]rune, length)
	for i := range id {
		id[i] = ALPHABET[rand.Intn(len(ALPHABET))]
	}
	return string(id)
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: go run server.go [server port]\n")
	os.Exit(1)
}

func checkError(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}

// </HELPER METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <ATOMIC HELPERS>

func (f *FSNodes) getAll() map[string]*FSNode {
	f.RLock()
	defer f.RUnlock()

	nodes := make(map[string]*FSNode)
	for key, val := range f.all {
		nodes[key] = val
	}

	return nodes
}

func (f *FSNodes) get(nodeID string) *FSNode {
	f.RLock()
	defer f.RUnlock()

	return f.all[nodeID]
}

func (f *FSNodes) add(node *FSNode) {
	f.Lock()
	defer f.Unlock()

	f.all[node.nodeID] = node
}

func (f *FSNodes) delete(nodeID string) {
	f.Lock()
	defer f.Unlock()

	delete(f.all, nodeID)
}

func (f *FSNodes) heartbeat(nodeID string) {
	f.RLock()
	defer f.RUnlock()

	if f.all[nodeID] != nil {
		atomic.StoreInt64(f.all[nodeID].lastHeartbeat, time.Now().UnixNano())
	}
}

func (f *FSNodes) dial(nodeID, addr string) {
	f.RLock()
	defer f.RUnlock()

	if f.all[nodeID] != nil {
		nodeConn, err := rpc.Dial("tcp", addr)
		checkError(err)
		f.all[nodeID].nodeConn = nodeConn
	}
}

func (s *Sessions) get(sessionID string) map[string]*FSNode {
	s.RLock()
	defer s.RUnlock()

	nodes := make(map[string]*FSNode)
	for key, val := range s.all[sessionID] {
		nodes[key] = val
	}

	return nodes
}

func (s *Sessions) addNode(sessionID string, node *FSNode) {
	s.Lock()
	defer s.Unlock()

	if s.all[sessionID] == nil {
		s.all[sessionID] = make(map[string]*FSNode)
	}

	s.all[sessionID][node.nodeID] = node
}

func (s *Sessions) removeNode(sessionID, nodeID string) {
	s.Lock()
	defer s.Unlock()

	if s.all[sessionID] != nil {
		delete(s.all[sessionID], nodeID)
	}
}

func (l *Logs) get(jobID string) map[string]*FSNode {
	l.RLock()
	defer l.RUnlock()

	nodes := make(map[string]*FSNode)
	for key, val := range l.all[jobID] {
		nodes[key] = val
	}

	return nodes
}

func (l *Logs) addNode(sessionID string, node *FSNode) {
	l.Lock()
	defer l.Unlock()

	if l.all[sessionID] == nil {
		l.all[sessionID] = make(map[string]*FSNode)
	}

	l.all[sessionID][node.nodeID] = node
}

func (l *Logs) removeNode(sessionID, nodeID string) {
	l.Lock()
	defer l.Unlock()

	if l.all[sessionID] != nil {
		delete(l.all[sessionID], nodeID)
	}
}

func (i *Index) get(sessionID string) map[string]bool {
	i.RLock()
	defer i.RUnlock()

	jobs := make(map[string]bool)
	for key, val := range i.logs[sessionID] {
		jobs[key] = val
	}

	return jobs
}

func (i *Index) addLog(sessionID, jobID string) {
	i.Lock()
	defer i.Unlock()

	if i.logs[sessionID] == nil {
		i.logs[sessionID] = make(map[string]bool)
	}

	i.logs[sessionID][jobID] = true
}

// </ATOMIC HELPERS>
////////////////////////////////////////////////////////////////////////////////////////////