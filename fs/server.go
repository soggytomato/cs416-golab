package main

// Usage: go run server.go [server port]

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"sync"
	"time"
	"math/rand"

	. "../util/types"
)

const HEARTBEAT_INTERVAL = 2000

type Server struct {
	logger   *log.Logger
	nodes    FSNodes
	sessions Sessions
	logs     Logs
	index    Index
}

type FSNodes struct {
	sync.RWMutex
	all map[string]*FSNode
}

type Sessions struct {
	sync.RWMutex
	all map[string]map[string]bool
}

type Logs struct {
	sync.RWMutex
	all map[string]map[string]bool
}

type Index struct {
	sync.RWMutex
	logs map[string]map[string]bool
}

type FSNode struct {
	nodeID        string
	nodeAddr      string
	nodeConn      *rpc.Client
	lastHeartbeat int64
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

	for {}
}

////////////////////////////////////////////////////////////////////////////////////////////
// <PRIVATE METHODS>

func (s *Server) init() {
	s.logger = log.New(os.Stdout, "[Initializing] ", log.Lshortfile)
	s.nodes = FSNodes{all: make(map[string]*FSNode)}
	s.sessions = Sessions{all: make(map[string]map[string]bool)}
	s.logs = Logs{all: make(map[string]map[string]bool)}
	s.index = Index{logs: make(map[string]map[string]bool)}

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
			s.logger.Println("New connection from "+conn.RemoteAddr().String())
			go rpc.ServeConn(conn)
		}
	}()
}

func (s *Server) saveSessionToNode(session *Session, node *FSNode) {
	s.logger.Println("Saving session [" + session.ID + "] to node [" + node.nodeID + "]")
	s.sessions.Lock()
	defer s.sessions.Unlock()

	request := new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = *session
	ok := false
	err := node.nodeConn.Call("FSNode.SaveSession", request, &ok)
	checkError(err)

	if s.sessions.all[session.ID] == nil {
		s.sessions.all[session.ID] = make(map[string]bool)
	}

	if ok {
		s.sessions.all[session.ID][node.nodeID] = true
		s.logger.Println("Session saved")
	} else {
		delete(s.sessions.all[session.ID], node.nodeID)
		s.logger.Println("Session could not be saved")
	}
}

func (s *Server) getSessionFromNode(sessionID string, node *FSNode) *Session {
	s.logger.Println("Retrieving session [" + sessionID + "] from node [" + node.nodeID + "]")

	request := new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = sessionID
	response := new(FSResponse)
	err := node.nodeConn.Call("FSNode.GetSession", request, response)
	checkError(err)

	if len(response.Payload) == 0 {
		s.logger.Println("Session could not be retrieved")
		return nil
	} else {
		s.logger.Println("Session retrieved")
		session := response.Payload[0].(Session)
		return &session
	}
}

func (s *Server) getLog(jobID string) *Log {
	s.nodes.RLock()
	s.logs.Lock()
	defer s.nodes.RUnlock()
	defer s.logs.Unlock()

	nodes := s.logs.all[jobID]

	for nodeID, _ := range nodes {
		node := s.nodes.all[nodeID]
		if isConnected(node) {
			_log := s.getLogFromNode(jobID, node)
			if _log != nil {
				return _log
			} else {
				delete(s.logs.all[jobID], node.nodeID)
			}
		}
	}

	return nil
}

func (s *Server) saveLogToNode(_log *Log, node *FSNode) {
	s.logger.Println("Saving log [" + _log.Job.JobID + "] to node [" + node.nodeID + "]")
	s.logs.Lock()
	defer s.logs.Unlock()

	request := new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = *_log
	ok := false
	err := node.nodeConn.Call("FSNode.SaveLog", request, &ok)
	checkError(err)

	if s.logs.all[_log.Job.JobID] == nil {
		s.logs.all[_log.Job.JobID] = make(map[string]bool)
	}

	if ok {
		s.logs.all[_log.Job.JobID][node.nodeID] = true
		s.logger.Println("Log saved")
	} else {
		delete(s.logs.all[_log.Job.JobID], node.nodeID)
		s.logger.Println("Log could not be saved")
	}
}

func (s *Server) getLogFromNode(jobID string, node *FSNode) *Log {
	s.logger.Println("Retrieving log [" + jobID + "] from node [" + node.nodeID + "]")

	request := new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = jobID
	response := new(FSResponse)
	err := node.nodeConn.Call("FSNode.GetLog", request, response)
	checkError(err)

	if len(response.Payload) == 0 {
		s.logger.Println("Log could not be retrieved")
		return nil
	} else {
		s.logger.Println("Log retrieved")
		_log := response.Payload[0].(Log)
		return &_log
	}
}

