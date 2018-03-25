package types

import "encoding/gob"

type FSRequest struct {
	Payload []interface{}
}

type FSResponse struct {
	Payload []interface{}
}

type WorkerResponse struct {
	Error   error
	Payload []interface{}
}

type WorkerRequest struct {
	Payload []interface{}
}

type Session struct {
	ID   string
	CRDT map[string]*Element
	Head string
	Next int
}

type Element struct {
	SessionID string
	ClientID  string
	ID        string
	PrevID    string
	NextID    string
	Text      string
	Deleted   bool
}

type Log struct {
	Job    Job
	Output string
}

type Job struct {
	SessionID string
	JobID     string
	Snippet   string
	Done      bool
}

type WorkerNetSettings struct {
	WorkerID                int `json:"workerID"`
	HeartBeat               int `json:"heartbeat"`
	MinNumWorkerConnections int `json:"min-num-worker-connections"`
}

func RegisterGob() {
	gob.Register(Session{})
	gob.Register(Log{})
	gob.Register([]Log{})
}
