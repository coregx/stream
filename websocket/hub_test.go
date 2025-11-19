package websocket

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"sync"
	"testing"
	"time"
)

// TestHub_RegisterUnregister tests client registration and unregistration.
func TestHub_RegisterUnregister(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Create mock client
	client := mockConnForHub(t)

	// Initially no clients
	if count := hub.ClientCount(); count != 0 {
		t.Errorf("Initial ClientCount() = %d, want 0", count)
	}

	// Register client
	hub.Register(client)

	// Wait for registration to process
	time.Sleep(10 * time.Millisecond)

	// Should have 1 client
	if count := hub.ClientCount(); count != 1 {
		t.Errorf("After Register() ClientCount() = %d, want 1", count)
	}

	// Unregister client
	hub.Unregister(client)

	// Wait for unregistration to process
	time.Sleep(10 * time.Millisecond)

	// Should have 0 clients
	if count := hub.ClientCount(); count != 0 {
		t.Errorf("After Unregister() ClientCount() = %d, want 0", count)
	}
}

// TestHub_Broadcast tests broadcasting messages to all clients.
func TestHub_Broadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Create 3 mock clients
	const numClients = 3
	clients := make([]*mockHubClient, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = newMockHubClient(t)
		hub.Register(clients[i].conn)
	}

	// Wait for registration
	time.Sleep(20 * time.Millisecond)

	// Broadcast message
	testMessage := []byte("Hello, everyone!")
	hub.Broadcast(testMessage)

	// Wait for delivery
	time.Sleep(50 * time.Millisecond)

	// Verify all clients received message
	for i, client := range clients {
		messages := client.Messages() // Thread-safe read
		if len(messages) == 0 {
			t.Errorf("Client %d received no messages", i)
			continue
		}

		if !bytes.Equal(messages[0], testMessage) {
			t.Errorf("Client %d received %q, want %q", i, messages[0], testMessage)
		}
	}
}

// TestHub_BroadcastText tests text broadcasting.
func TestHub_BroadcastText(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	client := newMockHubClient(t)
	hub.Register(client.conn)
	time.Sleep(10 * time.Millisecond)

	// Broadcast text
	testText := "Test notification"
	hub.BroadcastText(testText)
	time.Sleep(20 * time.Millisecond)

	// Verify received
	messages := client.Messages() // Thread-safe read
	if len(messages) == 0 {
		t.Fatal("Client received no messages")
	}

	if string(messages[0]) != testText {
		t.Errorf("Received %q, want %q", messages[0], testText)
	}
}

