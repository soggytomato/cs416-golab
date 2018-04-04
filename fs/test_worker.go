package main

// Usage: go run test_worker.go [server ip:port]

import (
	"fmt"
	"net/rpc"
	"os"
	"time"

	. "../lib/session"
	. "../lib/types"

	"github.com/DistributedClocks/GoVector/govec"
)

func main() {
	RegisterGob()

	golog := govec.InitGoVector("TestWorker", "TestWorker")

	// Connect to file server

	fmt.Println("Connecting to file server...")
	serverConn, err := rpc.Dial("tcp", os.Args[1])
	if checkError(err) != nil {
		return
	}
	fmt.Println("Connected to file server.")


	// Test save session

	session := Session{
		ID: "session-0",
		CRDT: make(map[string]*Element),
		Head: "element-0",
		Next: 4}
	session.CRDT["element-0"] = &Element{
		SessionID: "session-0",
		ClientID: "client-0",
		ID:       "element-0",
		PrevID:   "",
		NextID:   "element-1",
		Text:     "a",
		Deleted:  false}
	session.CRDT["element-1"] = &Element{
		SessionID: "session-0",
		ClientID: "client-0",
		ID:       "element-1",
		PrevID:   "element-0",
		NextID:   "element-2",
		Text:     "b",
		Deleted:  false}
	session.CRDT["element-2"] = &Element{
		SessionID: "session-0",
		ClientID: "client-0",
		ID:       "element-2",
		PrevID:   "element-1",
		NextID:   "",
		Text:     "c",
		Deleted:  false}

	request := new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = session
	request.Payload[1] = golog.PrepareSend("Saving session", []byte{})

	fmt.Println("Saving session:")
	fmt.Println(session)
	ignored := false
	err = serverConn.Call("Server.SaveSession", request, &ignored)
	checkError(err)
	fmt.Println("Session (probably) saved.")

	fmt.Println("Sleeping for 1000 ms...\n")
	time.Sleep(1000 * time.Millisecond)


	// Test save log

	_log := Log{
		Job: Job{
			SessionID: "session-0",
			JobID: "job-0",
			Snippet: `fmt.Println("Hello World!")`,
			Done: true},
		Output: `Hello World!`}

	request = new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = _log
	request.Payload[1] = golog.PrepareSend("Saving log", []byte{})

	fmt.Println("Saving log...")
	ignored = false
	err = serverConn.Call("Server.SaveLog", request, &ignored)
	checkError(err)
	fmt.Println("Log (probably) saved.")

	fmt.Println("Sleeping for 1000 ms...\n")
	time.Sleep(1000 * time.Millisecond)


	// Test get log

	fmt.Println("Getting log from file server...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = _log.Job.JobID
	request.Payload[1] = golog.PrepareSend("Getting log", []byte{})
	response := new(FSResponse)

	err = serverConn.Call("Server.GetLog", request, response)
	checkError(err)
	if len(response.Payload) == 0 {
		fmt.Println("Failed to get log from file server.")
		return
	}
	newLog := response.Payload[0].(Log)
	var recbuf []byte
	golog.UnpackReceive("Got log", response.Payload[1].([]byte), &recbuf)

	fmt.Println("Got log from file server:")
	fmt.Println(newLog)

	fmt.Println("Sleeping for 1000 ms...\n")
	time.Sleep(1000 * time.Millisecond)


	// Save a bunch of logs

	log1 := Log{
		Job: Job{
			SessionID: "session-0",
			JobID: "job-1",
			Snippet: `fmt.Println("I love CPSC 416!")`,
			Done: true},
		Output: `I love CPSC 416!`}
	log2 := Log{
		Job: Job{
			SessionID: "session-1",
			JobID: "job-2",
			Snippet: `fmt.Println("Ayy lmao")`,
			Done: true},
		Output: `Ayy lmao`}
	log3 := Log{
		Job: Job{
			SessionID: "session-0",
			JobID: "job-3",
			Snippet: `fmt.Println("Those A2 marks tho")`,
			Done: true},
		Output: `Those A2 marks tho`}

	fmt.Println("Saving three logs (job-1, job-2, job-3)...")

	fmt.Println("Saving log (job-1)...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = log1
	request.Payload[1] = golog.PrepareSend("Saving log", []byte{})
	ignored = false
	err = serverConn.Call("Server.SaveLog", request, &ignored)
	checkError(err)
	fmt.Println("Log (probably) saved.")

	fmt.Println("Saving log (job-2)...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = log2
	request.Payload[1] = golog.PrepareSend("Saving log", []byte{})
	ignored = false
	err = serverConn.Call("Server.SaveLog", request, &ignored)
	checkError(err)
	fmt.Println("Log (probably) saved.")

	fmt.Println("Saving log (job-3)...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = log3
	request.Payload[1] = golog.PrepareSend("Saving log", []byte{})
	ignored = false
	err = serverConn.Call("Server.SaveLog", request, &ignored)
	checkError(err)
	fmt.Println("Log (probably) saved.")

	fmt.Println("Sleeping for 1000 ms...\n")
	time.Sleep(1000 * time.Millisecond)


	// Test get session (session-1)

	fmt.Println("Getting session from file server...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 2)
	request.Payload[0] = session.ID
	request.Payload[1] = golog.PrepareSend("Getting session", []byte{})
	response = new(FSResponse)

	err = serverConn.Call("Server.GetSession", request, response)
	checkError(err)
	if len(response.Payload) == 0 {
		fmt.Println("Failed to get session from file server.")
		return
	}
	newSession := response.Payload[0].(Session)
	fmt.Println("Got session from file server:")
	fmt.Println(newSession)
	fmt.Println("First element: " + fmt.Sprint(*newSession.CRDT["element-0"]))
	fmt.Println("Second element: " + fmt.Sprint(*newSession.CRDT["element-1"]))
	fmt.Println("Third element: " + fmt.Sprint(*newSession.CRDT["element-2"]))

	newLogs := response.Payload[1].([]Log)
	fmt.Println("Got logs for the session:")
	fmt.Println(newLogs)

	golog.UnpackReceive("Got session", response.Payload[2].([]byte), &recbuf)
}

func checkError(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}