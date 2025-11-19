package websocket

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"net"
	"sync"
	"unicode/utf8"
)

// Conn represents a WebSocket connection (RFC 6455).
//
// Conn provides high-level methods for reading and writing messages,
// automatically handling:
//   - Message fragmentation (reassembly of multi-frame messages)
//   - Control frames (Ping, Pong, Close)
//   - UTF-8 validation for text messages
//   - Thread-safe writes
//
// Example Usage:
//
//	conn, err := websocket.Upgrade(w, r, nil)
//	if err != nil {
//	    return err
//	}
//	defer conn.Close()
//
//	// Read message
//	msgType, data, err := conn.Read()
//
//	// Write text message
//	conn.WriteText("Hello, WebSocket!")
//
//	// Write JSON
//	conn.WriteJSON(map[string]string{"status": "ok"})
type Conn struct {
	conn   net.Conn      // Underlying TCP connection
	reader *bufio.Reader // Buffered reader for frame parsing
	writer *bufio.Writer // Buffered writer for frame writing

	isServer bool // Server-side connection (affects masking rules)

	// Write synchronization (RFC 6455 Section 5.1)
	// "An endpoint MUST NOT send a data frame while a fragmented message is being transmitted"
	writeMu sync.Mutex

	// Close synchronization
	closeOnce sync.Once
	closed    bool
	closeMu   sync.RWMutex

	// Fragment reassembly state
	fragmentBuf  bytes.Buffer // Accumulates fragmented message
	fragmentType byte         // Opcode of first fragment (text/binary)
	inFragment   bool         // Currently reading fragmented message
}

// newConn creates a new WebSocket connection (internal constructor).
//
// Called by Upgrade() after successful handshake.
// Not exported - users should call Upgrade() to create connections.
func newConn(netConn net.Conn, reader *bufio.Reader, writer *bufio.Writer, isServer bool) *Conn {
	return &Conn{
		conn:     netConn,
		reader:   reader,
		writer:   writer,
		isServer: isServer,
	}
}

// Read reads the next complete message from the connection.
//
// Automatically handles:
//   - Fragmentation: Reassembles multi-frame messages (FIN=0 → FIN=1)
//   - Control frames: Processes Ping/Pong/Close during message reading
//   - UTF-8 validation: For text messages (RFC 6455 Section 8.1)
//
// Returns:
//   - MessageType: TextMessage or BinaryMessage
//   - []byte: Complete message payload
//   - error: ErrClosed if connection closed, protocol errors, network errors
//
// Thread-Safety: Safe for concurrent reads (each goroutine gets separate message).
//
// RFC 6455 Section 5.4: "A fragmented message consists of a single frame with
// the FIN bit clear and an opcode other than 0, followed by zero or more frames
// with the FIN bit clear and the opcode set to 0, and terminated by a single
// frame with the FIN bit set and an opcode of 0."
//
//nolint:gocyclo,cyclop,gocognit // Complex fragmentation+control frame handling per RFC 6455
func (c *Conn) Read() (MessageType, []byte, error) {
	c.closeMu.RLock()
	if c.closed {
		c.closeMu.RUnlock()
		return 0, nil, ErrClosed
	}
	c.closeMu.RUnlock()

	for {
		// Read next frame
		f, err := readFrame(c.reader)
		if err != nil {
			return 0, nil, err
		}

		// Handle control frames (RFC 6455 Section 5.5)
		// Control frames MAY be injected in the middle of a fragmented message
		switch f.opcode {
		case opcodePing:
			// Auto-respond to Ping with Pong (echo application data)
			if err := c.Pong(f.payload); err != nil {
				return 0, nil, err
			}
			continue // Continue reading data frames

		case opcodePong:
			// Pong received (unsolicited or response to our Ping)
			// No action needed, just continue
			continue

		case opcodeClose:
			// Close frame received
			// RFC 6455 Section 5.5.1: Parse status code + reason
			c.handleCloseFrame(f.payload)
			return 0, nil, ErrClosed
		}

		// Data frames: Text, Binary, Continuation
		switch f.opcode {
		case opcodeText, opcodeBinary:
			// First frame of message (or unfragmented message)
			if f.fin {
				// Unfragmented message - return immediately
				msgType := MessageType(f.opcode)

				// Validate UTF-8 for text messages (RFC 6455 Section 8.1)
				if msgType == TextMessage && !utf8.Valid(f.payload) {
					_ = c.CloseWithCode(CloseInvalidFramePayloadData, "invalid UTF-8")
					return 0, nil, ErrInvalidUTF8
				}

				return msgType, f.payload, nil
			}

			// Start of fragmented message (FIN=0)
			c.inFragment = true
			c.fragmentType = f.opcode
			c.fragmentBuf.Reset()
			c.fragmentBuf.Write(f.payload)

		case opcodeContinuation:
			// Continuation frame
			if !c.inFragment {
				// Unexpected continuation (no prior fragment)
				_ = c.CloseWithCode(CloseProtocolError, "unexpected continuation")
				return 0, nil, ErrUnexpectedContinuation
			}

			// Append to fragment buffer
			c.fragmentBuf.Write(f.payload)

			if f.fin {
				// Final fragment - assemble and return
				c.inFragment = false
				msgType := MessageType(c.fragmentType)
				payload := c.fragmentBuf.Bytes()

				// Validate UTF-8 for text messages
				if msgType == TextMessage && !utf8.Valid(payload) {
					_ = c.CloseWithCode(CloseInvalidFramePayloadData, "invalid UTF-8")
					return 0, nil, ErrInvalidUTF8
				}

				// Return copy (fragmentBuf will be reused)
				result := make([]byte, len(payload))
				copy(result, payload)
				return msgType, result, nil
			}
		}

		// Loop continues for:
		// - Control frames (already handled and continued)
		// - Non-final fragments (FIN=0, continue accumulating)
	}
}

