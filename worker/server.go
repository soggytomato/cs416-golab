/*

Default Port is 12345

Usage:

$ go run server.go

*/

package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"sort"
	"sync"
	"time"

	. "../lib/types"
)

// Errors that the server could return.
type AddressAlreadyRegisteredError string

func (e AddressAlreadyRegisteredError) Error() string {
	return fmt.Sprintf("Load Balancer: address already registered [%s]", string(e))
}

type UnknownWorkerIDError error

type LBServer int

type Worker struct {
	WorkerID        int
	RPCAddress      net.Addr
	HTTPAddress     net.Addr
	RecentHeartbeat int64
	NumClients      int
}

type AllWorkers struct {
	sync.RWMutex
	all map[int]*Worker
}

var (
	unknownWorkerIDError UnknownWorkerIDError = errors.New("Load Balancer: unknown worker")
	errLog               *log.Logger          = log.New(os.Stderr, "[serv] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	outLog               *log.Logger          = log.New(os.Stderr, "[serv] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	// Workers in the system.
	allWorkers              AllWorkers = AllWorkers{all: make(map[int]*Worker)}
	HeartBeatInterval                  = 2000 // every two second
	MinNumWorkerConnections            = 2
	NumWorkerToReturn                  = 4
	WorkerIDCounter                    = 0
	sessionIDs                         = make(map[string]bool)
)

// Parses args, setups up RPC server.
func main() {
	gob.Register(&net.TCPAddr{})
	RegisterGob()

	if len(os.Args) != 2 {
		usage()
	}

	addrs, _ := net.InterfaceAddrs()
	var externalIP string
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				externalIP = ipnet.IP.String()
			}
		}
	}
	args := os.Args[1:]
	externalIP = externalIP + ":" + args[0]
	tcpAddr, _ := net.ResolveTCPAddr("tcp", externalIP)

	rand.Seed(time.Now().UnixNano())

	lbserver := new(LBServer)

	server := rpc.NewServer()
	server.Register(lbserver)

	l, e := net.ListenTCP("tcp", tcpAddr)

	handleErrorFatal("listen error", e)
	outLog.Printf("Server started. Receiving on %s\n", l.Addr().String())

	for {
		conn, _ := l.Accept()
		go server.ServeConn(conn)
	}
}

type WorkerInfo struct {
	RPCAddress  net.Addr
	HTTPAddress net.Addr
}

// Function to delete dead worker (no recent heartbeat)
func monitor(workerID int, heartBeatInterval time.Duration) {
	for {
		allWorkers.Lock()
		if time.Now().UnixNano()-allWorkers.all[workerID].RecentHeartbeat > int64(heartBeatInterval) {
			outLog.Printf("%s timed out\n", allWorkers.all[workerID].RPCAddress.String())
			delete(allWorkers.all, workerID)
			allWorkers.Unlock()
			return
		}
		outLog.Printf("%s is alive with %d clients\n", allWorkers.all[workerID].RPCAddress.String(), allWorkers.all[workerID].NumClients)
		allWorkers.Unlock()
		time.Sleep(heartBeatInterval)
	}
}

// Registers a new worker with an address for other worker to use to
// connect to it (returned in GetNodes call below), and an
// id for this worker. Returns error, or if error is not set,
// then settings for the worker node.
//
// Returns:
// - AddressAlreadyRegisteredError if the server has already registered this address.
func (s *LBServer) RegisterNewWorker(w WorkerInfo, r *WorkerNetSettings) error {
	allWorkers.Lock()
	defer allWorkers.Unlock()

	// fmt.Println(m.Address)

	for _, worker := range allWorkers.all {
		if worker.RPCAddress.Network() == w.RPCAddress.Network() && worker.RPCAddress.String() == w.RPCAddress.String() {
			return AddressAlreadyRegisteredError(w.RPCAddress.String())
		}
	}

	newWorkerID := WorkerIDCounter

	newWorker := &Worker{
		newWorkerID,
		w.RPCAddress,
		w.HTTPAddress,
		time.Now().UnixNano(),
		0,
	}

	allWorkers.all[newWorkerID] = newWorker

	go monitor(newWorkerID, time.Duration(HeartBeatInterval)*time.Millisecond)

	*r = WorkerNetSettings{
		newWorkerID,
		HeartBeatInterval,
		MinNumWorkerConnections,
	}

	outLog.Printf("Got Register from %s\n", w.RPCAddress.String())
	WorkerIDCounter++
	return nil
}

