package session

import (
	"fmt"
	"strings"
	"sync"
)

// Flags whether to log the inputs and deletes
const LOG_ELEMENTS = false

// Since we are adding a character to the right of another character, we need
// a fake INITIAL_ID to use to place the first character in an empty message
const INITIAL_ID string = "12345"

type Session struct {
	ID   string
	CRDT map[string]*Element
	Head string
	Next int

	mux sync.RWMutex
}

type Element struct {
	SessionID string
	ClientID  string
	ID        string
	PrevID    string
	NextID    string
	Text      string
	Deleted   bool

	Timestamp int64
}

////////////////////////////////////////////////////////////////////////////////////////////
// <PRIVATE METHODS>

func (s *Session) exists(id string) bool {
	if s.CRDT[id] != nil || id == INITIAL_ID {
		return true
	} else {
		return false
	}
}

/*Checks if any other clients have made inserts to the same prevID. The algorithm
compares the prevElement's nextID to the incomingOp ID - if nextID is greater, incomingOp
will move further down the message until it is greater than the nextID
*/
func (s *Session) getPrev(element Element) *Element {
	id := element.ID
	prevID := element.PrevID
	prevElement := s.CRDT[prevID]

	if prevElement.NextID != "" && prevElement.NextID != id {
		nextID := prevElement.NextID
		for strings.Compare(nextID, id) == 1 && element.ClientID != s.CRDT[nextID].ClientID {
			prevElement = s.CRDT[nextID]
			nextID = prevElement.NextID
		}

		return s.CRDT[prevElement.ID]
	} else {
		return s.CRDT[prevID]
	}
}

func (s *Session) insert(element Element) {
	id := element.ID
	prevID := element.PrevID

	// Handle the case where the element does not
	// have a previous ID (ie. replacing head)
	if prevID == "" {
		if nextElem, ok := s.CRDT[s.Head]; ok {
			element.NextID = nextElem.ID
			nextElem.PrevID = id
		}

		s.Head = id
	} else {
		prevElement := s.getPrev(element)
		nextElement := s.CRDT[prevElement.NextID]

		if nextElement != nil {
			nextElement.PrevID = id
			element.NextID = nextElement.ID
		}

		prevElement.NextID = id
		element.PrevID = prevElement.ID
	}

	logElement(&element)

	s.CRDT[id] = &element
	s.Next++
}

func (s *Session) delete(element Element) bool {
	_element := s.CRDT[element.ID]
	if _element != nil && _element.Deleted == false {
		_element.Deleted = true

		logElement(_element)

		return true
	} else {
		return false
	}
}

// </PRIVATE METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <PUBLIC METHODS>

func (s *Session) Add(element Element) bool {
	s.mux.Lock()
	defer s.mux.Unlock()

	id := element.ID

	// If the element already exists don't insert
	if s.exists(id) {
		return false
	}

	s.insert(element)
	return true
}

func (s *Session) Delete(element Element) bool {
	s.mux.Lock()
	defer s.mux.Unlock()

	return s.delete(element)
}

// </PUBLIC METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <HELPER METHODS>

func logElement(element *Element) {
	if !LOG_ELEMENTS {
		return
	}

	if !element.Deleted {
		fmt.Println(
			"============INSERT===========\n",
			"SESSION: "+element.SessionID+"\n",
			"ID: "+element.ID+"\n",
			"PREV ID: "+element.PrevID+"\n",
			"NEXT ID: "+element.NextID+"\n",
			"TEXT: "+element.Text+"\n",
			"=============================")
	} else {
		fmt.Println(
			"============DELETE===========\n",
			"SESSION: "+element.SessionID+"\n",
			"ID: "+element.ID+"\n",
			"PREV ID: "+element.PrevID+"\n",
			"NEXT ID: "+element.NextID+"\n",
			"TEXT: "+element.Text+"\n",
			"=============================")
	}
}

// </HELPER METHODS>
////////////////////////////////////////////////////////////////////////////////////////////
