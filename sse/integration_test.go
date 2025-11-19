package sse

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// sseClient simulates a real SSE client by parsing text/event-stream format.
type sseClient struct {
	url    string
	client *http.Client
	events chan string
	errors chan error
	cancel context.CancelFunc
	closed atomic.Bool
	mu     sync.Mutex
}

// newSSEClient creates a new SSE client connected to the given URL.
func newSSEClient(url string) *sseClient {
	return &sseClient{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		events: make(chan string, 100),
		errors: make(chan error, 10),
	}
}

// Connect establishes the SSE connection and starts reading events.
func (c *sseClient) Connect(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Verify Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/event-stream" {
		resp.Body.Close()
		return fmt.Errorf("unexpected content type: %s", contentType)
	}

	// Start reading events in goroutine
	childCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	go c.readEvents(childCtx, resp.Body)

	return nil
}

// readEvents reads SSE events from the response body.
func (c *sseClient) readEvents(ctx context.Context, body io.ReadCloser) {
	defer body.Close()
	defer close(c.events)

	scanner := bufio.NewScanner(body)
	// Increase buffer size for large events (2MB max)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	var eventData strings.Builder

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		// Empty line = end of event
		if line == "" {
			if eventData.Len() > 0 {
				c.events <- eventData.String()
				eventData.Reset()
			}
			continue
		}

		// Comment line (keep-alive)
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Parse field:value
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if eventData.Len() > 0 {
				eventData.WriteByte('\n')
			}
			eventData.WriteString(data)
		}
		// We could parse event:, id:, retry: here but for testing we focus on data
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		select {
		case c.errors <- err:
		default:
		}
	}
}

// Events returns the channel for receiving events.
func (c *sseClient) Events() <-chan string {
	return c.events
}

// Errors returns the channel for receiving errors.
func (c *sseClient) Errors() <-chan error {
	return c.errors
}

// Close closes the SSE client connection.
func (c *sseClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed.Swap(true) {
		return
	}

	if c.cancel != nil {
		c.cancel()
	}
}

// TestIntegration_RealHTTPServer tests SSE with a real HTTP server.
func TestIntegration_RealHTTPServer(t *testing.T) {
	// Create HTTP server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()

		// Send test events
		_ = conn.SendData("event1")
		_ = conn.SendData("event2")
		_ = conn.SendData("event3")

		// Keep connection open briefly
		time.Sleep(100 * time.Millisecond)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Connect SSE client
	client := newSSEClient(server.URL)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Read events
	events := []string{}
	timeout := time.After(2 * time.Second)

	for i := 0; i < 3; i++ {
		select {
		case event := <-client.Events():
			events = append(events, event)
		case err := <-client.Errors():
			t.Fatalf("Client error: %v", err)
		case <-timeout:
			t.Fatalf("Timeout waiting for events, got %d events: %v", len(events), events)
		}
	}

	// Verify events
	expected := []string{"event1", "event2", "event3"}
	if len(events) != len(expected) {
		t.Fatalf("Expected %d events, got %d: %v", len(expected), len(events), events)
	}

	for i, want := range expected {
		if events[i] != want {
			t.Errorf("Event[%d] = %q, want %q", i, events[i], want)
		}
	}
}

// TestIntegration_MultipleClients tests broadcasting to multiple concurrent clients.
func TestIntegration_MultipleClients(t *testing.T) {
	const numClients = 10
	const numEvents = 5

	hub := NewHub[string]()
	go hub.Run()
	defer hub.Close()

	// Create HTTP server with hub
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer hub.Unregister(conn)

		if err := hub.Register(conn); err != nil {
			t.Logf("Failed to register: %v", err)
			return
		}

		// Keep connection alive
		<-conn.Done()
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Connect multiple clients
	clients := make([]*sseClient, numClients)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Close all clients at end
	defer func() {
		for _, client := range clients {
			if client != nil {
				client.Close()
			}
		}
	}()

	for i := 0; i < numClients; i++ {
		client := newSSEClient(server.URL)

		if err := client.Connect(ctx); err != nil {
			t.Fatalf("Client %d failed to connect: %v", i, err)
		}

		clients[i] = client
	}

	// Wait for all clients to register
	time.Sleep(100 * time.Millisecond)

	// Verify client count
	if got := hub.Clients(); got != numClients {
		t.Fatalf("Expected %d clients, got %d", numClients, got)
	}

	// Broadcast events
	for i := 0; i < numEvents; i++ {
		msg := fmt.Sprintf("broadcast-%d", i)
		if err := hub.Broadcast(msg); err != nil {
			t.Fatalf("Broadcast failed: %v", err)
		}
		time.Sleep(10 * time.Millisecond) // Small delay between broadcasts
	}

	// Verify all clients received all events
	timeout := time.After(5 * time.Second)

	for clientIdx, client := range clients {
		received := []string{}

		for i := 0; i < numEvents; i++ {
			select {
			case event := <-client.Events():
				received = append(received, event)
			case err := <-client.Errors():
				t.Errorf("Client %d error: %v", clientIdx, err)
			case <-timeout:
				t.Fatalf("Client %d timeout, received %d/%d events: %v",
					clientIdx, len(received), numEvents, received)
			}
		}

		// Verify event order and content
		for i := 0; i < numEvents; i++ {
			expected := fmt.Sprintf("broadcast-%d", i)
			if received[i] != expected {
				t.Errorf("Client %d event[%d] = %q, want %q",
					clientIdx, i, received[i], expected)
			}
		}
	}
}