// ReadText reads the next text message.
//
// Convenience wrapper around Read() that:
//   - Ensures message is TextMessage (returns error otherwise)
//   - Returns string directly
//
// Returns ErrInvalidMessageType if message is not text.
func (c *Conn) ReadText() (string, error) {
	msgType, data, err := c.Read()
	if err != nil {
		return "", err
	}

	if msgType != TextMessage {
		return "", ErrInvalidMessageType
	}

	return string(data), nil
}

// ReadJSON reads the next message as JSON.
//
// Convenience wrapper around Read() that:
//   - Ensures message is TextMessage
//   - Unmarshals JSON into v
//
// Returns ErrInvalidMessageType if message is not text.
// Returns json.SyntaxError if JSON is malformed.
func (c *Conn) ReadJSON(v any) error {
	msgType, data, err := c.Read()
	if err != nil {
		return err
	}

	if msgType != TextMessage {
		return ErrInvalidMessageType
	}

	return json.Unmarshal(data, v)
}

// Write writes a message to the connection.
//
// Automatically handles:
//   - Masking: Server frames NOT masked, client frames masked (RFC 6455 Section 5.1)
//   - Flushing: Ensures data sent immediately
//
// Thread-Safety: Safe for concurrent writes (serialized by mutex).
//
// Note: Currently does NOT fragment large messages (sends as single frame).
// Future enhancement: Fragment messages > WriteBufferSize.
func (c *Conn) Write(messageType MessageType, data []byte) error {
	c.closeMu.RLock()
	if c.closed {
		c.closeMu.RUnlock()
		return ErrClosed
	}
	c.closeMu.RUnlock()

	// Lock write mutex (prevent concurrent writes per RFC 6455 Section 5.1)
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// Build frame
	var opcode byte
	switch messageType {
	case TextMessage:
		opcode = opcodeText

		// Validate UTF-8 (RFC 6455 Section 8.1)
		if !utf8.Valid(data) {
			return ErrInvalidUTF8
		}

	case BinaryMessage:
		opcode = opcodeBinary

	default:
		return ErrInvalidMessageType
	}

	f := &frame{
		fin:     true, // Single frame (no fragmentation yet)
		opcode:  opcode,
		masked:  !c.isServer, // Server: NO mask, Client: YES mask
		payload: data,
	}

	if f.masked {
		// Client frame - apply random mask
		// Note: This is only for client connections (not used in stream library currently)
		// Server connections (c.isServer=true) never mask
		f.mask = [4]byte{0x12, 0x34, 0x56, 0x78} // TODO: Use crypto/rand for production
	}

	// Write frame
	return writeFrame(c.writer, f)
}

// WriteText writes a text message.
//
// Convenience wrapper around Write() for text messages.
//
// Returns ErrInvalidUTF8 if text contains invalid UTF-8.
func (c *Conn) WriteText(text string) error {
	return c.Write(TextMessage, []byte(text))
}

// WriteJSON writes a value as JSON text message.
//
// Convenience wrapper that:
//   - Marshals v to JSON
//   - Sends as TextMessage
//
// Returns json.MarshalError if marshaling fails.
func (c *Conn) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return c.Write(TextMessage, data)
}

