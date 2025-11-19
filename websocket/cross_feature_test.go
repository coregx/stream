package websocket

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coregx/stream/sse"
)

// dialWebSocket is a simple WebSocket client for integration tests.
func dialWebSocket(_ context.Context, url string) (*Conn, error) {
	// Parse URL
	url = strings.TrimPrefix(url, "ws://")
	parts := strings.SplitN(url, "/", 2)
	host := parts[0]
	path := "/"
	if len(parts) > 1 {
		path = "/" + parts[1]
	}

	// Connect to server
	conn, err := net.Dial("tcp", host)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	// Generate WebSocket key
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		conn.Close()
		return nil, fmt.Errorf("generate key: %w", err)
	}
	wsKey := base64.StdEncoding.EncodeToString(key)

	// Build handshake request
	req := fmt.Sprintf("GET %s HTTP/1.1\r\n", path)
	req += fmt.Sprintf("Host: %s\r\n", host)
	req += "Upgrade: websocket\r\n"
	req += "Connection: Upgrade\r\n"
	req += fmt.Sprintf("Sec-WebSocket-Key: %s\r\n", wsKey)
	req += "Sec-WebSocket-Version: 13\r\n"
	req += "\r\n"

	// Send handshake
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write handshake: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read response: %w", err)
	}
	defer resp.Body.Close()

	// Verify status code
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("handshake failed: status %d", resp.StatusCode)
	}

	// Create WebSocket connection using export test helper
	wsConn := NewConnForTest(conn, reader, false)
	return wsConn, nil
}

// TestIntegration_SSE_WebSocket_SameServer verifies SSE and WebSocket work independently on same server.
func TestIntegration_SSE_WebSocket_SameServer(t *testing.T) {
	// Setup server with both SSE and WebSocket endpoints
	mux := http.NewServeMux()

	// SSE endpoint - sends events every 50ms
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		conn, err := sse.Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		count := 0
		for {
			select {
			case <-ticker.C:
				count++
				event := sse.NewEvent(fmt.Sprintf("event-%d", count)).WithType("test")
				if err := conn.Send(event); err != nil {
					return
				}
				if count >= 5 {
					return
				}
			case <-conn.Done():
				return
			}
		}
	})

	// WebSocket endpoint - echo server
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
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
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Test with 5 concurrent SSE clients + 5 concurrent WS clients
	const numClients = 5
	var wg sync.WaitGroup
	wg.Add(numClients * 2)

	// SSE clients
	sseErrors := make(chan error, numClients)
	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()

			req, err := http.NewRequest("GET", server.URL+"/sse", http.NoBody)
			if err != nil {
				sseErrors <- fmt.Errorf("SSE client %d: request error: %w", clientID, err)
				return
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				sseErrors <- fmt.Errorf("SSE client %d: connection error: %w", clientID, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				sseErrors <- fmt.Errorf("SSE client %d: status %d", clientID, resp.StatusCode)
				return
			}

			scanner := bufio.NewScanner(resp.Body)
			eventsReceived := 0
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data:") {
					eventsReceived++
					if eventsReceived >= 5 {
						return
					}
				}
			}

			if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
				sseErrors <- fmt.Errorf("SSE client %d: scan error: %w", clientID, err)
			}
		}(i)
	}

	// WebSocket clients
	wsErrors := make(chan error, numClients)
	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()

			wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
			conn, err := dialWebSocket(context.Background(), wsURL)
			if err != nil {
				wsErrors <- fmt.Errorf("WS client %d: dial error: %w", clientID, err)
				return
			}
			defer conn.Close()

			// Send 5 messages and expect echoes
			for j := 0; j < 5; j++ {
				testMsg := []byte(fmt.Sprintf("msg-%d-%d", clientID, j))
				if err := conn.Write(TextMessage, testMsg); err != nil {
					wsErrors <- fmt.Errorf("WS client %d: write error: %w", clientID, err)
					return
				}

				_, data, err := conn.Read()
				if err != nil {
					wsErrors <- fmt.Errorf("WS client %d: read error: %w", clientID, err)
					return
				}

				if !bytes.Equal(data, testMsg) {
					wsErrors <- fmt.Errorf("WS client %d: got %q, want %q", clientID, data, testMsg)
					return
				}
			}
		}(i)
	}

	// Wait for all clients to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Test timeout - clients did not complete")
	}

	// Check for errors
	close(sseErrors)
	close(wsErrors)

	for err := range sseErrors {
		t.Errorf("SSE error: %v", err)
	}
	for err := range wsErrors {
		t.Errorf("WebSocket error: %v", err)
	}
}

