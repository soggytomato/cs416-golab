package util

import (
	"time"

	. "../types"
)

const MAINTENANCE_INTERVAL int = 2
const EXPIRY_THRESHOLD int = 5 * MAINTENANCE_INTERVAL

type Cache struct {
	elements map[string][]*Element
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

// </PRIVATE METHODS>
////////////////////////////////////////////////////////////////////////////////////////////

//

////////////////////////////////////////////////////////////////////////////////////////////
// <PUBLIC METHODS>

func (c *Cache) Init() {
	c.elements = make(map[string][]*Element)
}

func (c *Cache) Maintain() {
	for {
		time.Sleep(time.Second * time.Duration(MAINTENANCE_INTERVAL))

		for sessionID := range c.elements {
			go c.clean(sessionID)
		}
	}
}

func (c *Cache) Add(element *Element) {
	sessionID := element.SessionID

	element.Timestamp = time.Now().Unix()
	c.elements[sessionID] = append(c.elements[sessionID], element)
}

func (c *Cache) Get(sessionID string) []*Element {
	return c.elements[sessionID]
}

// </PUBLIC METHODS>
////////////////////////////////////////////////////////////////////////////////////////////
