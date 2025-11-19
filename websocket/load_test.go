package websocket

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLoad_ConcurrentConnections tests handling 100 concurrent WebSocket connections.
func TestLoad_ConcurrentConnections(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	// Setup echo server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		// Echo server
		for {
			msgType, data, err := conn.Read()
			if err != nil {
				break
			}
			if err := conn.Write(msgType, data); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	const (
		numClients        = 100
		messagesPerClient = 10
		totalExpected     = numClients * messagesPerClient
	)

	var (
		messagesReceived atomic.Int32
		errors           atomic.Int32
		wg               sync.WaitGroup
	)

	wg.Add(numClients)
	startTime := time.Now()

	// Create 100 concurrent connections
	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()

			wsURL := "ws" + server.URL[4:] // Replace http with ws
			conn, resp, err := Dial(context.Background(), wsURL, nil)
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				errors.Add(1)
				t.Errorf("Client %d: dial error: %v", clientID, err)
				return
			}
			defer conn.Close()

			// Send and receive 10 messages
			for j := 0; j < messagesPerClient; j++ {
				testMsg := []byte(fmt.Sprintf("client-%d-msg-%d", clientID, j))

				// Write
				if err := conn.Write(TextMessage, testMsg); err != nil {
					errors.Add(1)
					t.Errorf("Client %d: write error: %v", clientID, err)
					return
				}

				// Read echo
				_, data, err := conn.Read()
				if err != nil {
					errors.Add(1)
					t.Errorf("Client %d: read error: %v", clientID, err)
					return
				}

				if !bytes.Equal(data, testMsg) {
					errors.Add(1)
					t.Errorf("Client %d: got %q, want %q", clientID, data, testMsg)
					return
				}

				messagesReceived.Add(1)
			}
		}(i)
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		duration := time.Since(startTime)
		received := messagesReceived.Load()
		errCount := errors.Load()

		t.Logf("Load test completed in %v", duration)
		t.Logf("Messages sent/received: %d/%d", totalExpected, received)
		t.Logf("Errors: %d", errCount)
		t.Logf("Throughput: %.0f msg/s", float64(received)/duration.Seconds())

		if received != totalExpected {
			t.Errorf("Received %d messages, want %d", received, totalExpected)
		}

		if errCount > 0 {
			t.Errorf("Got %d errors during test", errCount)
		}

	case <-time.After(30 * time.Second):
		t.Fatal("Test timeout - not all clients completed within 30 seconds")
	}
}

// TestLoad_Hub_100Clients tests Hub broadcasting to 100 concurrent clients.
func TestLoad_Hub_100Clients(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Setup broadcast server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		hub.Register(conn)
		defer hub.Unregister(conn)

		// Keep connection alive
		for {
			if _, _, err := conn.Read(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	const numClients = 100
	const numBroadcasts = 1000

	// Track messages received by each client
	clientMessages := make([]atomic.Int32, numClients)
	var wg sync.WaitGroup
	wg.Add(numClients)

	// Connect 100 clients
	clients := make([]*Conn, numClients)
	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()

			wsURL := "ws" + server.URL[4:]
			conn, resp, err := Dial(context.Background(), wsURL, nil)
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				t.Errorf("Client %d: dial error: %v", clientID, err)
				return
			}
			defer conn.Close()

			clients[clientID] = conn

			// Read broadcasts
			for {
				_, _, err := conn.Read()
				if err != nil {
					break
				}
				clientMessages[clientID].Add(1)
			}
		}(i)
	}

	// Wait for all clients to connect
	time.Sleep(500 * time.Millisecond)

	connectedClients := hub.ClientCount()
	if connectedClients != numClients {
		t.Errorf("Connected clients = %d, want %d", connectedClients, numClients)
	}

	// Broadcast 1000 messages
	startTime := time.Now()
	for i := 0; i < numBroadcasts; i++ {
		msg := []byte(fmt.Sprintf("broadcast-%d", i))
		hub.Broadcast(msg)
	}
	broadcastDuration := time.Since(startTime)

	t.Logf("Broadcast phase completed in %v", broadcastDuration)
	t.Logf("Broadcast throughput: %.0f msg/s", float64(numBroadcasts)/broadcastDuration.Seconds())

	// Wait for messages to be delivered
	time.Sleep(2 * time.Second)

	// Close hub to terminate all clients
	hub.Close()

	// Wait for all clients to exit
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Clients did not exit within timeout")
	}

	totalDuration := time.Since(startTime)
	t.Logf("Total test duration: %v", totalDuration)

	// Verify message delivery
	var totalReceived int32
	for i := 0; i < numClients; i++ {
		received := clientMessages[i].Load()
		totalReceived += received

		// Allow some message loss due to timing (95% threshold)
		minExpected := int32(float64(numBroadcasts) * 0.95)
		if received < minExpected {
			t.Errorf("Client %d: received %d messages, want at least %d", i, received, minExpected)
		}
	}

	expectedTotal := int32(numClients * numBroadcasts)
	receivedPercent := float64(totalReceived) / float64(expectedTotal) * 100

	t.Logf("Total messages: %d/%d (%.1f%%)", totalReceived, expectedTotal, receivedPercent)

	if receivedPercent < 95.0 {
		t.Errorf("Message delivery rate %.1f%%, want >= 95%%", receivedPercent)
	}
}

