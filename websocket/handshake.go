package websocket

import (
	"bufio"
	"crypto/sha1" // #nosec G505 - SHA-1 required by RFC 6455 Section 1.3
	"encoding/base64"
	"net/http"
	"strings"
)

// Magic GUID from RFC 6455 Section 1.3.
// Used for computing Sec-WebSocket-Accept header.
const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Default buffer sizes for WebSocket connections.
const (
	defaultReadBufferSize  = 4096
	defaultWriteBufferSize = 4096
)

// UpgradeOptions configures WebSocket upgrade behavior.
//
// All fields are optional. Zero values use sensible defaults.
type UpgradeOptions struct {
	// Subprotocols is the list of subprotocols advertised by server.
	// Server will select first match from client's requested subprotocols.
	// Empty list = no subprotocol negotiation.
	Subprotocols []string

	// CheckOrigin verifies the Origin header.
	// nil = allow all origins (INSECURE in production!)
	// Return false to reject the connection.
	//
	// Example:
	//   CheckOrigin: func(r *http.Request) bool {
	//       origin := r.Header.Get("Origin")
	//       return origin == "https://example.com"
	//   }
	CheckOrigin func(*http.Request) bool

	// ReadBufferSize sets size of read buffer (default: 4096).
	// Larger buffers reduce syscalls for large messages.
	ReadBufferSize int

	// WriteBufferSize sets size of write buffer (default: 4096).
	// Larger buffers reduce syscalls for large messages.
	WriteBufferSize int
}

// Upgrade upgrades an HTTP connection to the WebSocket protocol.
//
// Implements RFC 6455 Section 4: Opening Handshake.
//
// Steps:
//  1. Verify HTTP method is GET
//  2. Check Upgrade: websocket header
//  3. Check Connection: Upgrade header
//  4. Verify Sec-WebSocket-Version: 13
//  5. Get Sec-WebSocket-Key
//  6. Check origin (if configured)
//  7. Negotiate subprotocol (if configured)
//  8. Compute Sec-WebSocket-Accept
//  9. Send 101 Switching Protocols response
//  10. Hijack connection
//  11. Create and return WebSocket connection
//
// Returns *Conn for reading/writing WebSocket messages.
//
// Example:
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//	    conn, err := websocket.Upgrade(w, r, nil)
//	    if err != nil {
//	        http.Error(w, err.Error(), http.StatusBadRequest)
//	        return
//	    }
//	    defer conn.Close()
//
//	    // Read and write messages
//	    msgType, data, _ := conn.Read()
//	    conn.Write(msgType, data)
//	}
//
//nolint:gocyclo,cyclop // Handshake requires many validation steps per RFC 6455
func Upgrade(w http.ResponseWriter, r *http.Request, opts *UpgradeOptions) (*Conn, error) {
	// Apply defaults
	if opts == nil {
		opts = &UpgradeOptions{}
	}
	if opts.ReadBufferSize == 0 {
		opts.ReadBufferSize = defaultReadBufferSize
	}
	if opts.WriteBufferSize == 0 {
		opts.WriteBufferSize = defaultWriteBufferSize
	}

	// 1. Verify HTTP method (RFC 6455 Section 4.1)
	if r.Method != http.MethodGet {
		return nil, ErrInvalidMethod
	}

	// 2. Check Upgrade header (RFC 6455 Section 4.2.1, item 3)
	if !headerContainsToken(r.Header.Get("Upgrade"), "websocket") {
		return nil, ErrMissingUpgrade
	}

	// 3. Check Connection header (RFC 6455 Section 4.2.1, item 4)
	if !headerContainsToken(r.Header.Get("Connection"), "upgrade") {
		return nil, ErrMissingConnection
	}

	// 4. Check Sec-WebSocket-Version (RFC 6455 Section 4.2.1, item 6)
	version := r.Header.Get("Sec-WebSocket-Version")
	if version != "13" {
		return nil, ErrInvalidVersion
	}

	// 5. Get Sec-WebSocket-Key (RFC 6455 Section 4.2.1, item 5)
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, ErrMissingSecKey
	}

	// 6. Check origin (application-level security)
	if opts.CheckOrigin != nil && !opts.CheckOrigin(r) {
		return nil, ErrOriginDenied
	}

	// 7. Negotiate subprotocol (RFC 6455 Section 4.2.2, item 5)
	subprotocol := negotiateSubprotocol(r, opts.Subprotocols)

	// 8. Compute Sec-WebSocket-Accept (RFC 6455 Section 4.2.2, item 4)
	accept := computeAcceptKey(key)

	// 9. Send 101 Switching Protocols response
	w.Header().Set("Upgrade", "websocket")
	w.Header().Set("Connection", "Upgrade")
	w.Header().Set("Sec-WebSocket-Accept", accept)
	if subprotocol != "" {
		w.Header().Set("Sec-WebSocket-Protocol", subprotocol)
	}
	w.WriteHeader(http.StatusSwitchingProtocols)

	// 10. Hijack connection (take over TCP socket)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, ErrHijackFailed
	}

	netConn, bufrw, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}

	// Ensure connection is flushed (101 response sent)
	if err := bufrw.Flush(); err != nil {
		_ = netConn.Close() // Best effort close
		return nil, err
	}

	// 11. Create buffered readers/writers with configured sizes
	// Reuse existing reader if buffer is large enough
	var reader *bufio.Reader
	if bufrw.Reader.Size() >= opts.ReadBufferSize {
		reader = bufrw.Reader
	} else {
		reader = bufio.NewReaderSize(netConn, opts.ReadBufferSize)
	}

	// Always create new writer with configured size
	writer := bufio.NewWriterSize(netConn, opts.WriteBufferSize)

	// 12. Create WebSocket connection (server-side)
	conn := newConn(netConn, reader, writer, true)

	return conn, nil
}

