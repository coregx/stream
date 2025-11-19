package websocket

import (
	"encoding/json/v2"
	"sync"
)

// Hub manages multiple WebSocket connections for broadcasting.
//
// Hub provides a central point for managing WebSocket clients and
// broadcasting messages to all connected clients simultaneously.
//
// Thread-safe operations allow concurrent client registration,
// unregistration, and broadcasting from multiple goroutines.
//
// Example Usage:
//
//	hub := websocket.NewHub()
//	go hub.Run()
//	defer hub.Close()
//
//	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
//	    conn, _ := websocket.Upgrade(w, r, nil)
//	    hub.Register(conn)
//
//	    go func() {
//	        defer hub.Unregister(conn)
//	        for {
//	            _, data, err := conn.Read()
//	            if err != nil {
//	                break
//	            }
//	            hub.Broadcast(data)
//	        }
//	    }()
//	})
type Hub struct {
	// Client management
	clients map[*Conn]bool // Registered clients

	// Channels for event loop
	register   chan *Conn  // Register new client
	unregister chan *Conn  // Unregister client
	broadcast  chan []byte // Broadcast message to all

	// Lifecycle management
	done   chan struct{}  // Shutdown signal
	closed bool           // Track if hub is closed
	wg     sync.WaitGroup // Wait for goroutines

	// Thread-safety for clients map and closed flag
	mu sync.RWMutex
}

// NewHub creates a new WebSocket Hub.
//
// The Hub must be started by calling Run() in a goroutine:
//
//	hub := websocket.NewHub()
//	go hub.Run()
//	defer hub.Close()
//
// Returns a ready-to-use Hub with initialized channels.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Conn]bool),
		register:   make(chan *Conn),
		unregister: make(chan *Conn),
		broadcast:  make(chan []byte, 256), // Buffered for performance
		done:       make(chan struct{}),
	}
}

// Run starts the Hub's event loop.
//
// This method blocks and should be called in a goroutine:
//
//	go hub.Run()
//
// The event loop handles:
//   - Client registration/unregistration
//   - Message broadcasting to all clients
//   - Graceful shutdown
//
// Run exits when Close() is called.
func (h *Hub) Run() {
	h.wg.Add(1)
	defer h.wg.Done()

	for {
		select {
		case client := <-h.register:
			// Register new client
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			// Unregister client
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				_ = client.Close() // Close connection
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			// Broadcast to all clients
			h.mu.RLock()
			for client := range h.clients {
				// Send in goroutine to avoid blocking on slow clients
				go func(c *Conn, msg []byte) {
					if err := c.Write(BinaryMessage, msg); err != nil {
						// Auto-unregister on write failure
						h.Unregister(c)
					}
				}(client, message)
			}
			h.mu.RUnlock()

		case <-h.done:
			// Shutdown
			return
		}
	}
}

// Register adds a client to the Hub.
//
// The client will receive all messages sent via Broadcast().
//
// Typically called after successful WebSocket upgrade:
//
//	conn, _ := websocket.Upgrade(w, r, nil)
//	hub.Register(conn)
//
// Thread-safe: can be called from multiple goroutines.
func (h *Hub) Register(client *Conn) {
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return
	}
	h.mu.RUnlock()

	h.register <- client
}

// Unregister removes a client from the Hub.
//
// The client's connection will be closed.
//
// Typically called in a defer after client registration:
//
//	defer hub.Unregister(conn)
//
// Thread-safe: can be called from multiple goroutines.
// Safe to call multiple times for the same client (no-op after first call).
func (h *Hub) Unregister(client *Conn) {
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return
	}
	h.mu.RUnlock()

	h.unregister <- client
}

// Broadcast sends a message to all connected clients.
//
// The message is queued for delivery. Actual delivery happens
// asynchronously in the event loop.
//
// If a client write fails, that client is automatically unregistered.
//
// Example:
//
//	hub.Broadcast([]byte("Hello, everyone!"))
//
// Thread-safe: can be called from multiple goroutines.
// Non-blocking: queues message and returns immediately.
func (h *Hub) Broadcast(message []byte) {
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return
	}
	h.mu.RUnlock()

	h.broadcast <- message
}

// BroadcastText sends a text message to all connected clients.
//
// Convenience wrapper around Broadcast() for text messages.
//
// Example:
//
//	hub.BroadcastText("Server notification")
//
// Thread-safe: can be called from multiple goroutines.
func (h *Hub) BroadcastText(text string) {
	h.Broadcast([]byte(text))
}

// BroadcastJSON sends a JSON message to all connected clients.
//
// Marshals the value to JSON and broadcasts as text message.
//
// Example:
//
//	type Message struct {
//	    Type string `json:"type"`
//	    Text string `json:"text"`
//	}
//	hub.BroadcastJSON(Message{Type: "notification", Text: "Hello"})
//
// Returns error if JSON marshaling fails.
// Thread-safe: can be called from multiple goroutines.
func (h *Hub) BroadcastJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	h.Broadcast(data)
	return nil
}

// ClientCount returns the number of currently connected clients.
//
// Thread-safe: can be called from multiple goroutines.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Close stops the Hub and disconnects all clients.
//
// Performs graceful shutdown:
//  1. Sets closed flag to prevent new operations
//  2. Stops the event loop
//  3. Waits for Run() to exit
//  4. Closes all client connections
//  5. Closes all channels
//
// Safe to call multiple times (no-op after first call).
//
// Example:
//
//	defer hub.Close()
func (h *Hub) Close() error {
	// Set closed flag first (prevents new Register/Unregister/Broadcast)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.mu.Unlock()

	// Signal shutdown to event loop
	close(h.done)

	// Wait for event loop to exit
	h.wg.Wait()

	// Close all client connections
	h.mu.Lock()
	for client := range h.clients {
		_ = client.Close()
	}
	h.clients = make(map[*Conn]bool) // Clear map
	h.mu.Unlock()

	// Close channels (safe now that event loop exited and no new sends)
	close(h.register)
	close(h.unregister)
	close(h.broadcast)

	return nil
}