// Registers a new worker with an address for other worker to use to
// connect to it (returned in GetNodes call below), and an
// id for this worker. Returns error, or if error is not set,
// then settings for the worker node.
//
// Returns:
// - AddressAlreadyRegisteredError if the server has already registered this address.
func (s *LBServer) RegisterNewClient(sessID string, retWorkerIP *string) error {

	allWorkers.Lock()
	defer allWorkers.Unlock()

	if len(allWorkers.all) == 0 {
		return nil
	}

	workersAvailable := make(map[int]*Worker)
	for k, v := range allWorkers.all {
		workersAvailable[k] = v
	}
	for {
		var nextWorker *Worker
		for _, worker := range workersAvailable {
			if nextWorker == nil {
				nextWorker = worker
			} else if worker.NumClients < nextWorker.NumClients {
				nextWorker = worker
			}
		}
		fmt.Println("Next worker: ", nextWorker)
		fmt.Println("allWorkers[workerID]", allWorkers.all[nextWorker.WorkerID])
		allWorkers.all[nextWorker.WorkerID].NumClients++

		workerCon, err := rpc.Dial("tcp", nextWorker.RPCAddress.String())
		defer workerCon.Close()
		if err != nil {
			fmt.Println(err)
			fmt.Println("Error connecting to worker %s while registering", nextWorker.RPCAddress.String())
			delete(workersAvailable, nextWorker.WorkerID)
		} else {
			var ignored bool
			var workerErr error
			if sessionIDs[sessID] == false {
				sessionIDs[sessID] = true
				workerErr = workerCon.Call("Worker.CreateNewSession", sessID, &ignored)
				if err != nil {
					fmt.Println("Error connecting to worker while calling CreateNewSession")
				}
			} else {
				sessionIDs[sessID] = true
				workerErr = workerCon.Call("Worker.LoadSession", sessID, &ignored)
				if err != nil {
					fmt.Println("Error connecting to worker while calling LoadSession")
				}
			}
			if workerErr == nil {
				fmt.Println("Your worker is: ", nextWorker.HTTPAddress.String())
				*retWorkerIP = nextWorker.HTTPAddress.String()
				break
			}
		}
	}
	return nil
}

type Addresses []net.Addr

func (a Addresses) Len() int           { return len(a) }
func (a Addresses) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Addresses) Less(i, j int) bool { return a[i].String() < a[j].String() }

// Returns addresses for a subset of workers in the system.
//
// Returns:
// - UnknownKeyError if the server does not know a worker with this id.
func (s *LBServer) GetNodes(workerID int, addrSet *[]net.Addr) error {

	allWorkers.RLock()
	defer allWorkers.RUnlock()

	if _, ok := allWorkers.all[workerID]; !ok {
		return unknownWorkerIDError
	}

	workerAddresses := make([]net.Addr, 0, len(allWorkers.all)-1)

	for id, worker := range allWorkers.all {
		if workerID == id {
			continue
		}
		workerAddresses = append(workerAddresses, worker.RPCAddress)
	}

	sort.Sort(Addresses(workerAddresses))

	deterministicRandomNumber := int64(workerID) % 32
	r := rand.New(rand.NewSource(deterministicRandomNumber))
	for n := len(workerAddresses); n > 0; n-- {
		randIndex := r.Intn(n)
		workerAddresses[n-1], workerAddresses[randIndex] = workerAddresses[randIndex], workerAddresses[n-1]
	}

	n := len(workerAddresses)
	if NumWorkerToReturn < n {
		n = NumWorkerToReturn
	}
	*addrSet = workerAddresses[:n]

	return nil
}

// The server also listens for heartbeats from known workers. A worker must
// send a heartbeat to the server every HeartBeat milliseconds
// (specified in settings from server) after calling Register, otherwise
// the server will stop returning this worker's address/key to other
// workers.
//
// Returns:
// - UnknownKeyError if the server does not know a worker with this id.
func (s *LBServer) HeartBeat(request WorkerRequest, _ignored *bool) error {
	allWorkers.Lock()
	defer allWorkers.Unlock()
	workerID := request.Payload[0].(int)
	numClient := request.Payload[1].(int)

	if _, ok := allWorkers.all[workerID]; !ok {
		return unknownWorkerIDError
	}

	allWorkers.all[workerID].RecentHeartbeat = time.Now().UnixNano()
	allWorkers.all[workerID].NumClients = numClient

	return nil
}

// This function is called when a worker receives a run request by their client
// The worker will save the job
func (s *LBServer) NewJob(jobID string, _ignored *bool) error {
	allWorkers.Lock()
	defer allWorkers.Unlock()

	if len(allWorkers.all) == 0 {
		return nil
	}

	response := new(WorkerResponse)
	request := new(WorkerRequest)
	workersAvailable := make(map[int]*Worker)
	for k, v := range allWorkers.all {
		workersAvailable[k] = v
	}
	for {
		var nextWorker *Worker
		for _, worker := range workersAvailable {
			if nextWorker == nil {
				nextWorker = worker
			} else if worker.NumClients < nextWorker.NumClients {
				nextWorker = worker
			}
		}
		nextWorkerIP := nextWorker.RPCAddress.String()

		workerCon, _ := rpc.Dial("tcp", nextWorkerIP)
		defer workerCon.Close()
		request.Payload = make([]interface{}, 1)
		request.Payload[0] = jobID
		err := workerCon.Call("Worker.RunJob", request, response)
		if err == nil && response.Payload[0] != nil {
			break
		}
		delete(workersAvailable, nextWorker.WorkerID)
	}

	log := response.Payload[0].(Log)
	// Send out the new log
	request = new(WorkerRequest)
	request.Payload = make([]interface{}, 1)
	request.Payload[0] = log
	var ignored bool

	fmt.Println(allWorkers.all)
	for _, worker := range allWorkers.all {
		workerCon, err := rpc.Dial("tcp", worker.RPCAddress.String())
		if err == nil {
			workerCon.Call("Worker.SendLog", request, &ignored)
		}
	}

	return nil
}

func handleErrorFatal(msg string, e error) {
	if e != nil {
		errLog.Fatalf("%s, err = %s\n", msg, e.Error())
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: go run server.go [port]\n")
	os.Exit(1)
}
