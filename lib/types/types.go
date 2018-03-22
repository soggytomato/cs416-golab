package types

import "encoding/gob"

type FSRequest struct {
	Payload []interface{}
}

type FSResponse struct {
	Payload []interface{}
}

type Session struct {
	ID       string
	Elements []Element
	Head     string
}

type Element struct {
	SessionID string
	ClientID  string
	ID        string
	NextID    string
	PrevID    string
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

func RegisterGob() {
	gob.Register(Session{})
	gob.Register(Log{})
	gob.Register([]Log{})
}
