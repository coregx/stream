package websocket_test

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coregx/stream/websocket"
)

// TestIntegration_WebSocketUpgrade tests full handshake with real HTTP server.
//
// This test is critical for achieving >90% coverage of Upgrade() function,
// since httptest.ResponseRecorder doesn't support hijacking.
func TestIntegration_WebSocketUpgrade(t *testing.T) {
	// Create HTTP server with WebSocket upgrade handler
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Get low-level reader/writer for frame testing
		reader := websocket.GetReaderForTest(conn)
		writer := websocket.GetWriterForTest(conn)

		// Echo server: read one frame, write it back
		// Note: readFrame already unmasks incoming frames
		frame, err := websocket.ReadFrameForTest(reader)
		if err != nil {
			t.Logf("ReadFrame error: %v", err)
			return
		}

		// Server must NOT mask frames (RFC 6455 Section 5.1)
		frame.Masked = false
		frame.Mask = [4]byte{}

		if err := websocket.WriteFrameForTest(writer, frame); err != nil {
			t.Logf("WriteFrame error: %v", err)
		}
	}))
	server.Start()
	defer server.Close()

	// Extract host from server URL
	host := strings.TrimPrefix(server.URL, "http://")

	// Connect as WebSocket client
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send WebSocket handshake request
	handshake := "GET / HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	if _, err := conn.Write([]byte(handshake)); err != nil {
		t.Fatalf("Write handshake failed: %v", err)
	}

	// Read HTTP response
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil {
		t.Fatalf("ReadResponse failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify 101 Switching Protocols
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 101 Switching Protocols, got %d: %s", resp.StatusCode, body)
	}

	// Verify Upgrade header
	if got := resp.Header.Get("Upgrade"); got != "websocket" {
		t.Errorf("Upgrade header = %q, want %q", got, "websocket")
	}

	// Verify Connection header
	if got := resp.Header.Get("Connection"); got != "Upgrade" {
		t.Errorf("Connection header = %q, want %q", got, "Upgrade")
	}

	// Verify Sec-WebSocket-Accept (RFC 6455 example)
	expectedAccept := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != expectedAccept {
		t.Errorf("Sec-WebSocket-Accept = %q, want %q", got, expectedAccept)
	}

	// Now connection is upgraded - test frame exchange
	// Send a text frame (masked, as required for client-to-server)
	testPayload := []byte("Hello, WebSocket!")

	// Create frame with unmasked payload
	// Note: writeFrame will apply the mask automatically
	frame := &websocket.FrameForTest{
		Fin:     true,
		Opcode:  websocket.OpcodeTextForTest,
		Masked:  true,
		Mask:    [4]byte{0x12, 0x34, 0x56, 0x78},
		Payload: testPayload,
	}

	// Write frame to server
	writer := bufio.NewWriter(conn)
	if err := websocket.WriteFrameForTest(writer, frame); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	// Read echo back (server frames are unmasked)
	// Note: readFrame automatically unmasks incoming frames
	echoFrame, err := websocket.ReadFrameForTest(reader)
	if err != nil {
		t.Fatalf("ReadFrame echo failed: %v", err)
	}

	// Verify echoed frame
	if !echoFrame.Fin {
		t.Error("Expected FIN=1 in echo")
	}
	if echoFrame.Opcode != websocket.OpcodeTextForTest {
		t.Errorf("Opcode = %d, want %d", echoFrame.Opcode, websocket.OpcodeTextForTest)
	}
	if echoFrame.Masked {
		t.Error("Server frame should not be masked")
	}

	// Verify payload (echo server received unmasked payload and sent it back)
	if !bytes.Equal(echoFrame.Payload, testPayload) {
		t.Errorf("Echo payload = %q, want %q", echoFrame.Payload, testPayload)
	}
}