// computeAcceptKey computes Sec-WebSocket-Accept from client key.
//
// RFC 6455 Section 1.3:
//
//	Sec-WebSocket-Accept = base64(SHA-1(key + GUID))
//
// Where GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11".
//
// Example:
//
//	key := "dGhlIHNhbXBsZSBub25jZQ=="
//	accept := computeAcceptKey(key)
//	// accept = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
func computeAcceptKey(key string) string {
	// #nosec G401 - SHA-1 required by RFC 6455 Section 1.3 (not for cryptographic security)
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// negotiateSubprotocol selects first match from client's requested subprotocols.
//
// RFC 6455 Section 1.9: Server selects ONE subprotocol from client's list.
//
// Returns empty string if no match or no subprotocols configured.
func negotiateSubprotocol(r *http.Request, serverProtos []string) string {
	if len(serverProtos) == 0 {
		return ""
	}

	clientProtos := strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",")
	for _, clientProto := range clientProtos {
		clientProto = strings.TrimSpace(clientProto)
		for _, serverProto := range serverProtos {
			if clientProto == serverProto {
				return clientProto
			}
		}
	}

	return ""
}

// headerContainsToken checks if header value contains token (case-insensitive).
//
// RFC 6455 Section 4.2.1: Header tokens are case-insensitive.
//
// Example:
//
//	headerContainsToken("Upgrade, HTTP/2.0", "upgrade") // true
//	headerContainsToken("keep-alive", "upgrade")        // false
func headerContainsToken(header, token string) bool {
	header = strings.ToLower(header)
	token = strings.ToLower(token)

	for _, h := range strings.Split(header, ",") {
		if strings.TrimSpace(h) == token {
			return true
		}
	}

	return false
}

// checkSameOrigin returns true if Origin header matches request host.
//
// Default origin checker for production use.
//
// Usage:
//
//	opts := &UpgradeOptions{
//	    CheckOrigin: websocket.CheckSameOrigin,
//	}
func checkSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No Origin header = non-browser client (e.g., curl, Go client)
		return true
	}

	// Build expected origin from request
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	expectedOrigin := scheme + "://" + r.Host

	return origin == expectedOrigin
}
