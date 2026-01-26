package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/maciekb2/task-manager/pkg/logger"
	"github.com/nats-io/nats.go"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	logger.Setup("visualizer")

	hub := NewHub()
	go hub.Run()

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
	consumer := NewConsumer(hub)

	for _, subj := range subjects {
		_, err := nc.Subscribe(subj, consumer.Consume)
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