// TestIntegration_OriginCheck tests origin validation during handshake.
func TestIntegration_OriginCheck(t *testing.T) {
	allowedOrigin := "https://example.com"

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opts := &websocket.UpgradeOptions{
			CheckOrigin: func(req *http.Request) bool {
				origin := req.Header.Get("Origin")
				return origin == allowedOrigin
			},
		}

		_, err := websocket.Upgrade(w, r, opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}))
	server.Start()
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")

	tests := []struct {
		name       string
		origin     string
		wantStatus int
	}{
		{"allowed origin", allowedOrigin, http.StatusSwitchingProtocols},
		{"different origin", "https://evil.com", http.StatusForbidden},
		{"no origin", "", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := net.Dial("tcp", host)
			if err != nil {
				t.Fatalf("Dial failed: %v", err)
			}
			defer conn.Close()

			handshake := "GET / HTTP/1.1\r\n" +
				"Host: " + host + "\r\n" +
				"Upgrade: websocket\r\n" +
				"Connection: Upgrade\r\n" +
				"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
				"Sec-WebSocket-Version: 13\r\n"

			if tt.origin != "" {
				handshake += "Origin: " + tt.origin + "\r\n"
			}
			handshake += "\r\n"

			if _, err := conn.Write([]byte(handshake)); err != nil {
				t.Fatalf("Write handshake failed: %v", err)
			}

			reader := bufio.NewReader(conn)
			resp, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
			if err != nil {
				t.Fatalf("ReadResponse failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("Status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

// TestIntegration_SubprotocolNegotiation tests subprotocol selection.
func TestIntegration_SubprotocolNegotiation(t *testing.T) {
	serverProtos := []string{"chat", "superchat"}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opts := &websocket.UpgradeOptions{
			Subprotocols: serverProtos,
		}

		_, err := websocket.Upgrade(w, r, opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}))
	server.Start()
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")

	tests := []struct {
		name            string
		clientProtos    string
		wantSubprotocol string
	}{
		{"first match", "chat, superchat", "chat"},
		{"second match", "mqtt, superchat", "superchat"},
		{"no match", "mqtt, amqp", ""},
		{"no client protos", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := net.Dial("tcp", host)
			if err != nil {
				t.Fatalf("Dial failed: %v", err)
			}
			defer conn.Close()

			handshake := "GET / HTTP/1.1\r\n" +
				"Host: " + host + "\r\n" +
				"Upgrade: websocket\r\n" +
				"Connection: Upgrade\r\n" +
				"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
				"Sec-WebSocket-Version: 13\r\n"

			if tt.clientProtos != "" {
				handshake += "Sec-WebSocket-Protocol: " + tt.clientProtos + "\r\n"
			}
			handshake += "\r\n"

			if _, err := conn.Write([]byte(handshake)); err != nil {
				t.Fatalf("Write handshake failed: %v", err)
			}

			reader := bufio.NewReader(conn)
			resp, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
			if err != nil {
				t.Fatalf("ReadResponse failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("Expected 101, got %d", resp.StatusCode)
			}

			got := resp.Header.Get("Sec-WebSocket-Protocol")
			if got != tt.wantSubprotocol {
				t.Errorf("Subprotocol = %q, want %q", got, tt.wantSubprotocol)
			}
		})
	}
}

// TestIntegration_BufferSizes tests custom buffer configuration.
func TestIntegration_BufferSizes(t *testing.T) {
	customReadSize := 8192
	customWriteSize := 16384

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opts := &websocket.UpgradeOptions{
			ReadBufferSize:  customReadSize,
			WriteBufferSize: customWriteSize,
		}

		conn, err := websocket.Upgrade(w, r, opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Get low-level reader/writer for frame testing
		reader := websocket.GetReaderForTest(conn)
		writer := websocket.GetWriterForTest(conn)

		// Verify buffer sizes (bufio doesn't expose size directly, but we can check it works)
		// Read and echo a frame
		frame, err := websocket.ReadFrameForTest(reader)
		if err != nil {
			t.Logf("ReadFrame error: %v", err)
			return
		}

		// Server must NOT mask frames
		frame.Masked = false
		frame.Mask = [4]byte{}

		if err := websocket.WriteFrameForTest(writer, frame); err != nil {
			t.Logf("WriteFrame error: %v", err)
		}
	}))
	server.Start()
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")

	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	handshake := "GET / HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	if _, err := conn.Write([]byte(handshake)); err != nil {
		t.Fatalf("Write handshake failed: %v", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil {
		t.Fatalf("ReadResponse failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("Expected 101, got %d", resp.StatusCode)
	}

	// Send frame and verify echo works (implicitly tests buffer sizes)
	payload := make([]byte, 1000)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	// Create frame (writeFrame will apply mask)
	frame := &websocket.FrameForTest{
		Fin:     true,
		Opcode:  websocket.OpcodeBinaryForTest,
		Masked:  true,
		Mask:    [4]byte{0xAA, 0xBB, 0xCC, 0xDD},
		Payload: payload,
	}

	writer := bufio.NewWriter(conn)
	if err := websocket.WriteFrameForTest(writer, frame); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	echoFrame, err := websocket.ReadFrameForTest(reader)
	if err != nil {
		t.Fatalf("ReadFrame echo failed: %v", err)
	}

	if len(echoFrame.Payload) != len(payload) {
		t.Errorf("Echo payload length = %d, want %d", len(echoFrame.Payload), len(payload))
	}

	// Verify payload matches original (unmasked)
	for i := range payload {
		if echoFrame.Payload[i] != payload[i] {
			t.Errorf("Payload mismatch at byte %d: got %d, want %d", i, echoFrame.Payload[i], payload[i])
			break
		}
	}
}

// TestIntegration_HubEchoServer tests full Hub-based echo server scenario.
//
// This test verifies:
//   - WebSocket upgrade with real HTTP server
//   - Hub registration/broadcasting
//   - Multiple clients receiving messages
//   - Client-to-hub-to-all-clients flow
func TestIntegration_HubEchoServer(t *testing.T) {
	// Create Hub
	hub := websocket.NewHub()
	go hub.Run()

	// Create HTTP server with WebSocket upgrade handler
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Register with hub
		hub.Register(conn)

		// Handle incoming messages and broadcast
		go func() {
			defer hub.Unregister(conn)
			for {
				_, data, err := conn.Read()
				if err != nil {
					break
				}
				// Broadcast to all clients (including sender)
				hub.Broadcast(data)
			}
		}()
	}))
	server.Start()

	host := strings.TrimPrefix(server.URL, "http://")

	// Connect 3 WebSocket clients
	const numClients = 3
	clients := make([]*testClient, numClients)
	for i := 0; i < numClients; i++ {
		client, err := connectWebSocketClient(t, host)
		if err != nil {
			t.Fatalf("Client %d connect failed: %v", i, err)
		}
		clients[i] = client
	}

	// Wait for all clients to register with hub
	waitForClientCount(t, hub, numClients, 100)

	// Client 0 sends a message
	testMessage := []byte("Hello from client 0")
	if err := clients[0].sendTextFrame(testMessage); err != nil {
		t.Fatalf("Client 0 send failed: %v", err)
	}

	// All clients should receive the broadcast
	for i := 0; i < numClients; i++ {
		received, err := clients[i].receiveFrame()
		if err != nil {
			t.Fatalf("Client %d receive failed: %v", i, err)
		}

		if !bytes.Equal(received, testMessage) {
			t.Errorf("Client %d received %q, want %q", i, received, testMessage)
		}
	}

	// Verify hub still has all clients
	if count := hub.ClientCount(); count != numClients {
		t.Errorf("Hub ClientCount() = %d, want %d", count, numClients)
	}

	// Close clients first to avoid "send on closed channel" from deferred Unregister
	for i := 0; i < numClients; i++ {
		clients[i].conn.Close()
	}

	// Close server before hub
	server.Close()

	// Wait a bit for unregistrations to complete
	time.Sleep(50 * time.Millisecond)

	// Now close hub
	hub.Close()
}

