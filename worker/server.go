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
	Address         net.Addr
	RecentHeartbeat int64
}

type WorkerNetSettings struct {
	WorkerID                int `json:"workerID"`
	HeartBeat               int `json:"heartbeat"`
	MinNumWorkerConnections int `json:"min-num-worker-connections"`
}

type AllWorkers struct {
	sync.RWMutex
	all   map[int]*Worker
	queue []*Worker
}

var (
	unknownWorkerIDError UnknownWorkerIDError = errors.New("Load Balancer: unknown worker")
	errLog               *log.Logger          = log.New(os.Stderr, "[serv] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	outLog               *log.Logger          = log.New(os.Stderr, "[serv] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	// Workers in the system.
	allWorkers              AllWorkers = AllWorkers{all: make(map[int]*Worker), queue: make([]*Worker, 0)}
	HeartBeatInterval                  = 2000 // every two second
	MinNumWorkerConnections            = 2
	NumWorkerToReturn                  = 4
	WorkerIDCounter                    = 0
)

// Parses args, setups up RPC server.
func main() {
	gob.Register(&net.TCPAddr{})

	addrs, _ := net.InterfaceAddrs()
	var externalIP string
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				externalIP = ipnet.IP.String()
			}
		}
	}
	externalIP = externalIP + ":12345"
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
	Address net.Addr
}

// Function to delete dead worker (no recent heartbeat)
func monitor(workerID int, heartBeatInterval time.Duration) {
	for {
		allWorkers.Lock()
		if time.Now().UnixNano()-allWorkers.all[workerID].RecentHeartbeat > int64(heartBeatInterval) {
			outLog.Printf("%s timed out\n", allWorkers.all[workerID].Address.String())
			delete(allWorkers.all, workerID)
			for index, worker := range allWorkers.queue {
				if worker.WorkerID == workerID {
					allWorkers.queue = append(allWorkers.queue[:index], allWorkers.queue[index+1:]...)
				}
			}
			allWorkers.Unlock()
			return
		}
		outLog.Printf("%s is alive\n", allWorkers.all[workerID].Address.String())
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
		if worker.Address.Network() == w.Address.Network() && worker.Address.String() == w.Address.String() {
			return AddressAlreadyRegisteredError(w.Address.String())
		}
	}

	newWorkerID := WorkerIDCounter

	newWorker := &Worker{
		newWorkerID,
		w.Address,
		time.Now().UnixNano(),
	}

	allWorkers.all[newWorkerID] = newWorker

	go monitor(newWorkerID, time.Duration(HeartBeatInterval)*time.Millisecond)

	*r = WorkerNetSettings{
		newWorkerID,
		HeartBeatInterval,
		MinNumWorkerConnections,
	}

	outLog.Printf("Got Register from %s\n", w.Address.String())
	WorkerIDCounter++
	allWorkers.queue = append(allWorkers.queue, newWorker)
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

	if len(allWorkers.queue) == 0 {
		return nil
	}

	nextWorker := allWorkers.queue[0]
	allWorkers.queue = allWorkers.queue[1:]

	*retWorkerIP = nextWorker.Address.String()

	allWorkers.queue = append(allWorkers.queue, nextWorker)
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
		workerAddresses = append(workerAddresses, worker.Address)
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
func (s *LBServer) HeartBeat(workerID int, _ignored *bool) error {
	allWorkers.Lock()
	defer allWorkers.Unlock()

	if _, ok := allWorkers.all[workerID]; !ok {
		return unknownWorkerIDError
	}

	allWorkers.all[workerID].RecentHeartbeat = time.Now().UnixNano()

	return nil
}

func handleErrorFatal(msg string, e error) {
	if e != nil {
		errLog.Fatalf("%s, err = %s\n", msg, e.Error())
	}
}