// TestHub_BroadcastJSON tests JSON broadcasting.
func TestHub_BroadcastJSON(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	client := newMockHubClient(t)
	hub.Register(client.conn)

	// Wait for registration to complete (polling pattern instead of sleep)
	timeout := time.After(1 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

waitRegistration:
	for {
		select {
		case <-ticker.C:
			if hub.ClientCount() > 0 {
				break waitRegistration
			}
		case <-timeout:
			t.Fatal("Timeout waiting for client registration")
		}
	}

	// Broadcast JSON
	type Message struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	msg := Message{Type: "notification", Text: "Hello"}

	err := hub.BroadcastJSON(msg)
	if err != nil {
		t.Fatalf("BroadcastJSON() error = %v", err)
	}

	// Wait for message to arrive (polling pattern instead of sleep)
	timeout = time.After(1 * time.Second)
	ticker = time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	var messages [][]byte
waitMessage:
	for {
		select {
		case <-ticker.C:
			messages = client.Messages() // Thread-safe read
			if len(messages) > 0 {
				break waitMessage
			}
		case <-timeout:
			t.Fatal("Client received no messages")
		}
	}

	var received Message
	if err := json.Unmarshal(messages[0], &received); err != nil {
		t.Fatalf("JSON unmarshal error = %v", err)
	}

	if received != msg {
		t.Errorf("Received %+v, want %+v", received, msg)
	}
}

// TestHub_ClientCount tests accurate client counting.
func TestHub_ClientCount(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Add clients incrementally
	const maxClients = 5
	clients := make([]*mockHubClient, maxClients)

	for i := 0; i < maxClients; i++ {
		clients[i] = newMockHubClient(t)
		hub.Register(clients[i].conn)
		time.Sleep(5 * time.Millisecond)

		expected := i + 1
		if count := hub.ClientCount(); count != expected {
			t.Errorf("After %d registrations, ClientCount() = %d, want %d", expected, count, expected)
		}
	}

	// Remove clients incrementally
	for i := 0; i < maxClients; i++ {
		hub.Unregister(clients[i].conn)
		time.Sleep(5 * time.Millisecond)

		expected := maxClients - i - 1
		if count := hub.ClientCount(); count != expected {
			t.Errorf("After %d unregistrations, ClientCount() = %d, want %d", i+1, count, expected)
		}
	}
}

// TestHub_ConcurrentRegistration tests thread-safe concurrent operations.
func TestHub_ConcurrentRegistration(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Register 50 clients concurrently
	const numClients = 50
	var wg sync.WaitGroup
	wg.Add(numClients)

	for i := 0; i < numClients; i++ {
		go func() {
			defer wg.Done()
			client := mockConnForHub(t)
			hub.Register(client)
		}()
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Should have all clients registered
	if count := hub.ClientCount(); count != numClients {
		t.Errorf("ClientCount() = %d, want %d", count, numClients)
	}
}

// TestHub_ClientDisconnect tests auto-unregister on write failure.
//
// When a client connection fails, the Hub should automatically
// unregister it during broadcast.
func TestHub_ClientDisconnect(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Create client that will fail writes
	failingClient := mockConnForHub(t)

	// Close connection to simulate failure
	failingClient.closeMu.Lock()
	failingClient.closed = true
	failingClient.closeMu.Unlock()

	hub.Register(failingClient)
	time.Sleep(10 * time.Millisecond)

	// Should have 1 client
	if count := hub.ClientCount(); count != 1 {
		t.Errorf("Before broadcast, ClientCount() = %d, want 1", count)
	}

	// Broadcast (should fail and auto-unregister)
	hub.Broadcast([]byte("test"))
	time.Sleep(50 * time.Millisecond)

	// Should have 0 clients (auto-unregistered)
	if count := hub.ClientCount(); count != 0 {
		t.Errorf("After failed broadcast, ClientCount() = %d, want 0", count)
	}
}

// TestHub_Close tests graceful shutdown.
func TestHub_Close(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Register clients
	client1 := newMockHubClient(t)
	client2 := newMockHubClient(t)
	hub.Register(client1.conn)
	hub.Register(client2.conn)
	time.Sleep(20 * time.Millisecond)

	// Should have 2 clients
	if count := hub.ClientCount(); count != 2 {
		t.Errorf("Before Close(), ClientCount() = %d, want 2", count)
	}

	// Close hub
	err := hub.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Should have 0 clients after close
	if count := hub.ClientCount(); count != 0 {
		t.Errorf("After Close(), ClientCount() = %d, want 0", count)
	}

	// Calling Close() again should be safe (no-op)
	err = hub.Close()
	if err != nil {
		t.Errorf("Second Close() error = %v", err)
	}
}

// TestHub_BroadcastAfterClose tests that broadcasting after close is safe.
func TestHub_BroadcastAfterClose(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := newMockHubClient(t)
	hub.Register(client.conn)
	time.Sleep(10 * time.Millisecond)

	// Stop extractMessages goroutine BEFORE hub.Close() to prevent race
	// Hub.Close() writes close frame → writeBuf, extractMessages reads writeBuf
	client.Stop()
	time.Sleep(10 * time.Millisecond) // Wait for goroutine to exit

	// Close hub
	hub.Close()

	// Wait for hub to fully close (prevents race with closed flag)
	time.Sleep(20 * time.Millisecond)

	// Broadcasting after close should not panic
	// (closed flag prevents send on closed channel)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Broadcast after Close() panicked: %v", r)
		}
	}()

	// These should all be no-ops after close (closed flag check)
	hub.Broadcast([]byte("test"))
	hub.BroadcastText("test")
	hub.Register(client.conn)
	hub.Unregister(client.conn)

	// Should not panic - operations are safely ignored
}

// mockHubClient is a test helper that captures messages sent to it.
type mockHubClient struct {
	conn             *Conn
	writeBuf         *bytes.Buffer
	receivedMessages [][]byte
	mu               sync.Mutex    // Protects writeBuf AND receivedMessages
	done             chan struct{} // Signal to stop extractMessages goroutine
	stopOnce         sync.Once     // Ensure done is only closed once
}

// Write implements io.Writer with thread-safe buffer access.
func (c *mockHubClient) Write(p []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeBuf.Write(p)
}

// newMockHubClient creates a mock client that captures broadcast messages.
func newMockHubClient(t *testing.T) *mockHubClient {
	t.Helper()

	// Create client with buffer
	client := &mockHubClient{
		writeBuf:         &bytes.Buffer{},
		receivedMessages: make([][]byte, 0),
		done:             make(chan struct{}),
	}

	// Create writer that uses client.Write() (thread-safe)
	writer := bufio.NewWriter(client)

	conn := &Conn{
		conn:     nil,
		reader:   nil,
		writer:   writer,
		isServer: true,
	}

	client.conn = conn

	// Start goroutine to extract messages from buffer
	go client.extractMessages()

	// Stop goroutine when test completes
	t.Cleanup(func() {
		client.Stop()
	})

	return client
}

// Stop safely stops the extractMessages goroutine (can be called multiple times).
func (c *mockHubClient) Stop() {
	c.stopOnce.Do(func() {
		close(c.done)
	})
}

// extractMessages reads frames from buffer and extracts payloads.
func (c *mockHubClient) extractMessages() {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.mu.Lock()
			if c.writeBuf.Len() == 0 {
				c.mu.Unlock()
				continue
			}

			// Read frame from buffer
			reader := bufio.NewReader(bytes.NewReader(c.writeBuf.Bytes()))
			frame, err := readFrame(reader)
			if err != nil {
				c.mu.Unlock()
				continue
			}

			// Store payload
			c.receivedMessages = append(c.receivedMessages, frame.payload)

			// Clear buffer (simple approach - reset)
			c.writeBuf.Reset()
			c.mu.Unlock()
		}
	}
}

// Messages returns a thread-safe copy of received messages.
func (c *mockHubClient) Messages() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([][]byte, len(c.receivedMessages))
	copy(result, c.receivedMessages)
	return result
}
