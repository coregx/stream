package sse

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"sync"
)

// Common errors returned by Hub.
var (
	// ErrHubClosed is returned when attempting to use a closed hub.
	ErrHubClosed = errors.New("sse: hub closed")
)

// Hub manages broadcasting events to multiple SSE connections.
//
// Hub[T] is a generic type that manages a pool of SSE connections and enables
// efficient broadcasting of typed events to all connected clients. It handles
// client registration, unregistration, and automatic cleanup of failed connections.
//
// Example:
//
//	hub := sse.NewHub[string]()
//	go hub.Run()
//	defer hub.Close()
//
//	// Register connections
//	hub.Register(conn1)
//	hub.Register(conn2)
//
//	// Broadcast to all clients
//	hub.Broadcast("Hello, everyone!")
//
// The Hub uses channels for thread-safe coordination and a select loop in Run()
// to handle concurrent registration, unregistration, and broadcasting operations.
type Hub[T any] struct {
	// clients is the set of active connections.
	clients map[*Conn]bool

	// broadcast channel receives events to broadcast to all clients.
	broadcast chan T

	// register channel receives new client connections.
	register chan *Conn

	// unregister channel receives clients to disconnect.
	unregister chan *Conn

	// done channel signals hub shutdown.
	done chan struct{}

	// mu protects clients map during read operations.
	mu sync.RWMutex

	// closed indicates if the hub is shut down.
	closed bool
}

// NewHub creates a new Hub for broadcasting events of type T.
//
// The returned Hub must be started by calling Run() in a goroutine before use.
// Always call Close() when done to properly shut down the hub.
//
// Example:
//
//	hub := sse.NewHub[UserEvent]()
//	go hub.Run()
//	defer hub.Close()
func NewHub[T any]() *Hub[T] {
	return &Hub[T]{
		clients:    make(map[*Conn]bool),
		broadcast:  make(chan T, 256), // Buffered for burst traffic
		register:   make(chan *Conn, 16),
		unregister: make(chan *Conn, 16),
		done:       make(chan struct{}),
		closed:     false,
	}
}

// Run starts the hub's event loop.
//
// Run processes client registration, unregistration, and broadcast operations.
// It should be called in a goroutine and will block until Close() is called.
//
// Example:
//
//	hub := sse.NewHub[string]()
//	go hub.Run()
func (h *Hub[T]) Run() {
	for {
		select {
		case client := <-h.register:
			h.handleRegister(client)

		case client := <-h.unregister:
			h.handleUnregister(client)

		case data := <-h.broadcast:
			h.handleBroadcast(data)

		case <-h.done:
			return
		}
	}
}

// handleRegister adds a new client to the hub.
func (h *Hub[T]) handleRegister(client *Conn) {
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()
}

// handleUnregister removes a client from the hub.
func (h *Hub[T]) handleUnregister(client *Conn) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		_ = client.Close()
	}
	h.mu.Unlock()
}

// handleBroadcast sends data to all connected clients.
func (h *Hub[T]) handleBroadcast(data T) {
	// Get snapshot of clients
	h.mu.RLock()
	clients := make([]*Conn, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	// Convert data to string
	dataStr := h.convertToString(data)
	if dataStr == "" {
		return
	}

	// Send to all clients (outside lock to avoid blocking)
	for _, client := range clients {
		if err := client.SendData(dataStr); err != nil {
			h.removeClient(client)
		}
	}
}

// convertToString converts T to string for sending.
func (h *Hub[T]) convertToString(data T) string {
	switch v := any(data).(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		// Try JSON encoding
		jsonData, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(jsonData)
	}
}

// removeClient removes a failed client from the hub.
func (h *Hub[T]) removeClient(client *Conn) {
	h.mu.Lock()
	delete(h.clients, client)
	_ = client.Close()
	h.mu.Unlock()
}

// Register adds a connection to the hub.
//
// The connection will receive all future broadcasts until it's unregistered
// or fails to send.
//
// Returns ErrHubClosed if the hub is already closed.
//
// Example:
//
//	conn, err := sse.Upgrade(w, r)
//	if err != nil {
//	    return err
//	}
//	err = hub.Register(conn)
func (h *Hub[T]) Register(conn *Conn) error {
	h.mu.RLock()
	closed := h.closed
	h.mu.RUnlock()

	if closed {
		return ErrHubClosed
	}

	h.register <- conn
	return nil
}

// Unregister removes a connection from the hub.
//
// The connection will be closed and removed from the broadcast list.
// It's safe to call Unregister multiple times for the same connection.
//
// Returns ErrHubClosed if the hub is already closed.
//
// Example:
//
//	err := hub.Unregister(conn)
func (h *Hub[T]) Unregister(conn *Conn) error {
	h.mu.RLock()
	closed := h.closed
	h.mu.RUnlock()

	if closed {
		return ErrHubClosed
	}

	h.unregister <- conn
	return nil
}

// Broadcast sends data to all connected clients.
//
// The data will be converted to a string representation:
//   - string: sent as-is
//   - fmt.Stringer: String() method called
//   - other types: JSON-encoded
//
// Failed sends automatically remove the client from the hub.
//
// Returns ErrHubClosed if the hub is already closed.
//
// Example:
//
//	err := hub.Broadcast("Server restarting in 5 minutes")
func (h *Hub[T]) Broadcast(data T) error {
	h.mu.RLock()
	closed := h.closed
	h.mu.RUnlock()

	if closed {
		return ErrHubClosed
	}

	h.broadcast <- data
	return nil
}

// BroadcastJSON sends a JSON-encoded value to all connected clients.
//
// This is a convenience method for sending structured data.
// The value is marshaled to JSON and sent to all clients.
//
// Returns an error if JSON marshaling fails or if the hub is closed.
//
// Example:
//
//	event := UserEvent{ID: 1, Action: "login"}
//	err := hub.BroadcastJSON(event)
func (h *Hub[T]) BroadcastJSON(v any) error {
	h.mu.RLock()
	closed := h.closed
	h.mu.RUnlock()

	if closed {
		return ErrHubClosed
	}

	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("sse: failed to marshal JSON: %w", err)
	}

	// Try to convert to T
	var t T
	switch any(t).(type) {
	case string:
		// T is string, convert directly
		return h.Broadcast(any(string(data)).(T))
	default:
		// For other types, try to unmarshal into T
		if err := json.Unmarshal(data, &t); err != nil {
			// If unmarshal fails, try broadcasting the raw value
			if typed, ok := v.(T); ok {
				return h.Broadcast(typed)
			}
			return fmt.Errorf("sse: cannot convert to hub type: %w", err)
		}
		return h.Broadcast(t)
	}
}

// Clients returns the number of currently connected clients.
//
// This is safe to call concurrently with other Hub operations.
//
// Example:
//
//	count := hub.Clients()
//	log.Printf("Active connections: %d", count)
func (h *Hub[T]) Clients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Close shuts down the hub and closes all client connections.
//
// After Close, all operations on the hub will return ErrHubClosed.
// It's safe to call Close multiple times.
//
// Example:
//
//	defer hub.Close()
func (h *Hub[T]) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil
	}

	h.closed = true
	close(h.done)

	// Close all client connections
	for client := range h.clients {
		_ = client.Close()
	}
	h.clients = make(map[*Conn]bool)

	return nil
}