// TestIntegration_BroadcastToAll tests broadcasting to both SSE and WebSocket clients simultaneously.
func TestIntegration_BroadcastToAll(t *testing.T) {
	// Setup SSE Hub
	sseHub := sse.NewHub[string]()
	go sseHub.Run()
	defer sseHub.Close()

	// Setup WebSocket Hub
	wsHub := NewHub()
	go wsHub.Run()
	defer wsHub.Close()

	// Unified broadcast function
	broadcastToAll := func(msg string) {
		sseHub.Broadcast(msg)
		wsHub.BroadcastText(msg)
	}

	// Setup server
	mux := http.NewServeMux()

	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		conn, err := sse.Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		sseHub.Register(conn)
		defer sseHub.Unregister(conn)

		<-conn.Done()
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		wsHub.Register(conn)
		defer wsHub.Unregister(conn)

		// Keep connection alive
		for {
			if _, _, err := conn.Read(); err != nil {
				break
			}
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect 10 SSE clients
	const numSSE = 10
	sseReceivedMap := sync.Map{} // clientID -> count
	var sseWG sync.WaitGroup
	sseWG.Add(numSSE)

	for i := 0; i < numSSE; i++ {
		go func(clientID int) {
			defer sseWG.Done()

			req, _ := http.NewRequest("GET", server.URL+"/sse", http.NoBody)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("SSE client %d: connection error: %v", clientID, err)
				return
			}
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			count := 0
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data:") {
					count++
					sseReceivedMap.Store(clientID, count)
					if count >= 5 {
						return
					}
				}
			}
		}(i)
	}

	// Connect 10 WebSocket clients
	const numWS = 10
	wsReceivedMap := sync.Map{} // clientID -> count
	var wsWG sync.WaitGroup
	wsWG.Add(numWS)

	for i := 0; i < numWS; i++ {
		go func(clientID int) {
			defer wsWG.Done()

			wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
			conn, err := dialWebSocket(context.Background(), wsURL)
			if err != nil {
				t.Errorf("WS client %d: dial error: %v", clientID, err)
				return
			}
			defer conn.Close()

			count := 0
			for {
				_, data, err := conn.Read()
				if err != nil {
					return
				}
				if len(data) > 0 {
					count++
					wsReceivedMap.Store(clientID, count)
					if count >= 5 {
						return
					}
				}
			}
		}(i)
	}

	// Wait for all clients to connect
	time.Sleep(200 * time.Millisecond)

	// Broadcast 5 messages to all clients
	for i := 1; i <= 5; i++ {
		broadcastToAll(fmt.Sprintf("broadcast-%d", i))
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for all clients to receive messages
	sseDone := make(chan struct{})
	go func() {
		sseWG.Wait()
		close(sseDone)
	}()

	wsDone := make(chan struct{})
	go func() {
		wsWG.Wait()
		close(wsDone)
	}()

	select {
	case <-sseDone:
	case <-time.After(5 * time.Second):
		t.Fatal("SSE clients timeout")
	}

	select {
	case <-wsDone:
	case <-time.After(5 * time.Second):
		t.Fatal("WebSocket clients timeout")
	}

	// Verify all SSE clients received all messages
	for i := 0; i < numSSE; i++ {
		val, ok := sseReceivedMap.Load(i)
		if !ok {
			t.Errorf("SSE client %d: no messages received", i)
			continue
		}
		if count := val.(int); count < 5 {
			t.Errorf("SSE client %d: received %d messages, want 5", i, count)
		}
	}

	// Verify all WebSocket clients received all messages
	for i := 0; i < numWS; i++ {
		val, ok := wsReceivedMap.Load(i)
		if !ok {
			t.Errorf("WS client %d: no messages received", i)
			continue
		}
		if count := val.(int); count < 5 {
			t.Errorf("WS client %d: received %d messages, want 5", i, count)
		}
	}
}