// TestLoad_SSE_100Clients tests SSE Broker broadcasting to 100 concurrent clients.
// This test is placed in websocket package to compare performance with WebSocket Hub.
func TestLoad_SSE_100Clients(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	// Note: This test requires importing sse package, but for simplicity
	// and to avoid circular dependencies, we'll test WebSocket throughput
	// and separately benchmark SSE in its own package.
	t.Skip("SSE load test should be in sse package")
}

// TestLoad_RapidMessages tests rapid message sending and receiving.
func TestLoad_RapidMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	// Setup echo server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		for {
			msgType, data, err := conn.Read()
			if err != nil {
				break
			}
			if err := conn.Write(msgType, data); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	conn, resp, err := Dial(context.Background(), wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer conn.Close()

	const numMessages = 10000
	var (
		sent     atomic.Int32
		received atomic.Int32
		wg       sync.WaitGroup
	)

	wg.Add(2)

	// Sender goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < numMessages; i++ {
			msg := []byte(fmt.Sprintf("msg-%d", i))
			if err := conn.Write(TextMessage, msg); err != nil {
				t.Errorf("Write error: %v", err)
				return
			}
			sent.Add(1)
		}
	}()

	// Receiver goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < numMessages; i++ {
			if _, _, err := conn.Read(); err != nil {
				t.Errorf("Read error: %v", err)
				return
			}
			received.Add(1)
		}
	}()

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	startTime := time.Now()
	select {
	case <-done:
		duration := time.Since(startTime)
		sentCount := sent.Load()
		receivedCount := received.Load()

		t.Logf("Rapid messages test completed in %v", duration)
		t.Logf("Sent: %d, Received: %d", sentCount, receivedCount)
		t.Logf("Throughput: %.0f msg/s", float64(receivedCount)/duration.Seconds())

		if sentCount != numMessages {
			t.Errorf("Sent %d messages, want %d", sentCount, numMessages)
		}
		if receivedCount != numMessages {
			t.Errorf("Received %d messages, want %d", receivedCount, numMessages)
		}

	case <-time.After(30 * time.Second):
		t.Fatal("Test timeout")
	}
}

// TestLoad_ParallelHubs tests multiple Hubs running concurrently.
func TestLoad_ParallelHubs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		numHubs          = 10
		clientsPerHub    = 20
		broadcastsPerHub = 100
	)

	var wg sync.WaitGroup
	wg.Add(numHubs)

	startTime := time.Now()
	errors := make(chan error, numHubs*clientsPerHub)

	for hubID := 0; hubID < numHubs; hubID++ {
		go func(id int) {
			defer wg.Done()

			hub := NewHub()
			go hub.Run()
			defer hub.Close()

			// Setup server for this hub
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := Upgrade(w, r, nil)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				defer conn.Close()

				hub.Register(conn)
				defer hub.Unregister(conn)

				for {
					if _, _, err := conn.Read(); err != nil {
						break
					}
				}
			}))
			defer server.Close()

			// Connect clients
			clients := make([]*Conn, clientsPerHub)
			clientReceived := make([]atomic.Int32, clientsPerHub)

			var clientWG sync.WaitGroup
			clientWG.Add(clientsPerHub)

			for i := 0; i < clientsPerHub; i++ {
				go func(clientID int) {
					defer clientWG.Done()

					wsURL := "ws" + server.URL[4:]
					conn, resp, err := Dial(context.Background(), wsURL, nil)
					if resp != nil && resp.Body != nil {
						defer resp.Body.Close()
					}
					if err != nil {
						errors <- fmt.Errorf("Hub %d, Client %d: dial error: %w", id, clientID, err)
						return
					}
					defer conn.Close()

					clients[clientID] = conn

					for {
						if _, _, err := conn.Read(); err != nil {
							break
						}
						clientReceived[clientID].Add(1)
					}
				}(i)
			}

			time.Sleep(200 * time.Millisecond)

			// Broadcast messages
			for i := 0; i < broadcastsPerHub; i++ {
				msg := []byte(fmt.Sprintf("hub-%d-broadcast-%d", id, i))
				hub.Broadcast(msg)
			}

			time.Sleep(500 * time.Millisecond)
			hub.Close()

			clientWG.Wait()

			// Verify delivery
			for i := 0; i < clientsPerHub; i++ {
				received := clientReceived[i].Load()
				if received < int32(broadcastsPerHub*90/100) {
					errors <- fmt.Errorf("Hub %d, Client %d: received %d, want ~%d", id, i, received, broadcastsPerHub)
				}
			}
		}(hubID)
	}

	wg.Wait()
	close(errors)

	duration := time.Since(startTime)
	totalBroadcasts := numHubs * broadcastsPerHub
	totalMessages := totalBroadcasts * clientsPerHub

	t.Logf("Parallel hubs test completed in %v", duration)
	t.Logf("Hubs: %d, Clients per hub: %d", numHubs, clientsPerHub)
	t.Logf("Total broadcasts: %d", totalBroadcasts)
	t.Logf("Total messages: %d", totalMessages)
	t.Logf("Throughput: %.0f msg/s", float64(totalMessages)/duration.Seconds())

	var errorCount int
	for err := range errors {
		t.Error(err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Got %d errors during parallel hubs test", errorCount)
	}
}