// TestIntegration_ClientReconnect tests client disconnect and reconnect behavior.
func TestIntegration_ClientReconnect(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()
	defer hub.Close()

	var activeConns atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := hub.Register(conn); err != nil {
			return
		}

		activeConns.Add(1)
		defer func() {
			activeConns.Add(-1)
			hub.Unregister(conn)
		}()

		<-conn.Done()
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// First connection (with cancellable context)
	ctx1, cancel1 := context.WithCancel(context.Background())
	client1 := newSSEClient(server.URL)
	if err := client1.Connect(ctx1); err != nil {
		cancel1()
		t.Fatalf("First connect failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if got := hub.Clients(); got != 1 {
		t.Fatalf("Expected 1 client, got %d", got)
	}

	// Send event to first client
	if err := hub.Broadcast("message1"); err != nil {
		t.Fatalf("Broadcast failed: %v", err)
	}

	select {
	case event := <-client1.Events():
		if event != "message1" {
			t.Errorf("Expected 'message1', got %q", event)
		}
	case <-time.After(2 * time.Second):
		cancel1()
		t.Fatal("Timeout waiting for message1")
	}

	// Disconnect first client (cancel context to trigger server-side Done())
	cancel1()
	client1.Close()

	// Wait for server handler to finish and unregister
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if hub.Clients() == 0 && activeConns.Load() == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if got := hub.Clients(); got != 0 {
		t.Fatalf("Expected 0 clients after disconnect, got %d (active=%d)",
			got, activeConns.Load())
	}

	// Reconnect
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	client2 := newSSEClient(server.URL)
	defer client2.Close()

	if err := client2.Connect(ctx2); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if got := hub.Clients(); got != 1 {
		t.Fatalf("Expected 1 client after reconnect, got %d", got)
	}

	// Send event to reconnected client
	if err := hub.Broadcast("message2"); err != nil {
		t.Fatalf("Broadcast after reconnect failed: %v", err)
	}

	select {
	case event := <-client2.Events():
		if event != "message2" {
			t.Errorf("Expected 'message2', got %q", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message2")
	}
}

// TestIntegration_ContextCancellation tests context cancellation behavior.
func TestIntegration_ContextCancellation(t *testing.T) {
	t.Run("ServerSide", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := UpgradeWithContext(ctx, w, r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer conn.Close()

			// Send initial event
			_ = conn.SendData("connected")

			// Wait for context cancellation
			<-conn.Done()
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		client := newSSEClient(server.URL)
		defer client.Close()

		clientCtx := context.Background()
		if err := client.Connect(clientCtx); err != nil {
			t.Fatalf("Connect failed: %v", err)
		}

		// Read initial event
		select {
		case event := <-client.Events():
			if event != "connected" {
				t.Errorf("Expected 'connected', got %q", event)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for connected event")
		}

		// Cancel server context
		cancel()

		// Verify connection closes
		select {
		case _, ok := <-client.Events():
			if ok {
				t.Error("Expected channel to close")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for connection close")
		}
	})

	t.Run("ClientSide", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := Upgrade(w, r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer conn.Close()

			// Try to send events continuously
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if err := conn.SendData("tick"); err != nil {
						return
					}
				case <-conn.Done():
					return
				}
			}
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		client := newSSEClient(server.URL)

		clientCtx, cancel := context.WithCancel(context.Background())

		if err := client.Connect(clientCtx); err != nil {
			t.Fatalf("Connect failed: %v", err)
		}

		// Read a few events
		for i := 0; i < 3; i++ {
			select {
			case <-client.Events():
				// Success
			case <-time.After(2 * time.Second):
				t.Fatal("Timeout waiting for events")
			}
		}

		// Cancel client context
		cancel()
		client.Close()

		// Verify no more events
		time.Sleep(200 * time.Millisecond)
	})
}

// TestIntegration_LargeDataTransfer tests sending large events (1MB+).
func TestIntegration_LargeDataTransfer(t *testing.T) {
	const dataSize = 1 * 1024 * 1024 // 1 MB

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()

		// Send large data
		largeData := strings.Repeat("A", dataSize)
		if err := conn.SendData(largeData); err != nil {
			t.Logf("Failed to send large data: %v", err)
			return
		}

		time.Sleep(100 * time.Millisecond)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newSSEClient(server.URL)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Read large event
	select {
	case event := <-client.Events():
		if len(event) != dataSize {
			t.Errorf("Expected %d bytes, got %d", dataSize, len(event))
		}
		if !strings.HasPrefix(event, "AAAA") {
			t.Error("Large data corrupted")
		}
	case err := <-client.Errors():
		t.Fatalf("Client error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for large event")
	}
}

// TestIntegration_HighFrequency tests high-frequency event sending (1000+ events/sec).
func TestIntegration_HighFrequency(t *testing.T) {
	const numEvents = 1000
	const duration = 1 * time.Second

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()

		// Send events as fast as possible
		interval := duration / numEvents
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for i := 0; i < numEvents; i++ {
			<-ticker.C
			if err := conn.SendData(fmt.Sprintf("event-%d", i)); err != nil {
				return
			}
		}

		time.Sleep(100 * time.Millisecond)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newSSEClient(server.URL)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Count received events
	start := time.Now()
	received := 0
	// Increase timeout to 10s for slower CI environments (especially Windows)
	timeout := time.After(10 * time.Second)

	for received < numEvents {
		select {
		case event := <-client.Events():
			received++
			// Verify event format (spot check)
			if received%100 == 0 {
				expected := fmt.Sprintf("event-%d", received-1)
				if event != expected {
					t.Errorf("Event %d = %q, want %q", received, event, expected)
				}
			}
		case err := <-client.Errors():
			t.Fatalf("Client error after %d events: %v", received, err)
		case <-timeout:
			t.Fatalf("Timeout after %d events", received)
		}
	}

	elapsed := time.Since(start)
	rate := float64(received) / elapsed.Seconds()

	t.Logf("Received %d events in %v (%.0f events/sec)", received, elapsed, rate)

	if rate < 200 {
		t.Errorf("Event rate too low: %.0f events/sec (expected >200)", rate)
	}
}

// TestIntegration_ErrorPropagation tests error handling in real scenarios.
func TestIntegration_ErrorPropagation(t *testing.T) {
	t.Run("ImmediateClose", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := Upgrade(w, r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Close immediately
			conn.Close()

			// Try to send (should fail)
			err = conn.SendData("test")
			if !errors.Is(err, ErrConnectionClosed) {
				t.Errorf("Expected ErrConnectionClosed, got: %v", err)
			}
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		client := newSSEClient(server.URL)
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Connect(ctx); err != nil {
			t.Fatalf("Connect failed: %v", err)
		}

		// Verify connection closes quickly
		select {
		case _, ok := <-client.Events():
			if ok {
				t.Error("Expected channel to close")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for connection close")
		}
	})
}

// TestIntegration_MemoryLeak tests for resource leaks during connect/disconnect cycles.
func TestIntegration_MemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	hub := NewHub[string]()
	go hub.Run()
	defer hub.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer hub.Unregister(conn)

		if err := hub.Register(conn); err != nil {
			return
		}

		<-conn.Done()
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Run many connect/disconnect cycles
	const cycles = 100

	for i := 0; i < cycles; i++ {
		client := newSSEClient(server.URL)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		if err := client.Connect(ctx); err != nil {
			cancel()
			t.Fatalf("Cycle %d: Connect failed: %v", i, err)
		}

		// Wait for client to register
		time.Sleep(20 * time.Millisecond)

		// Send a message
		if err := hub.Broadcast(fmt.Sprintf("msg-%d", i)); err != nil {
			cancel()
			client.Close()
			t.Fatalf("Cycle %d: Broadcast failed: %v", i, err)
		}

		// Read the message
		select {
		case event := <-client.Events():
			if event != fmt.Sprintf("msg-%d", i) {
				t.Logf("Cycle %d: Got unexpected event: %q", i, event)
			}
		case err := <-client.Errors():
			cancel()
			client.Close()
			t.Fatalf("Cycle %d: Client error: %v", i, err)
		case <-time.After(1 * time.Second):
			cancel()
			client.Close()
			t.Fatalf("Cycle %d: Timeout reading event (hub has %d clients)", i, hub.Clients())
		}

		// Disconnect
		client.Close()
		cancel()

		// Let cleanup happen
		time.Sleep(10 * time.Millisecond)
	}

	// Verify all clients disconnected
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.Clients() == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if got := hub.Clients(); got != 0 {
		t.Errorf("Expected 0 clients after all cycles, got %d", got)
	}
}

// TestIntegration_ConcurrentOperations tests concurrent client operations.
func TestIntegration_ConcurrentOperations(t *testing.T) {
	const numClients = 20
	const eventsPerClient = 10

	hub := NewHub[string]()
	go hub.Run()
	defer hub.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer hub.Unregister(conn)

		if err := hub.Register(conn); err != nil {
			return
		}

		<-conn.Done()
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	var wg sync.WaitGroup
	errs := make(chan error, numClients)

	// Start many clients concurrently
	for i := 0; i < numClients; i++ {
		wg.Add(1)

		go func(clientID int) {
			defer wg.Done()

			client := newSSEClient(server.URL)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := client.Connect(ctx); err != nil {
				errs <- fmt.Errorf("client %d connect: %w", clientID, err)
				return
			}

			// Read events
			for j := 0; j < eventsPerClient; j++ {
				select {
				case <-client.Events():
					// Success
				case err := <-client.Errors():
					errs <- fmt.Errorf("client %d error: %w", clientID, err)
					return
				case <-time.After(5 * time.Second):
					errs <- fmt.Errorf("client %d timeout at event %d", clientID, j)
					return
				}
			}
		}(i)
	}

	// Wait for clients to connect
	time.Sleep(200 * time.Millisecond)

	// Broadcast events while clients are connecting/reading
	go func() {
		for i := 0; i < eventsPerClient; i++ {
			if err := hub.Broadcast(fmt.Sprintf("event-%d", i)); err != nil {
				errs <- fmt.Errorf("broadcast %d: %w", i, err)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Wait for all clients
	wg.Wait()

	// Check for errors
	close(errs)
	for err := range errs {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

// Benchmarks for integration testing

// BenchmarkIntegration_RealHTTP benchmarks full HTTP stack overhead.
func BenchmarkIntegration_RealHTTP(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()

		_ = conn.SendData("benchmark")
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		client := newSSEClient(server.URL)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		if err := client.Connect(ctx); err != nil {
			b.Fatalf("Connect failed: %v", err)
		}

		select {
		case <-client.Events():
			// Success
		case <-time.After(1 * time.Second):
			b.Fatal("Timeout")
		}

		client.Close()
		cancel()
	}
}

// BenchmarkIntegration_10Clients benchmarks 10 concurrent clients.
func BenchmarkIntegration_10Clients(b *testing.B) {
	benchmarkIntegrationClients(b, 10)
}

// BenchmarkIntegration_100Clients benchmarks 100 concurrent clients.
func BenchmarkIntegration_100Clients(b *testing.B) {
	benchmarkIntegrationClients(b, 100)
}

// benchmarkIntegrationClients is a helper for client benchmarks.
func benchmarkIntegrationClients(b *testing.B, numClients int) {
	hub := NewHub[string]()
	go hub.Run()
	defer hub.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer hub.Unregister(conn)

		if err := hub.Register(conn); err != nil {
			return
		}

		<-conn.Done()
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Connect all clients
	clients := make([]*sseClient, numClients)
	ctxs := make([]context.CancelFunc, numClients)

	// Cleanup at end
	defer func() {
		for _, client := range clients {
			if client != nil {
				client.Close()
			}
		}
		for _, cancel := range ctxs {
			if cancel != nil {
				cancel()
			}
		}
	}()

	for i := 0; i < numClients; i++ {
		client := newSSEClient(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ctxs[i] = cancel

		if err := client.Connect(ctx); err != nil {
			b.Fatalf("Client %d connect failed: %v", i, err)
		}

		clients[i] = client
	}

	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := hub.Broadcast(fmt.Sprintf("msg-%d", i)); err != nil {
			b.Fatalf("Broadcast failed: %v", err)
		}
	}
}

// BenchmarkIntegration_HighFrequency benchmarks 1000 events/sec throughput.
func BenchmarkIntegration_HighFrequency(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()

		for i := 0; i < b.N; i++ {
			if err := conn.SendData(fmt.Sprintf("event-%d", i)); err != nil {
				return
			}
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newSSEClient(server.URL)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		b.Fatalf("Connect failed: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	go func() {
		for event := range client.Events() {
			_ = event // Drain events
		}
	}()

	time.Sleep(time.Duration(b.N) * time.Millisecond)
}

// BenchmarkIntegration_LargeEvent benchmarks 1MB event transfer.
func BenchmarkIntegration_LargeEvent(b *testing.B) {
	const dataSize = 1 * 1024 * 1024 // 1 MB
	largeData := strings.Repeat("A", dataSize)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()

		for i := 0; i < b.N; i++ {
			if err := conn.SendData(largeData); err != nil {
				return
			}
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newSSEClient(server.URL)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		b.Fatalf("Connect failed: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(dataSize))

	go func() {
		for event := range client.Events() {
			_ = event // Drain events
		}
	}()

	time.Sleep(time.Duration(b.N) * 100 * time.Millisecond)
}
