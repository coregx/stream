package websocket

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
)

// DialOptions contains options for WebSocket client connection.
type DialOptions struct {
	Header           http.Header
	Subprotocols     []string
	CheckOrigin      bool
	HandshakeTimeout int // seconds
}

// Dial connects to a WebSocket server and performs the opening handshake.
//
// This is a simple client implementation for testing purposes.
// For production use, consider using a full-featured WebSocket client library.
func Dial(_ context.Context, url string, opts *DialOptions) (*Conn, *http.Response, error) {
	if opts == nil {
		opts = &DialOptions{}
	}

	// Parse URL
	if !strings.HasPrefix(url, "ws://") && !strings.HasPrefix(url, "wss://") {
		return nil, nil, fmt.Errorf("invalid WebSocket URL scheme: %s", url)
	}

	// For testing, only support ws:// (not wss://)
	if strings.HasPrefix(url, "wss://") {
		return nil, nil, fmt.Errorf("wss:// not supported in test client")
	}

	// Extract host and path
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
		return nil, nil, fmt.Errorf("dial failed: %w", err)
	}

	// Generate WebSocket key
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}
	wsKey := base64.StdEncoding.EncodeToString(key)

	// Build handshake request
	req := fmt.Sprintf("GET %s HTTP/1.1\r\n", path)
	req += fmt.Sprintf("Host: %s\r\n", host)
	req += "Upgrade: websocket\r\n"
	req += "Connection: Upgrade\r\n"
	req += fmt.Sprintf("Sec-WebSocket-Key: %s\r\n", wsKey)
	req += "Sec-WebSocket-Version: 13\r\n"

	if len(opts.Subprotocols) > 0 {
		req += "Sec-WebSocket-Protocol: " + strings.Join(opts.Subprotocols, ", ") + "\r\n"
	}

	// Add custom headers
	if opts.Header != nil {
		for key, values := range opts.Header {
			for _, value := range values {
				req += fmt.Sprintf("%s: %s\r\n", key, value)
			}
		}
	}

	req += "\r\n"

	// Send handshake
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("write handshake: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("read response: %w", err)
	}
	defer resp.Body.Close()

	// Verify status code
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, resp, fmt.Errorf("handshake failed: status %d", resp.StatusCode)
	}

	// Verify Upgrade header
	if strings.ToLower(resp.Header.Get("Upgrade")) != "websocket" {
		conn.Close()
		return nil, resp, fmt.Errorf("invalid Upgrade header: %s", resp.Header.Get("Upgrade"))
	}

	// Create WebSocket connection
	wsConn := &Conn{
		conn:     conn,
		reader:   reader,
		writer:   bufio.NewWriter(conn),
		isServer: false, // This is a client connection
	}

	return wsConn, resp, nil
}

// dialTestServer is a helper function for tests to dial a test server.
func dialTestServer(tb interface {
	Helper()
	Fatalf(string, ...any)
}, server *httptest.Server) *Conn {
	tb.Helper()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, resp, err := Dial(context.Background(), wsURL, nil)
	if err != nil {
		tb.Fatalf("Dial error: %v", err)
	}
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	return conn
}

// newTestServer is a helper to create test HTTP server with WebSocket handler.
func newTestServer(tb interface{ Helper() }, handler func(*Conn)) *httptest.Server {
	tb.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()
		handler(conn)
	}))

	return server
}
