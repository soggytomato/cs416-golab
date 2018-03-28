package session

import (
	"fmt"
	"strings"
)

// Since we are adding a character to the right of another character, we need
// a fake INITIAL_ID to use to place the first character in an empty message
const INITIAL_ID string = "12345"

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

	Timestamp int64
}

////////////////////////////////////////////////////////////////////////////////////////////
// <PRIVATE METHODS>

/*Checks if any other clients have made inserts to the same prevID. The algorithm
compares the prevElement's nextID to the incomingOp ID - if nextID is greater, incomingOp
will move further down the message until it is greater than the nextID
*/
func (s *Session) getPrev(newElement *Element) *Element {
	id := newElement.ID
	prevID := newElement.PrevID
	prevElement := s.CRDT[prevID]

	if prevElement.NextID != "" {
		nextID := prevElement.NextID
		for strings.Compare(nextID, id) == 1 && newElement.ClientID != s.CRDT[nextID].ClientID {
			prevElement = s.CRDT[nextID]
			nextID = prevElement.NextID
		}

		return s.CRDT[prevElement.ID]
	} else {
		return s.CRDT[prevID]
	}
}

func (s *Session) exists(id string) bool {
	if _, ok := s.CRDT[id]; ok || id == INITIAL_ID {
		return true
	} else {
		return false
	}
}

func (s *Session) insert(element *Element) {
	fmt.Println("Inserting element: ", element)

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

		if nextElement, ok := s.CRDT[prevElement.NextID]; ok {
			nextElement.PrevID = id
			element.NextID = nextElement.ID
		}

		prevElement.NextID = id
		element.PrevID = prevElement.ID
	}

	s.CRDT[id] = element
	s.Next++
}

func (s *Session) delete(element *Element) {
	if _element, ok := s.CRDT[element.ID]; ok {
		_element.Deleted = true
	}
}

// </PRIVATE METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <PUBLIC METHODS>

func (s *Session) Add(element *Element) bool {
	id := element.ID

	// If the element already exists or is deleted, dont insert
	if s.exists(id) || (s.exists(id) && s.CRDT[id].Deleted == true) {
		return false
	}

	s.insert(element)
	return true
}

func (s *Session) Delete(element *Element) error {
	s.delete(element)

	return nil
}

// </PUBLIC METHODS>
////////////////////////////////////////////////////////////////////////////////////////////