// TestIntegration_HubMultipleClients tests Hub with 10+ clients broadcasting.
//
// This test verifies:
//   - Concurrent client connections
//   - Hub handling multiple simultaneous broadcasts
//   - Message delivery to all clients
//   - No message loss or corruption
func TestIntegration_HubMultipleClients(t *testing.T) {
	// Create Hub
	hub := websocket.NewHub()
	go hub.Run()

	// Create HTTP server
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Register with hub
		hub.Register(conn)

		// Just keep connection alive (no reading needed for this test)
		go func() {
			defer hub.Unregister(conn)
			// Block until connection closes
			_, _, _ = conn.Read()
		}()
	}))
	server.Start()

	host := strings.TrimPrefix(server.URL, "http://")

	// Connect 15 clients
	const numClients = 15
	clients := make([]*testClient, numClients)
	for i := 0; i < numClients; i++ {
		client, err := connectWebSocketClient(t, host)
		if err != nil {
			t.Fatalf("Client %d connect failed: %v", i, err)
		}
		clients[i] = client
	}

	// Wait for all clients to register
	waitForClientCount(t, hub, numClients, 200)

	// Broadcast 5 messages from hub
	const numMessages = 5
	for msgNum := 0; msgNum < numMessages; msgNum++ {
		msg := []byte("Broadcast message " + string(rune('A'+msgNum)))
		hub.Broadcast(msg)

		// All clients should receive this message
		for clientNum := 0; clientNum < numClients; clientNum++ {
			received, err := clients[clientNum].receiveFrame()
			if err != nil {
				t.Fatalf("Message %d, Client %d receive failed: %v", msgNum, clientNum, err)
			}

			if !bytes.Equal(received, msg) {
				t.Errorf("Message %d, Client %d received %q, want %q", msgNum, clientNum, received, msg)
			}
		}
	}

	// Verify hub still has all clients
	if count := hub.ClientCount(); count != numClients {
		t.Errorf("Hub ClientCount() = %d, want %d", count, numClients)
	}

	// Close clients first to avoid "send on closed channel" from deferred Unregister
	for i := 0; i < numClients; i++ {
		clients[i].conn.Close()
	}

	// Close server before hub
	server.Close()

	// Wait for unregistrations to complete
	time.Sleep(50 * time.Millisecond)

	// Now close hub
	hub.Close()
}