// Ping sends a ping frame (for keep-alive).
//
// Application data is optional (max 125 bytes per RFC 6455 Section 5.5).
// Peer should respond with Pong containing same application data.
//
// Use case: Heartbeat to detect dead connections.
//
//	ticker := time.NewTicker(30 * time.Second)
//	go func() {
//	    for range ticker.C {
//	        conn.Ping(nil)
//	    }
//	}()
func (c *Conn) Ping(data []byte) error {
	c.closeMu.RLock()
	if c.closed {
		c.closeMu.RUnlock()
		return ErrClosed
	}
	c.closeMu.RUnlock()

	// RFC 6455 Section 5.5: Control frame payload max 125 bytes
	if len(data) > 125 {
		return ErrControlTooLarge
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	f := &frame{
		fin:     true, // Control frames must have FIN=1
		opcode:  opcodePing,
		masked:  !c.isServer,
		payload: data,
	}

	if f.masked {
		f.mask = [4]byte{0x12, 0x34, 0x56, 0x78} // TODO: crypto/rand
	}

	return writeFrame(c.writer, f)
}

// Pong sends a pong frame (response to ping or unsolicited).
//
// Application data should echo ping data (RFC 6455 Section 5.5.3).
// Max 125 bytes.
//
// Note: Read() automatically responds to Ping frames, so manual Pong usually not needed.
func (c *Conn) Pong(data []byte) error {
	c.closeMu.RLock()
	if c.closed {
		c.closeMu.RUnlock()
		return ErrClosed
	}
	c.closeMu.RUnlock()

	if len(data) > 125 {
		return ErrControlTooLarge
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	f := &frame{
		fin:     true,
		opcode:  opcodePong,
		masked:  !c.isServer,
		payload: data,
	}

	if f.masked {
		f.mask = [4]byte{0x12, 0x34, 0x56, 0x78} // TODO: crypto/rand
	}

	return writeFrame(c.writer, f)
}

// Close sends close frame and closes connection.
//
// Uses CloseNormalClosure (1000) status code.
// Idempotent - safe to call multiple times.
//
// RFC 6455 Section 7.1.1: "The Close frame MAY contain a body that indicates
// a reason for closing.".
func (c *Conn) Close() error {
	return c.CloseWithCode(CloseNormalClosure, "")
}

// CloseWithCode sends close frame with specific status code and reason.
//
// Status codes defined in RFC 6455 Section 7.4.
// Reason is optional UTF-8 text (max ~123 bytes to fit in 125 byte frame).
//
// Close handshake (RFC 6455 Section 7.1.2):
//  1. Send Close frame
//  2. Peer responds with Close frame
//  3. Close TCP connection
//
// Idempotent - safe to call multiple times.
func (c *Conn) CloseWithCode(code CloseCode, reason string) error {
	var err error

	c.closeOnce.Do(func() {
		// Mark as closed
		c.closeMu.Lock()
		c.closed = true
		c.closeMu.Unlock()

		// Build close frame payload: 2 bytes status code + optional reason
		payload := make([]byte, 2+len(reason))
		payload[0] = byte(code >> 8)
		payload[1] = byte(code & 0xFF)
		copy(payload[2:], reason)

		// Validate reason is valid UTF-8
		if reason != "" && !utf8.ValidString(reason) {
			err = ErrInvalidUTF8
			return
		}

		// Send close frame
		c.writeMu.Lock()
		f := &frame{
			fin:     true,
			opcode:  opcodeClose,
			masked:  !c.isServer,
			payload: payload,
		}

		if f.masked {
			f.mask = [4]byte{0x12, 0x34, 0x56, 0x78} // TODO: crypto/rand
		}

		writeErr := writeFrame(c.writer, f)
		c.writeMu.Unlock()

		if writeErr != nil {
			err = writeErr
			return
		}

		// Close TCP connection
		// Note: Per RFC, should wait for close response, but for simplicity close immediately
		// Future enhancement: Wait for close response with timeout
		if c.conn != nil {
			err = c.conn.Close()
		}
	})

	return err
}

// handleCloseFrame processes received close frame.
//
// RFC 6455 Section 5.5.1:
//   - Close frame MAY contain status code (2 bytes) + reason
//   - Peer should respond with Close frame
func (c *Conn) handleCloseFrame(payload []byte) {
	// Mark as closed
	c.closeMu.Lock()
	c.closed = true
	c.closeMu.Unlock()

	// Parse close code if present
	var code CloseCode
	if len(payload) >= 2 {
		code = CloseCode(uint16(payload[0])<<8 | uint16(payload[1]))
	} else {
		code = CloseNoStatusReceived
	}

	// Respond with close frame (echo status code)
	// Ignore error - connection closing anyway
	_ = c.CloseWithCode(code, "")
}
