package main

import (
	"log"
)

type hub struct {
	connections map[*connection]bool
	register    chan *connection
	unregister  chan *connection
}

var h = hub{
	register:    make(chan *connection),
	unregister:  make(chan *connection),
	connections: make(map[*connection]bool),
}

func (h *hub) run() {
	for {
		select {
		case c := <-h.register:
			h.connections[c] = true
			log.Println("Register user:", c.String())
		case c := <-h.unregister:
			log.Println("Unregister user:", c.String())
			if _, ok := h.connections[c]; ok {
				delete(h.connections, c)
				close(c.send)
			}
		}
	}
}

