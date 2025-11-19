// Package main demonstrates basic Server-Sent Events (SSE) usage.
//
// This example shows:
//   - Creating an SSE endpoint
//   - Sending time updates every second
//   - Proper context handling
//   - Graceful shutdown on Ctrl+C
//
// Run with: go run main.go
// Test with: curl http://localhost:8080/events
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coregx/stream/sse"
)

func main() {
	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/events", handleSSE)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Start server in background
	go func() {
		slog.Info("Starting SSE server", "addr", server.Addr)
		slog.Info("Connect with: curl http://localhost:8080/events")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	slog.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("Server stopped")
}

// handleSSE handles Server-Sent Events connections.
//
// This endpoint sends the current time to the client every second
// using SSE. The connection stays open until the client disconnects
// or the context is canceled.
func handleSSE(w http.ResponseWriter, r *http.Request) {
	// Upgrade to SSE connection
	conn, err := sse.Upgrade(w, r)
	if err != nil {
		slog.Error("Failed to upgrade to SSE", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	slog.Info("Client connected", "remote", r.RemoteAddr)

	// Send events every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Counter for event IDs
	eventID := 0

	for {
		select {
		case <-ticker.C:
			// Create time event with ID
			eventID++
			now := time.Now().Format(time.RFC3339)
			event := sse.NewEvent(now).
				WithType("time").
				WithID(fmt.Sprintf("evt-%d", eventID))

			// Send event
			if err := conn.Send(event); err != nil {
				slog.Error("Failed to send event", "error", err)
				return
			}

			slog.Info("Sent event", "id", eventID, "time", now)

		case <-conn.Done():
			// Connection closed
			slog.Info("Client disconnected", "remote", r.RemoteAddr)
			return
		}
	}
}
