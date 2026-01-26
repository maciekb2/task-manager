package main

import (
	"sync"
)

// Client defines the interface for websocket clients
type Client interface {
	WriteJSON(v interface{}) error
	Close() error
}

type Event struct {
	Subject   string      `json:"subject"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

type Hub struct {
	clients    map[Client]bool
	broadcast  chan Event
	register   chan Client
	unregister chan Client
	lock       sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan Event),
		register:   make(chan Client),
		unregister: make(chan Client),
		clients:    make(map[Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.lock.Lock()
			h.clients[client] = true
			h.lock.Unlock()
		case client := <-h.unregister:
			h.lock.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.lock.Unlock()
		case message := <-h.broadcast:
			h.lock.Lock()
			for client := range h.clients {
				err := client.WriteJSON(message)
				if err != nil {
					client.Close()
					delete(h.clients, client)
				}
			}
			h.lock.Unlock()
		}
	}
}
