package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/maciekb2/task-manager/pkg/logger"
	"github.com/nats-io/nats.go"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Event struct {
	Subject   string      `json:"subject"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan Event
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	lock       sync.Mutex
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan Event),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		clients:    make(map[*websocket.Conn]bool),
	}
}

func (h *Hub) run() {
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

func main() {
	logger.Setup("visualizer")

	hub := newHub()
	go hub.run()

	// NATS Connection
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://nats:4222"
	}

	// Retry loop for NATS connection
	var nc *nats.Conn
	var err error
	for i := 0; i < 10; i++ {
		nc, err = nats.Connect(natsURL)
		if err == nil {
			break
		}
		slog.Info("Waiting for NATS...", "error", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		logger.Fatal("Could not connect to NATS", err)
	}
	defer nc.Close()
	slog.Info("Connected to NATS")

	subjects := []string{"tasks.>", "events.>"}

	for _, subj := range subjects {
		_, err := nc.Subscribe(subj, func(msg *nats.Msg) {
			var payload interface{}
			// Try to unmarshal, if it fails, send as string or ignore?
			// Most messages are JSON.
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				// If not JSON, send as string
				payload = string(msg.Data)
			}

			event := Event{
				Subject:   msg.Subject,
				Data:      payload,
				Timestamp: time.Now().UnixMilli(),
			}
			hub.broadcast <- event
		})
		if err != nil {
			logger.Error("Error subscribing", err, "subject", subj)
		}
	}

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("upgrade failed", "error", err)
			return
		}
		hub.register <- conn
	})

	srv := &http.Server{Addr: ":8085"}
	go func() {
		slog.Info("Visualizer started on :8085")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("visualizer server failed", err)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()

	slog.Info("Shutting down visualizer...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("visualizer shutdown error", err)
	}
	slog.Info("Visualizer stopped.")
}
