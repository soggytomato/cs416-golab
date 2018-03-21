package main

// Usage: go run fsnode.go [server ip:port]

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"time"
	"encoding/gob"
	"bytes"
	"path"
	"io/ioutil"

	. "../util/types"
)

const SESS_DIR = "./session"
const LOG_DIR = "./log"
const NODE_ID_PATH = "nodeID"
const HEARTBEAT_INTERVAL = 500

type FSNode struct {
	logger     *log.Logger
	nodeAddr   string
	serverAddr string
	serverConn *rpc.Client
	id         string
}

func main() {
	if len(os.Args) != 2 {
		usage()
	}

	RegisterGob()

	fsnode := new(FSNode)
	rpc.Register(fsnode)

	fsnode.init()
	fsnode.listenRPC()
	fsnode.registerWithServer()

	for {
	}
}

////////////////////////////////////////////////////////////////////////////////////////////
// <PRIVATE METHODS>

func (f *FSNode) init() {
	f.logger = log.New(os.Stdout, "[Initializing] ", log.Lshortfile)
	f.serverAddr = os.Args[1]

	exists, err := checkFileOrDirectory(SESS_DIR)
	checkError(err)
	if !exists {
		os.Mkdir(SESS_DIR, 0755)
	}

	exists, err = checkFileOrDirectory(LOG_DIR)
	checkError(err)
	if !exists {
		os.Mkdir(LOG_DIR, 0755)
	}
}

func (f *FSNode) listenRPC() {
	var externalIP string

	// Use external IP (uncomment below) when deployed on Azure,
	// because his doesn't work on my home network

	// addrs, _ := net.InterfaceAddrs()
	// for _, a := range addrs {
	// 	if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
	// 		if ipnet.IP.To4() != nil {
	// 			externalIP = ipnet.IP.String()
	// 		}
	// 	}
	// }
	externalIP = "localhost:0"
	tcpAddr, _ := net.ResolveTCPAddr("tcp", externalIP)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError(err)

	f.nodeAddr = listener.Addr().String()

	f.logger.Println("Now listening at " + f.nodeAddr)
	f.logger.SetPrefix("[Ready] ")

	go func() {
		for {
			conn, err := listener.Accept()
			checkError(err)
			f.logger.Println("New connection from " + conn.RemoteAddr().String())
			go rpc.ServeConn(conn)
		}
	}()
}

func (f *FSNode) registerWithServer() {
	serverConn, err := rpc.Dial("tcp", f.serverAddr)
	checkError(err)

	nodeID := getNodeID()
	request := new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = nodeID
	request.Payload[1] = f.nodeAddr
	response := new(FSResponse)

	serverConn.Call("Server.RegisterNode", request, response)
	if len(response.Payload) > 0 {
		nodeID = response.Payload[0].(string)
		storeNodeID(nodeID)

		f.logger.Println("Registered as new node")
	} else {
		f.logger.Println("Registered as existing node")
	}

	f.serverConn = serverConn
	f.id = nodeID
	go f.heartbeat()

	f.logger.Println("Node [" + f.id + "] connected to server")
}

func (f *FSNode) heartbeat() {
	ignored := false
	for {
		f.serverConn.Call("Server.Heartbeat", f.id, &ignored)
		time.Sleep(time.Duration(HEARTBEAT_INTERVAL * time.Millisecond))
	}
}

// </PRIVATE METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <RPC METHODS>

func (f *FSNode) SaveSession(request *FSRequest, ok *bool) error {
	session := request.Payload[0].(Session)
	f.logger.Println("Saving session [" + session.ID + "] to disk")

	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(session)

	filePath := path.Join(SESS_DIR, session.ID)
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0755)
	checkError(err)
	defer file.Close()
	err = file.Truncate(0)
	checkError(err)

	_, err = file.Write(buffer.Bytes())
	*ok = checkError(err) == nil

	file.Sync()

	return nil
}

func (f *FSNode) GetSession(request *FSRequest, response *FSResponse) error {
	sessionID := request.Payload[0].(string)
	f.logger.Println("Retrieving session [" + sessionID + "] from disk")
	filePath := path.Join(SESS_DIR, sessionID)
	sessionExists, err := checkFileOrDirectory(filePath)
	checkError(err)
	if !sessionExists {
		return nil
	}

	sessionBytes, err := ioutil.ReadFile(filePath)
	checkError(err)
	dec := gob.NewDecoder(bytes.NewReader(sessionBytes))
	session := new(Session)
	err = dec.Decode(session)
	checkError(err)

	response.Payload = make([]interface{}, 1)
	response.Payload[0] = *session

	return nil
}

func (f *FSNode) SaveLog(request *FSRequest, ok *bool) error {
	_log := request.Payload[0].(Log)
	f.logger.Println("Saving log [" + _log.Job.JobID + "] to disk")

	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(_log)

	filePath := path.Join(LOG_DIR, _log.Job.JobID)
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0755)
	checkError(err)
	defer file.Close()
	err = file.Truncate(0)
	checkError(err)

	_, err = file.Write(buffer.Bytes())
	*ok = checkError(err) == nil

	file.Sync()

	return nil
}

func (f *FSNode) GetLog(request *FSRequest, response *FSResponse) error {
	jobID := request.Payload[0].(string)
	f.logger.Println("Retrieving log [" + jobID + "] from disk")
	filePath := path.Join(LOG_DIR, jobID)
	logExists, err := checkFileOrDirectory(filePath)
	checkError(err)
	if !logExists {
		return nil
	}

	logBytes, err := ioutil.ReadFile(filePath)
	checkError(err)
	dec := gob.NewDecoder(bytes.NewReader(logBytes))
	_log := new(Log)
	err = dec.Decode(_log)
	checkError(err)

	response.Payload = make([]interface{}, 1)
	response.Payload[0] = *_log

	return nil
}

// </RPC METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <HELPER METHODS>

func getNodeID() (nodeID string) {
	nodeIDExists, err := checkFileOrDirectory(NODE_ID_PATH)
	checkError(err)

	if nodeIDExists {
		id, err := ioutil.ReadFile(NODE_ID_PATH)
		checkError(err)
		nodeID = string(id)
	}

	return nodeID
}

func storeNodeID(nodeID string) {
	f, err := os.Create(NODE_ID_PATH)
	checkError(err)
	defer f.Close()

	data := []byte(nodeID)
	_, err = f.Write(data)
	checkError(err)

	f.Sync()
}

func checkFileOrDirectory(path string) (exists bool, err error) {
	_, err = os.Stat(path)
	if err == nil {
		exists = true
	} else if os.IsNotExist(err) {
		exists = false
		err = nil
	}

	return exists, err
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: go run fsnode.go [server ip:port]\n")
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
