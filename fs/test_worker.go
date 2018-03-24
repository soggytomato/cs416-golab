package main

// Usage: go run test_worker.go [server ip:port]

import (
	"fmt"
	"net/rpc"
	"os"
	"time"

	. "../lib/types"
)

func main() {
	RegisterGob()

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
		SessionID: "element-0",
		ClientID: "client-0",
		ID:       "element-0",
		PrevID:   "",
		NextID:   "element-1",
		Text:     "a",
		Deleted:  false}
	session.CRDT["element-1"] = &Element{
		SessionID: "element-0",
		ClientID: "client-0",
		ID:       "element-1",
		PrevID:   "element-0",
		NextID:   "element-2",
		Text:     "b",
		Deleted:  false}
	session.CRDT["element-2"] = &Element{
		SessionID: "element-0",
		ClientID: "client-0",
		ID:       "element-2",
		PrevID:   "element-1",
		NextID:   "",
		Text:     "c",
		Deleted:  false}

	request := new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = session

	fmt.Println("Saving session:")
	fmt.Println(session)
	ignored := false
	err = serverConn.Call("Server.SaveSession", request, &ignored)
	checkError(err)
	fmt.Println("Session (probably) saved.")

	fmt.Println("Sleeping for 1000 ms...\n")
	time.Sleep(1000 * time.Millisecond)


	// Test get session

	fmt.Println("Getting session from file server...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = session.ID
	response := new(FSResponse)

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
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = _log

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
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = _log.Job.JobID
	response = new(FSResponse)

	err = serverConn.Call("Server.GetLog", request, response)
	checkError(err)
	if len(response.Payload) == 0 {
		fmt.Println("Failed to get log from file server.")
		return
	}
	newLog := response.Payload[0].(Log)
	fmt.Println("Got log from file server:")
	fmt.Println(newLog)

	fmt.Println("Sleeping for 1000 ms...\n")
	time.Sleep(1000 * time.Millisecond)


	// Save a bunch of logs

	log1 := Log{
		Job: Job{
			SessionID: "session-1",
			JobID: "job-1",
			Snippet: `fmt.Println("I love CPSC 416!")`,
			Done: true},
		Output: `I love CPSC 416!`}
	log2 := Log{
		Job: Job{
			SessionID: "session-2",
			JobID: "job-2",
			Snippet: `fmt.Println("Ayy lmao")`,
			Done: true},
		Output: `Ayy lmao`}
	log3 := Log{
		Job: Job{
			SessionID: "session-2",
			JobID: "job-3",
			Snippet: `fmt.Println("Those A2 marks tho")`,
			Done: true},
		Output: `Those A2 marks tho`}

	fmt.Println("Saving three logs (job-1, job-2, job-3)...")

	fmt.Println("Saving log (job-1)...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = log1
	ignored = false
	err = serverConn.Call("Server.SaveLog", request, &ignored)
	checkError(err)
	fmt.Println("Log (probably) saved.")

	fmt.Println("Saving log (job-2)...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = log2
	ignored = false
	err = serverConn.Call("Server.SaveLog", request, &ignored)
	checkError(err)
	fmt.Println("Log (probably) saved.")

	fmt.Println("Saving log (job-3)...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = log3
	ignored = false
	err = serverConn.Call("Server.SaveLog", request, &ignored)
	checkError(err)
	fmt.Println("Log (probably) saved.")

	fmt.Println("Sleeping for 1000 ms...\n")
	time.Sleep(1000 * time.Millisecond)


	// Get all logs in session-1

	fmt.Println("Getting logs from file server in session-1 (job-1)...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = log1.Job.SessionID
	response = new(FSResponse)

	err = serverConn.Call("Server.GetLogs", request, response)
	checkError(err)
	if len(response.Payload) == 0 {
		fmt.Println("Failed to get logs from file server.")
		return
	}
	logs := response.Payload[0].([]Log)
	fmt.Println("Got logs from file server:")
	fmt.Println(logs)

	fmt.Println("Sleeping for 1000 ms...\n")
	time.Sleep(1000 * time.Millisecond)


	// Get all logs in session-2

	fmt.Println("Getting logs from file server in session-2 (job-2, job-3)...")
	request = new(FSRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = log2.Job.SessionID
	response = new(FSResponse)

	err = serverConn.Call("Server.GetLogs", request, response)
	checkError(err)
	if len(response.Payload) == 0 {
		fmt.Println("Failed to get logs from file server.")
		return
	}
	logs = response.Payload[0].([]Log)
	fmt.Println("Got logs from file server:")
	fmt.Println(logs)
}

func checkError(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}