package cache

import (
	"time"

	. "../session"
)

const MAINTENANCE_INTERVAL int = 2
const EXPIRY_THRESHOLD int = 5 * MAINTENANCE_INTERVAL

type Cache struct {
	elements        map[string][]Element
	pendingSessions map[string]bool
}

////////////////////////////////////////////////////////////////////////////////////////////
// <PRIVATE METHODS>

func (c *Cache) remove(sessionID string, index int) {
	session := c.elements[sessionID]
	c.elements[sessionID] = append(session[:index], session[index+1:]...)
}

func (c *Cache) clean(sessionID string) {
	var numDeleted int = 0

	cachedElements := c.elements[sessionID]
	for i, element := range cachedElements {
		if int(time.Now().Unix()-element.Timestamp) < EXPIRY_THRESHOLD {
			break
		} else {
			i = i - numDeleted
			numDeleted++

			c.remove(sessionID, i)
		}
	}
}

func (c *Cache) exists(element Element) bool {
	sessionID := element.SessionID

	var exists bool = false
	for _, _element := range c.elements[sessionID] {
		if _element.ID == element.ID && _element.Deleted == element.Deleted {
			exists = true
			break
		}
	}

	return exists
}

// </PRIVATE METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <PUBLIC METHODS>

func (c *Cache) Init() {
	c.elements = make(map[string][]Element)
	c.pendingSessions = make(map[string]bool)
}

func (c *Cache) Maintain() {
	for {
		time.Sleep(time.Second * time.Duration(MAINTENANCE_INTERVAL))

		for sessionID := range c.elements {
			if pending, ok := c.pendingSessions[sessionID]; !ok || pending == false {
				go c.clean(sessionID)
			}
		}
	}
}

func (c *Cache) Add(element Element) {
	sessionID := element.SessionID

	if !c.exists(element) {
		element.Timestamp = time.Now().Unix()
		c.elements[sessionID] = append(c.elements[sessionID], element)
	}
}

func (c *Cache) Get(sessionID string) []Element {
	return c.elements[sessionID]
}

func (c *Cache) AddPending(sessionID string) {
	c.pendingSessions[sessionID] = true
}

func (c *Cache) RemovePending(sessionID string) {
	c.pendingSessions[sessionID] = false
}

// </PUBLIC METHODS>
////////////////////////////////////////////////////////////////////////////////////////////
