package main

// Usage: go run fsnode.go [server ip:port]

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"
	"path"
	"sync"
	"time"

	. "../lib/session"
	. "../lib/types"

	"github.com/DistributedClocks/GoVector/govec"
)

const NODE_ID_PATH = "nodeID"
const HEARTBEAT_INTERVAL = 500

// Setting TEMP_MODE to true will turn off persistence for
// file server nodes. It will attempt to connect to the server as a
// new node every time, and will generate new folders for sessions
// and logs, allowing for multiple FSNodes to be run from the same
// directory.
//
const TEMP_MODE = true

type FSNode struct {
	logger     *log.Logger
	nodeAddr   string
	serverAddr string
	serverConn *rpc.Client
	id         string
	sessionDir string
	logDir     string
	golog      *govec.GoLog
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

	wg := &sync.WaitGroup{}
	wg.Add(1)
	wg.Wait()
}

////////////////////////////////////////////////////////////////////////////////////////////
// <PRIVATE METHODS>

func (f *FSNode) init() {
	f.logger = log.New(os.Stdout, "[Initializing] ", log.Lshortfile)
	f.serverAddr = os.Args[1]

	if !TEMP_MODE {
		f.sessionDir = "./session"
		f.logDir = "./log"
		f.createDirectories()
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
	if len(response.Payload) == 2 && response.Payload[0].(bool) {
		if response.Payload[1] != nil {
			nodeID = response.Payload[1].(string)
			storeNodeID(nodeID)
			f.logger.Println("Registered as new node")

			if TEMP_MODE {
				f.sessionDir = "./session_" + nodeID
				f.logDir = "./log_" + nodeID
				f.createDirectories()
			}
		} else {
			f.logger.Println("Registered as existing node")
		}

		f.serverConn = serverConn
		f.id = nodeID
		go f.heartbeat()

		f.logger.Println("Node [" + f.id + "] connected to server")
		f.golog = govec.InitGoVector("FSNode_" + f.id, "FSNode_" + f.id)
	} else {
		f.logger.Println("Rejected - failed to register with server")
		f.logger.Println("Are you using an old nodeID? If you restarted the server, don't forget to remove it so that you can be assigned a new one. Alternatively, turn on TEMP_MODE.")
		os.Exit(1)
	}
}

func (f *FSNode) heartbeat() {
	ignored := false
	for {
		f.serverConn.Call("Server.Heartbeat", f.id, &ignored)
		time.Sleep(time.Duration(HEARTBEAT_INTERVAL * time.Millisecond))
	}
}

func (f *FSNode) createDirectories() {
	exists, err := checkFileOrDirectory(f.sessionDir)
	checkError(err)
	if !exists {
		os.Mkdir(f.sessionDir, 0755)
	}

	exists, err = checkFileOrDirectory(f.logDir)
	checkError(err)
	if !exists {
		os.Mkdir(f.logDir, 0755)
	}
}

// </PRIVATE METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <RPC METHODS>

func (f *FSNode) SaveSession(request *FSRequest, ok *bool) (_ error) {
	session := request.Payload[0].(Session)
	logMsg := "Saving session [" + session.ID + "] to disk"

	f.logger.Println(logMsg)
	var ignored []byte
	f.golog.UnpackReceive(logMsg, request.Payload[1].([]byte), &ignored)

	*ok = false
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(session)

	filePath := path.Join(f.sessionDir, session.ID)
	file, err := openFile(filePath)
	if checkError(err) != nil {
		return
	}

	defer file.Close()
	err = file.Truncate(0)
	if checkError(err) != nil {
		return
	}

	_, err = file.Write(buffer.Bytes())
	if checkError(err) != nil {
		return
	}

	file.Sync()
	*ok = true
	logMsg = "Session [" + session.ID + "] saved"
	f.logger.Println(logMsg)
	f.golog.LogLocalEvent(logMsg)

	return
}

func (f *FSNode) GetSession(request *FSRequest, response *FSResponse) (_ error) {
	sessionID := request.Payload[0].(string)
	logMsg := "Retrieving session [" + sessionID + "] from disk"

	f.logger.Println(logMsg)
	var ignored []byte
	f.golog.UnpackReceive(logMsg, request.Payload[1].([]byte), &ignored)

	filePath := path.Join(f.sessionDir, sessionID)
	sessionExists, err := checkFileOrDirectory(filePath)
	if checkError(err) != nil || !sessionExists {
		return
	}

	sessionBytes, err := ioutil.ReadFile(filePath)
	if checkError(err) != nil {
		return
	}

	dec := gob.NewDecoder(bytes.NewReader(sessionBytes))
	session := new(Session)
	err = dec.Decode(session)
	if checkError(err) != nil {
		return
	}

	logMsg = "Sending session [" + sessionID + "] to server"
	f.logger.Println(logMsg)
	response.Payload = make([]interface{}, 2)
	response.Payload[0] = *session
	response.Payload[1] = f.golog.PrepareSend(logMsg, []byte{})

	return
}

func (f *FSNode) SaveLog(request *FSRequest, ok *bool) (_ error) {
	_log := request.Payload[0].(Log)
	logMsg := "Saving log [" + _log.Job.JobID + "] to disk"

	f.logger.Println(logMsg)
	var ignored []byte
	f.golog.UnpackReceive(logMsg, request.Payload[1].([]byte), &ignored)

	*ok = false
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(_log)

	filePath := path.Join(f.logDir, _log.Job.JobID)
	file, err := openFile(filePath)
	if checkError(err) != nil {
		return
	}
	defer file.Close()
	err = file.Truncate(0)
	if checkError(err) != nil {
		return
	}

	_, err = file.Write(buffer.Bytes())
	if checkError(err) != nil {
		return
	}

	file.Sync()
	*ok = true
	logMsg = "Log [" + _log.Job.JobID + "] saved"
	f.logger.Println(logMsg)
	f.golog.LogLocalEvent(logMsg)

	return
}

func (f *FSNode) GetLog(request *FSRequest, response *FSResponse) (_ error) {
	jobID := request.Payload[0].(string)
	logMsg := "Retrieving log [" + jobID + "] from disk"

	f.logger.Println(logMsg)
	var ignored []byte
	f.golog.UnpackReceive(logMsg, request.Payload[1].([]byte), &ignored)

	filePath := path.Join(f.logDir, jobID)
	logExists, err := checkFileOrDirectory(filePath)
	if checkError(err) != nil || !logExists {
		return
	}

	logBytes, err := ioutil.ReadFile(filePath)
	if checkError(err) != nil {
		return
	}
	dec := gob.NewDecoder(bytes.NewReader(logBytes))
	_log := new(Log)
	err = dec.Decode(_log)
	if checkError(err) != nil {
		return
	}

	logMsg = "Sending log [" + jobID + "] to server"
	f.logger.Println(logMsg)
	response.Payload = make([]interface{}, 2)
	response.Payload[0] = *_log
	response.Payload[1] = f.golog.PrepareSend(logMsg, []byte{})

	return
}

// </RPC METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <HELPER METHODS>

func getNodeID() (nodeID string) {
	if TEMP_MODE {
		return ""
	}

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
	if TEMP_MODE {
		return
	}

	f, err := openFile(NODE_ID_PATH)
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

func openFile(path string) (file *os.File, err error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0755)
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