// </PRIVATE METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <RPC METHODS>

func (s *Server) Heartbeat(nodeID string, _ *bool) error {
	s.nodes.Lock()
	defer s.nodes.Unlock()

	if s.nodes.all[nodeID] != nil {
		s.nodes.all[nodeID].lastHeartbeat = time.Now().UnixNano()
	}

	return nil
}

func (s *Server) RegisterNode(request *FSRequest, response *FSResponse) error {
	s.nodes.Lock()
	defer s.nodes.Unlock()

	nodeID := request.Payload[0].(string)
	nodeAddr := request.Payload[1].(string)

	if len(nodeID) == 0 {
		nodeID = generateNodeID(16)
		s.nodes.all[nodeID] = &FSNode{
			nodeID: nodeID,
			nodeAddr: nodeAddr}

		response.Payload = make([]interface{}, 1)
		response.Payload[0] = nodeID

		s.logger.Println("New node [" + nodeID + "] registered")
	} else {
		if s.nodes.all[nodeID] != nil {
			s.logger.Println("Existing node [" + nodeID + "] registered")
		} else {
			s.logger.Println("Node [" + nodeID + "] rejected")
			return nil
		}
	}

	nodeConn, err := rpc.Dial("tcp", nodeAddr)
	checkError(err)
	s.nodes.all[nodeID].nodeConn = nodeConn

	return nil
}

func (s *Server) SaveSession(request *FSRequest, _ *bool) error {
	s.nodes.RLock()
	s.sessions.Lock()
	defer s.nodes.RUnlock()
	defer s.sessions.Unlock()

	session := request.Payload[0].(Session)

	for _, node := range s.nodes.all {
		if isConnected(node) {
			go s.saveSessionToNode(&session, node)
		} else if s.sessions.all[session.ID] != nil {
			delete(s.sessions.all[session.ID], node.nodeID)
		}
	}

	return nil
}

func (s *Server) GetSession(request *FSRequest, response *FSResponse) error {
	s.nodes.RLock()
	s.sessions.Lock()
	defer s.nodes.RUnlock()
	defer s.sessions.Unlock()

	sessionID := request.Payload[0].(string)
	nodes := s.sessions.all[sessionID]

	for nodeID, _ := range nodes {
		node := s.nodes.all[nodeID]
		if isConnected(node) {
			session := s.getSessionFromNode(sessionID, node)
			if session != nil {
				response.Payload = make([]interface{}, 1)
				response.Payload[0] = *session
				break
			} else {
				delete(s.sessions.all[sessionID], node.nodeID)
			}
		}
	}

	return nil
}

func (s *Server) SaveLog(request *FSRequest, _ *bool) error {
	s.nodes.RLock()
	s.logs.Lock()
	s.index.Lock()
	defer s.nodes.RUnlock()
	defer s.logs.Unlock()
	defer s.index.Unlock()

	_log := request.Payload[0].(Log)

	for _, node := range s.nodes.all {
		if isConnected(node) {
			go s.saveLogToNode(&_log, node)
		} else if s.logs.all[_log.Job.JobID] != nil {
			delete(s.logs.all[_log.Job.JobID], node.nodeID)
		}
	}

	if s.index.logs[_log.Job.SessionID] == nil {
		s.index.logs[_log.Job.SessionID] = make(map[string]bool)
	}
	s.index.logs[_log.Job.SessionID][_log.Job.JobID] = true

	return nil
}

func (s *Server) GetLog(request *FSRequest, response *FSResponse) error {
	jobID := request.Payload[0].(string)
	_log := s.getLog(jobID)
	if _log != nil {
		response.Payload = make([]interface{}, 1)
		response.Payload[0] = *_log
	}

	return nil
}

func (s *Server) GetLogs(request *FSRequest, response *FSResponse) error {
	s.index.RLock()
	defer s.index.RUnlock()

	sessionID := request.Payload[0].(string)
	jobIDs := s.index.logs[sessionID]
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

	response.Payload = make([]interface{}, 1)
	response.Payload[0] = logs

	return nil
}

// </RPC METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <HELPER METHODS>

func isConnected(node *FSNode) bool {
	return time.Now().UnixNano() - node.lastHeartbeat <= int64(HEARTBEAT_INTERVAL * time.Millisecond)
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
