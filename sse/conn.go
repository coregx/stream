package sse

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// Common errors returned by Conn.
var (
	// ErrConnectionClosed is returned when attempting to send on a closed connection.
	ErrConnectionClosed = errors.New("sse: connection closed")

	// ErrNoFlusher is returned when http.ResponseWriter doesn't support flushing.
	// This usually indicates an incompatible HTTP server or proxy.
	ErrNoFlusher = errors.New("sse: ResponseWriter does not support flushing")
)

// Conn represents an active SSE connection to a client.
//
// Conn manages the lifecycle of a Server-Sent Events connection, handling
// event transmission, context cancellation, and graceful shutdown.
//
// Example:
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//	    conn, err := sse.Upgrade(w, r)
//	    if err != nil {
//	        http.Error(w, err.Error(), http.StatusInternalServerError)
//	        return
//	    }
//	    defer conn.Close()
//
//	    // Send events
//	    conn.SendData("Hello, SSE!")
//	    conn.SendJSON(map[string]string{"status": "connected"})
//	}
type Conn struct {
	w       http.ResponseWriter
	flusher http.Flusher
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	closed  bool
	mu      sync.Mutex
}

// Upgrade upgrades an HTTP connection to SSE with the request's context.
//
// It sets the necessary SSE headers, validates that the ResponseWriter supports
// flushing, and sends an initial connection comment.
//
// The connection uses r.Context() for cancellation tracking.
//
// Returns ErrNoFlusher if the ResponseWriter doesn't implement http.Flusher.
//
// Example:
//
//	conn, err := sse.Upgrade(w, r)
//	if err != nil {
//	    http.Error(w, err.Error(), http.StatusInternalServerError)
//	    return
//	}
//	defer conn.Close()
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	return UpgradeWithContext(r.Context(), w, r)
}

// UpgradeWithContext upgrades an HTTP connection to SSE with a custom context.
//
// This is useful when you need finer control over connection lifecycle
// independent of the HTTP request context.
//
// Returns ErrNoFlusher if the ResponseWriter doesn't implement http.Flusher.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
//	defer cancel()
//	conn, err := sse.UpgradeWithContext(ctx, w, r)
func UpgradeWithContext(ctx context.Context, w http.ResponseWriter, _ *http.Request) (*Conn, error) {
	// Verify ResponseWriter supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, ErrNoFlusher
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Send initial connection comment
	_, err := io.WriteString(w, ": connected\n\n")
	if err != nil {
		return nil, fmt.Errorf("sse: failed to write connection comment: %w", err)
	}
	flusher.Flush()

	// Create connection with context
	connCtx, cancel := context.WithCancel(ctx)
	conn := &Conn{
		w:       w,
		flusher: flusher,
		ctx:     connCtx,
		cancel:  cancel,
		done:    make(chan struct{}),
		closed:  false,
	}

	// Watch for context cancellation
	go conn.watchContext()

	return conn, nil
}

// watchContext monitors the context and closes the connection when canceled.
func (c *Conn) watchContext() {
	<-c.ctx.Done()
	_ = c.Close()
}

// Send sends an Event to the client.
//
// Returns ErrConnectionClosed if the connection is already closed.
//
// Example:
//
//	event := sse.NewEvent("User logged in").
//	    WithType("notification").
//	    WithID("evt-123")
//	err := conn.Send(event)
func (c *Conn) Send(event *Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrConnectionClosed
	}

	// Write event to response
	_, err := io.WriteString(c.w, event.String())
	if err != nil {
		return fmt.Errorf("sse: failed to write event: %w", err)
	}

	// Flush immediately to send to client
	c.flusher.Flush()
	return nil
}

// SendData sends a simple data-only event to the client.
//
// This is a convenience method equivalent to Send(NewEvent(data)).
//
// Returns ErrConnectionClosed if the connection is already closed.
//
// Example:
//
//	err := conn.SendData("Hello, World!")
func (c *Conn) SendData(data string) error {
	return c.Send(NewEvent(data))
}

// SendJSON sends a JSON-encoded event to the client.
//
// The value is marshaled to JSON using encoding/json/v2. If marshaling fails,
// the error is returned.
//
// Returns ErrConnectionClosed if the connection is already closed.
//
// Example:
//
//	user := map[string]string{"name": "Alice", "status": "online"}
//	err := conn.SendJSON(user)
func (c *Conn) SendJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("sse: failed to marshal JSON: %w", err)
	}
	return c.SendData(string(data))
}

// Close closes the SSE connection.
//
// It's safe to call Close multiple times. Subsequent calls are no-ops.
//
// After Close, all Send operations will return ErrConnectionClosed.
//
// Example:
//
//	defer conn.Close()
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	c.cancel()
	close(c.done)
	return nil
}

// Done returns a channel that's closed when the connection is closed.
//
// This is useful for coordinating shutdown with goroutines sending events.
//
// Example:
//
//	go func() {
//	    ticker := time.NewTicker(time.Second)
//	    defer ticker.Stop()
//	    for {
//	        select {
//	        case <-ticker.C:
//	            conn.SendData("ping")
//	        case <-conn.Done():
//	            return
//	        }
//	    }
//	}()
func (c *Conn) Done() <-chan struct{} {
	return c.done
}