// testClient wraps a WebSocket connection for integration testing.
type testClient struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

// connectWebSocketClient performs WebSocket handshake and returns connected client.
func connectWebSocketClient(t *testing.T, host string) (*testClient, error) {
	t.Helper()

	conn, err := net.Dial("tcp", host)
	if err != nil {
		return nil, err
	}

	// Send WebSocket handshake
	handshake := "GET / HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	if _, err := conn.Write([]byte(handshake)); err != nil {
		conn.Close()
		return nil, err
	}

	// Read handshake response
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil {
		conn.Close()
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, err
	}

	return &testClient{
		conn:   conn,
		reader: reader,
		writer: bufio.NewWriter(conn),
	}, nil
}

// sendTextFrame sends a text frame to the server.
func (c *testClient) sendTextFrame(payload []byte) error {
	frame := &websocket.FrameForTest{
		Fin:     true,
		Opcode:  websocket.OpcodeTextForTest,
		Masked:  true,
		Mask:    [4]byte{0x12, 0x34, 0x56, 0x78},
		Payload: payload,
	}

	return websocket.WriteFrameForTest(c.writer, frame)
}

// receiveFrame reads a frame from the server and returns its payload.
func (c *testClient) receiveFrame() ([]byte, error) {
	frame, err := websocket.ReadFrameForTest(c.reader)
	if err != nil {
		return nil, err
	}
	return frame.Payload, nil
}

// waitForClientCount waits for hub to have expected number of clients.
func waitForClientCount(t *testing.T, hub *websocket.Hub, expected, maxWaitMS int) {
	t.Helper()

	for i := 0; i < maxWaitMS; i++ {
		if hub.ClientCount() == expected {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("Timeout waiting for %d clients, got %d", expected, hub.ClientCount())
}