// TestIntegration_WebSocket_To_SSE_Relay tests relaying WebSocket messages to SSE clients.
func TestIntegration_WebSocket_To_SSE_Relay(t *testing.T) {
	// Setup SSE Hub for broadcasting
	sseHub := sse.NewHub[string]()
	go sseHub.Run()
	defer sseHub.Close()

	// Message relay function (WebSocket -> SSE)
	var messagesSent atomic.Int32

	// Setup server
	mux := http.NewServeMux()

	// SSE endpoint - clients receive relayed messages
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		conn, err := sse.Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		sseHub.Register(conn)
		defer sseHub.Unregister(conn)

		<-conn.Done()
	})

	// WebSocket endpoint - messages are relayed to SSE
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		for {
			_, data, err := conn.Read()
			if err != nil {
				break
			}

			// Relay to SSE clients
			message := string(data)
			sseHub.Broadcast(message)
			messagesSent.Add(1)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect 5 SSE clients
	const numSSE = 5
	sseReceived := make([][]string, numSSE)
	var sseMu sync.Mutex
	var sseWG sync.WaitGroup
	sseWG.Add(numSSE)

	for i := 0; i < numSSE; i++ {
		go func(clientID int) {
			defer sseWG.Done()

			req, _ := http.NewRequest("GET", server.URL+"/sse", http.NoBody)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("SSE client %d: connection error: %v", clientID, err)
				return
			}
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data:") {
					data := strings.TrimPrefix(line, "data:")
					data = strings.TrimSpace(data)

					sseMu.Lock()
					sseReceived[clientID] = append(sseReceived[clientID], data)
					if len(sseReceived[clientID]) >= 10 {
						sseMu.Unlock()
						return
					}
					sseMu.Unlock()
				}
			}
		}(i)
	}

	// Wait for SSE clients to connect
	time.Sleep(200 * time.Millisecond)

	// Connect 3 WebSocket clients and send messages
	const numWS = 3
	const msgsPerWS = 10
	var wsWG sync.WaitGroup
	wsWG.Add(numWS)

	for i := 0; i < numWS; i++ {
		go func(clientID int) {
			defer wsWG.Done()

			wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
			conn, err := dialWebSocket(context.Background(), wsURL)
			if err != nil {
				t.Errorf("WS client %d: dial error: %v", clientID, err)
				return
			}
			defer conn.Close()

			// Send 10 messages
			for j := 0; j < msgsPerWS; j++ {
				msg := fmt.Sprintf("ws-%d-msg-%d", clientID, j)
				if err := conn.Write(TextMessage, []byte(msg)); err != nil {
					t.Errorf("WS client %d: write error: %v", clientID, err)
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Wait for WebSocket clients to send all messages
	wsWG.Wait()
	time.Sleep(200 * time.Millisecond)

	// Verify all messages were sent
	sent := messagesSent.Load()
	expectedSent := int32(numWS * msgsPerWS)
	if sent != expectedSent {
		t.Errorf("Messages sent = %d, want %d", sent, expectedSent)
	}

	// Wait for SSE clients to receive messages
	sseDone := make(chan struct{})
	go func() {
		sseWG.Wait()
		close(sseDone)
	}()

	select {
	case <-sseDone:
	case <-time.After(5 * time.Second):
		t.Fatal("SSE clients timeout")
	}

	// Verify each SSE client received messages
	for i := 0; i < numSSE; i++ {
		sseMu.Lock()
		received := len(sseReceived[i])
		sseMu.Unlock()

		if received < 10 {
			t.Errorf("SSE client %d: received %d messages, want at least 10", i, received)
		}
	}

	t.Logf("Relay test completed: %d messages sent, SSE clients received successfully", sent)
}

// Message represents a test message for JSON serialization.
type Message struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// TestIntegration_JSON_Communication tests JSON communication across SSE and WebSocket.
func TestIntegration_JSON_Communication(t *testing.T) {
	// Setup WebSocket Hub
	wsHub := NewHub()
	go wsHub.Run()
	defer wsHub.Close()

	// Setup server
	mux := http.NewServeMux()

	// WebSocket endpoint - receives and broadcasts JSON
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		wsHub.Register(conn)
		defer wsHub.Unregister(conn)

		for {
			_, data, err := conn.Read()
			if err != nil {
				break
			}

			// Parse and rebroadcast
			var msg Message
			if err := json.Unmarshal(data, &msg); err == nil {
				wsHub.Broadcast(data)
			}
		}
	})

	// SSE endpoint - sends JSON events
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		conn, err := sse.Upgrade(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		// Send JSON events
		for i := 1; i <= 5; i++ {
			msg := Message{ID: i, Text: fmt.Sprintf("SSE message %d", i)}
			if err := conn.SendJSON(msg); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Test SSE JSON
	t.Run("SSE_JSON", func(t *testing.T) {
		req, _ := http.NewRequest("GET", server.URL+"/sse", http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("SSE connection error: %v", err)
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		messagesReceived := 0

		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimPrefix(line, "data:")
				data = strings.TrimSpace(data)

				var msg Message
				if err := json.Unmarshal([]byte(data), &msg); err != nil {
					t.Errorf("JSON unmarshal error: %v", err)
					continue
				}

				if msg.ID != messagesReceived+1 {
					t.Errorf("Message ID = %d, want %d", msg.ID, messagesReceived+1)
				}

				messagesReceived++
				if messagesReceived >= 5 {
					break
				}
			}
		}

		if messagesReceived != 5 {
			t.Errorf("Received %d messages, want 5", messagesReceived)
		}
	})

	// Test WebSocket JSON broadcast
	t.Run("WebSocket_JSON_Broadcast", func(t *testing.T) {
		// Connect 3 WebSocket clients
		const numClients = 3
		clients := make([]*Conn, numClients)

		for i := 0; i < numClients; i++ {
			wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
			conn, err := dialWebSocket(context.Background(), wsURL)
			if err != nil {
				t.Fatalf("Client %d dial error: %v", i, err)
			}
			clients[i] = conn
		}

		// Cleanup
		t.Cleanup(func() {
			for _, conn := range clients {
				if conn != nil {
					_ = conn.Close()
				}
			}
		})

		time.Sleep(100 * time.Millisecond)

		// First client sends JSON message
		msg := Message{ID: 100, Text: "Broadcast test"}
		data, _ := json.Marshal(msg)

		if err := clients[0].Write(TextMessage, data); err != nil {
			t.Fatalf("Write error: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		// All clients should receive the broadcast
		for i := 0; i < numClients; i++ {
			_, received, err := clients[i].Read()
			if err != nil {
				t.Errorf("Client %d read error: %v", i, err)
				continue
			}

			var receivedMsg Message
			if err := json.Unmarshal(received, &receivedMsg); err != nil {
				t.Errorf("Client %d unmarshal error: %v", i, err)
				continue
			}

			if receivedMsg.ID != 100 || receivedMsg.Text != "Broadcast test" {
				t.Errorf("Client %d received %+v, want ID=100", i, receivedMsg)
			}
		}
	})
}
