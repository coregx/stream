package websocket

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// mockConn creates a mock connection with pre-written frames.
func mockConn(t *testing.T, frames []*frame, isServer bool) *Conn {
	t.Helper()

	// Write frames to buffer
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	for _, f := range frames {
		if err := writeFrame(w, f); err != nil {
			t.Fatalf("mockConn writeFrame error: %v", err)
		}
	}
	w.Flush()

	// Create connection with buffer as reader
	reader := bufio.NewReader(&buf)
	writer := bufio.NewWriter(io.Discard) // Writes go nowhere
	return newConn(nil, reader, writer, isServer)
}

// mockConnNoValidation creates a mock connection with frames (no validation).
//
// Used for testing edge cases (invalid UTF-8, protocol violations).
func mockConnNoValidation(t *testing.T, frames []*frame, isServer bool) *Conn {
	t.Helper()

	// Write frames to buffer without validation
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	for _, f := range frames {
		if err := writeFrameNoValidation(w, f); err != nil {
			t.Fatalf("mockConnNoValidation writeFrame error: %v", err)
		}
	}
	w.Flush()

	// Create connection with buffer as reader
	reader := bufio.NewReader(&buf)
	writer := bufio.NewWriter(io.Discard) // Writes go nowhere
	return newConn(nil, reader, writer, isServer)
}

// mockConnWriter creates a mock connection that captures writes.
//
// Always creates server-side connection (isServer=true, no masking).
func mockConnWriter(t *testing.T) (*Conn, *bytes.Buffer) {
	t.Helper()

	var writeBuf bytes.Buffer
	reader := bufio.NewReader(bytes.NewReader(nil)) // Empty reader
	writer := bufio.NewWriter(&writeBuf)
	conn := newConn(nil, reader, writer, true) // Server-side
	return conn, &writeBuf
}

// TestConn_Read tests basic message reading.
func TestConn_Read(t *testing.T) {
	tests := []struct {
		name        string
		frames      []*frame
		wantType    MessageType
		wantPayload string
		wantErr     error
	}{
		{
			name: "unfragmented text message",
			frames: []*frame{
				{fin: true, opcode: opcodeText, payload: []byte("Hello, World!")},
			},
			wantType:    TextMessage,
			wantPayload: "Hello, World!",
		},
		{
			name: "unfragmented binary message",
			frames: []*frame{
				{fin: true, opcode: opcodeBinary, payload: []byte{0x01, 0x02, 0x03}},
			},
			wantType:    BinaryMessage,
			wantPayload: "\x01\x02\x03",
		},
		{
			name: "invalid UTF-8 in text message",
			frames: []*frame{
				{fin: true, opcode: opcodeText, payload: []byte{0xFF, 0xFE}},
			},
			wantErr: ErrInvalidUTF8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use mockConnNoValidation for tests expecting errors
			var conn *Conn
			if tt.wantErr != nil {
				conn = mockConnNoValidation(t, tt.frames, false)
			} else {
				conn = mockConn(t, tt.frames, false)
			}

			msgType, payload, err := conn.Read()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Read() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Read() unexpected error: %v", err)
			}

			if msgType != tt.wantType {
				t.Errorf("Read() msgType = %v, want %v", msgType, tt.wantType)
			}

			if string(payload) != tt.wantPayload {
				t.Errorf("Read() payload = %q, want %q", payload, tt.wantPayload)
			}
		})
	}
}

// TestConn_ReadFragmented tests fragmented message reassembly.
func TestConn_ReadFragmented(t *testing.T) {
	frames := []*frame{
		{fin: false, opcode: opcodeText, payload: []byte("Hello, ")},
		{fin: false, opcode: opcodeContinuation, payload: []byte("World")},
		{fin: true, opcode: opcodeContinuation, payload: []byte("!")},
	}

	conn := mockConn(t, frames, false)

	msgType, payload, err := conn.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if msgType != TextMessage {
		t.Errorf("msgType = %v, want TextMessage", msgType)
	}

	want := "Hello, World!"
	if string(payload) != want {
		t.Errorf("payload = %q, want %q", payload, want)
	}
}

// TestConn_ReadControlDuringFragmentation tests control frames during fragmented message.
func TestConn_ReadControlDuringFragmentation(t *testing.T) {
	// Fragmented message with PING in the middle
	frames := []*frame{
		{fin: false, opcode: opcodeText, payload: []byte("Part1")},
		{fin: true, opcode: opcodePing, payload: []byte("ping")}, // Control frame
		{fin: true, opcode: opcodeContinuation, payload: []byte("Part2")},
	}

	conn := mockConn(t, frames, true) // server-side

	// Note: Pong will be written but we're using io.Discard writer
	msgType, payload, err := conn.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if msgType != TextMessage {
		t.Errorf("msgType = %v, want TextMessage", msgType)
	}

	want := "Part1Part2"
	if string(payload) != want {
		t.Errorf("payload = %q, want %q", payload, want)
	}
}

// TestConn_ReadText tests ReadText convenience method.
func TestConn_ReadText(t *testing.T) {
	tests := []struct {
		name     string
		frames   []*frame
		wantText string
		wantErr  error
	}{
		{
			name: "text message",
			frames: []*frame{
				{fin: true, opcode: opcodeText, payload: []byte("Hello")},
			},
			wantText: "Hello",
		},
		{
			name: "binary message (error)",
			frames: []*frame{
				{fin: true, opcode: opcodeBinary, payload: []byte{0x01}},
			},
			wantErr: ErrInvalidMessageType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := mockConn(t, tt.frames, false)

			text, err := conn.ReadText()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("ReadText() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("ReadText() error = %v", err)
			}

			if text != tt.wantText {
				t.Errorf("ReadText() = %q, want %q", text, tt.wantText)
			}
		})
	}
}

// TestConn_ReadJSON tests ReadJSON convenience method.
func TestConn_ReadJSON(t *testing.T) {
	type Message struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}

	tests := []struct {
		name    string
		frames  []*frame
		want    Message
		wantErr bool
	}{
		{
			name: "valid JSON",
			frames: []*frame{
				{fin: true, opcode: opcodeText, payload: []byte(`{"type":"greeting","text":"Hello"}`)},
			},
			want: Message{Type: "greeting", Text: "Hello"},
		},
		{
			name: "invalid JSON",
			frames: []*frame{
				{fin: true, opcode: opcodeText, payload: []byte(`{invalid}`)},
			},
			wantErr: true,
		},
		{
			name: "binary message (error)",
			frames: []*frame{
				{fin: true, opcode: opcodeBinary, payload: []byte(`{"type":"test"}`)},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := mockConn(t, tt.frames, false)

			var msg Message
			err := conn.ReadJSON(&msg)

			if tt.wantErr {
				if err == nil {
					t.Error("ReadJSON() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ReadJSON() error = %v", err)
			}

			if msg != tt.want {
				t.Errorf("ReadJSON() = %+v, want %+v", msg, tt.want)
			}
		})
	}
}

// TestConn_Write tests basic message writing.
func TestConn_Write(t *testing.T) {
	tests := []struct {
		name        string
		msgType     MessageType
		payload     []byte
		wantOpcode  byte
		wantPayload string
		wantErr     error
	}{
		{
			name:        "text message",
			msgType:     TextMessage,
			payload:     []byte("Hello"),
			wantOpcode:  opcodeText,
			wantPayload: "Hello",
		},
		{
			name:        "binary message",
			msgType:     BinaryMessage,
			payload:     []byte{0x01, 0x02},
			wantOpcode:  opcodeBinary,
			wantPayload: "\x01\x02",
		},
		{
			name:    "invalid UTF-8 in text",
			msgType: TextMessage,
			payload: []byte{0xFF, 0xFE},
			wantErr: ErrInvalidUTF8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, writeBuf := mockConnWriter(t) // server-side (no masking)

			err := conn.Write(tt.msgType, tt.payload)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Write() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}

			// Read frame from buffer
			r := bufio.NewReader(writeBuf)
			frame, err := readFrame(r)
			if err != nil {
				t.Fatalf("readFrame() error = %v", err)
			}

			if frame.opcode != tt.wantOpcode {
				t.Errorf("opcode = %d, want %d", frame.opcode, tt.wantOpcode)
			}

			if string(frame.payload) != tt.wantPayload {
				t.Errorf("payload = %q, want %q", frame.payload, tt.wantPayload)
			}

			if frame.masked {
				t.Error("Server frame should not be masked")
			}
		})
	}
}

// TestConn_WriteText tests WriteText convenience method.
func TestConn_WriteText(t *testing.T) {
	conn, writeBuf := mockConnWriter(t)

	text := "Hello, WebSocket!"
	err := conn.WriteText(text)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}

	r := bufio.NewReader(writeBuf)
	frame, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}

	if frame.opcode != opcodeText {
		t.Errorf("opcode = %d, want %d", frame.opcode, opcodeText)
	}

	if string(frame.payload) != text {
		t.Errorf("payload = %q, want %q", frame.payload, text)
	}
}

// TestConn_WriteJSON tests WriteJSON convenience method.
func TestConn_WriteJSON(t *testing.T) {
	type Message struct {
		Type string `json:"type"`
		Data int    `json:"data"`
	}

	conn, writeBuf := mockConnWriter(t)

	msg := Message{Type: "test", Data: 42}
	err := conn.WriteJSON(msg)
	if err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	r := bufio.NewReader(writeBuf)
	frame, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}

	if frame.opcode != opcodeText {
		t.Errorf("opcode = %d, want %d", frame.opcode, opcodeText)
	}

	var decoded Message
	if err := json.Unmarshal(frame.payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded != msg {
		t.Errorf("decoded = %+v, want %+v", decoded, msg)
	}
}

// TestConn_Ping tests Ping frame sending.
func TestConn_Ping(t *testing.T) {
	conn, writeBuf := mockConnWriter(t)

	pingData := []byte("ping-data")
	err := conn.Ping(pingData)
	if err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	r := bufio.NewReader(writeBuf)
	frame, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}

	if frame.opcode != opcodePing {
		t.Errorf("opcode = %d, want %d", frame.opcode, opcodePing)
	}

	if !bytes.Equal(frame.payload, pingData) {
		t.Errorf("payload = %v, want %v", frame.payload, pingData)
	}

	if !frame.fin {
		t.Error("Ping frame should have FIN=1")
	}
}

// TestConn_Pong tests Pong frame sending.
func TestConn_Pong(t *testing.T) {
	conn, writeBuf := mockConnWriter(t)

	pongData := []byte("pong-data")
	err := conn.Pong(pongData)
	if err != nil {
		t.Fatalf("Pong() error = %v", err)
	}

	r := bufio.NewReader(writeBuf)
	frame, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}

	if frame.opcode != opcodePong {
		t.Errorf("opcode = %d, want %d", frame.opcode, opcodePong)
	}

	if !bytes.Equal(frame.payload, pongData) {
		t.Errorf("payload = %v, want %v", frame.payload, pongData)
	}

	if !frame.fin {
		t.Error("Pong frame should have FIN=1")
	}
}

// TestConn_Close tests normal close.
func TestConn_Close(t *testing.T) {
	conn, writeBuf := mockConnWriter(t)

	err := conn.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify close frame sent
	r := bufio.NewReader(writeBuf)
	frame, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}

	if frame.opcode != opcodeClose {
		t.Errorf("opcode = %d, want %d", frame.opcode, opcodeClose)
	}

	// Parse status code
	if len(frame.payload) >= 2 {
		code := CloseCode(uint16(frame.payload[0])<<8 | uint16(frame.payload[1]))
		if code != CloseNormalClosure {
			t.Errorf("close code = %d, want %d", code, CloseNormalClosure)
		}
	} else {
		t.Error("Close frame should have status code")
	}
}

// TestConn_CloseWithCode tests close with custom status code.
func TestConn_CloseWithCode(t *testing.T) {
	tests := []struct {
		name   string
		code   CloseCode
		reason string
	}{
		{"normal closure", CloseNormalClosure, "goodbye"},
		{"going away", CloseGoingAway, "server restart"},
		{"protocol error", CloseProtocolError, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, writeBuf := mockConnWriter(t)

			err := conn.CloseWithCode(tt.code, tt.reason)
			if err != nil {
				t.Fatalf("CloseWithCode() error = %v", err)
			}

			// Verify close frame
			r := bufio.NewReader(writeBuf)
			frame, err := readFrame(r)
			if err != nil {
				t.Fatalf("readFrame() error = %v", err)
			}

			if frame.opcode != opcodeClose {
				t.Errorf("opcode = %d, want %d", frame.opcode, opcodeClose)
			}

			if len(frame.payload) < 2 {
				t.Fatal("Close frame should have status code")
			}

			code := CloseCode(uint16(frame.payload[0])<<8 | uint16(frame.payload[1]))
			if code != tt.code {
				t.Errorf("close code = %d, want %d", code, tt.code)
			}

			if len(frame.payload) > 2 {
				reason := string(frame.payload[2:])
				if reason != tt.reason {
					t.Errorf("reason = %q, want %q", reason, tt.reason)
				}
			}
		})
	}
}

// TestConn_ConcurrentWrites tests write serialization with mutex.
func TestConn_ConcurrentWrites(t *testing.T) {
	conn, _ := mockConnWriter(t)

	const numWrites = 100
	var wg sync.WaitGroup
	wg.Add(numWrites)

	// Start concurrent writes
	for i := 0; i < numWrites; i++ {
		go func(_ int) {
			defer wg.Done()
			_ = conn.WriteText("message")
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
		// Success - all writes completed without deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("Concurrent writes timeout - possible deadlock")
	}
}

// TestConn_DoubleClose tests Close idempotency.
func TestConn_DoubleClose(t *testing.T) {
	conn, writeBuf := mockConnWriter(t)

	// First close
	err1 := conn.Close()
	if err1 != nil {
		t.Fatalf("First Close() error = %v", err1)
	}

	// Read first close frame
	r := bufio.NewReader(writeBuf)
	frame1, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}
	if frame1.opcode != opcodeClose {
		t.Error("Expected close frame")
	}

	// Second close (should be no-op)
	err2 := conn.Close()
	if err2 != nil {
		t.Fatalf("Second Close() error = %v", err2)
	}

	// Try to read second frame (should be EOF)
	frame2, err := readFrame(r)
	if err == nil && frame2 != nil {
		t.Error("Second close frame sent (Close not idempotent)")
	}
}

// TestConn_WriteAfterClose tests that writes fail after close.
func TestConn_WriteAfterClose(t *testing.T) {
	conn, _ := mockConnWriter(t)

	// Close connection
	_ = conn.Close()

	// Try to write (should fail)
	err := conn.WriteText("test")
	if !errors.Is(err, ErrClosed) {
		t.Errorf("WriteText() after Close() error = %v, want ErrClosed", err)
	}
}

// TestConn_ReadAfterClose tests that reads fail after close.
func TestConn_ReadAfterClose(t *testing.T) {
	frames := []*frame{
		{fin: true, opcode: opcodeText, payload: []byte("test")},
	}
	conn := mockConn(t, frames, false)

	// Close connection
	conn.closeMu.Lock()
	conn.closed = true
	conn.closeMu.Unlock()

	// Try to read (should fail)
	_, _, err := conn.Read()
	if !errors.Is(err, ErrClosed) {
		t.Errorf("Read() after close error = %v, want ErrClosed", err)
	}
}

// TestConn_ReceiveCloseFrame tests receiving close frame from peer.
func TestConn_ReceiveCloseFrame(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte // Close frame payload (status code + reason)
	}{
		{
			name:    "close with status and reason",
			payload: []byte{0x03, 0xE8, 'N', 'o', 'r', 'm', 'a', 'l'}, // 1000 + "Normal"
		},
		{
			name:    "close with status only",
			payload: []byte{0x03, 0xE9}, // 1001 (Going Away)
		},
		{
			name:    "close without status",
			payload: []byte{}, // No status code
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frames := []*frame{
				{fin: true, opcode: opcodeClose, payload: tt.payload},
			}
			conn := mockConn(t, frames, false)

			// Read should return ErrClosed after receiving close frame
			_, _, err := conn.Read()
			if !errors.Is(err, ErrClosed) {
				t.Errorf("Read() after close frame error = %v, want ErrClosed", err)
			}

			// Connection should be marked as closed
			conn.closeMu.RLock()
			if !conn.closed {
				t.Error("Connection not marked as closed after receiving close frame")
			}
			conn.closeMu.RUnlock()
		})
	}
}

// TestConn_PingTooLarge tests Ping with payload > 125 bytes.
func TestConn_PingTooLarge(t *testing.T) {
	conn, _ := mockConnWriter(t)

	// Create payload > 125 bytes
	largePayload := make([]byte, 126)

	err := conn.Ping(largePayload)
	if !errors.Is(err, ErrControlTooLarge) {
		t.Errorf("Ping() with 126 bytes error = %v, want ErrControlTooLarge", err)
	}
}

// TestConn_PongTooLarge tests Pong with payload > 125 bytes.
func TestConn_PongTooLarge(t *testing.T) {
	conn, _ := mockConnWriter(t)

	// Create payload > 125 bytes
	largePayload := make([]byte, 126)

	err := conn.Pong(largePayload)
	if !errors.Is(err, ErrControlTooLarge) {
		t.Errorf("Pong() with 126 bytes error = %v, want ErrControlTooLarge", err)
	}
}

// TestConn_CloseWithInvalidUTF8Reason tests CloseWithCode with invalid UTF-8 reason.
func TestConn_CloseWithInvalidUTF8Reason(t *testing.T) {
	conn, _ := mockConnWriter(t)

	// Invalid UTF-8 string
	invalidReason := string([]byte{0xFF, 0xFE})

	err := conn.CloseWithCode(CloseNormalClosure, invalidReason)
	if !errors.Is(err, ErrInvalidUTF8) {
		t.Errorf("CloseWithCode() with invalid UTF-8 error = %v, want ErrInvalidUTF8", err)
	}
}

// TestConn_WriteJSONMarshalError tests WriteJSON with non-marshalable value.
func TestConn_WriteJSONMarshalError(t *testing.T) {
	conn, _ := mockConnWriter(t)

	// Channels cannot be marshaled to JSON
	nonMarshalable := make(chan int)

	err := conn.WriteJSON(nonMarshalable)
	if err == nil {
		t.Error("WriteJSON() with channel should return marshal error")
	}
}

// TestConn_ReadUnexpectedContinuation tests Read with unexpected continuation frame.
func TestConn_ReadUnexpectedContinuation(t *testing.T) {
	frames := []*frame{
		{fin: true, opcode: opcodeContinuation, payload: []byte("unexpected")},
	}
	conn := mockConn(t, frames, false)

	_, _, err := conn.Read()
	if !errors.Is(err, ErrUnexpectedContinuation) {
		t.Errorf("Read() unexpected continuation error = %v, want ErrUnexpectedContinuation", err)
	}
}

// TestConn_ReadFragmentedInvalidUTF8 tests fragmented message with invalid UTF-8.
func TestConn_ReadFragmentedInvalidUTF8(t *testing.T) {
	frames := []*frame{
		{fin: false, opcode: opcodeText, payload: []byte("Hello ")},          // Start fragment
		{fin: true, opcode: opcodeContinuation, payload: []byte{0xFF, 0xFE}}, // Invalid UTF-8 in final fragment
	}
	conn := mockConnNoValidation(t, frames, false)

	_, _, err := conn.Read()
	if !errors.Is(err, ErrInvalidUTF8) {
		t.Errorf("Read() fragmented invalid UTF-8 error = %v, want ErrInvalidUTF8", err)
	}
}

// TestConn_PingAfterClose tests Ping after connection is closed.
func TestConn_PingAfterClose(t *testing.T) {
	conn, _ := mockConnWriter(t)

	// Close connection
	conn.closeMu.Lock()
	conn.closed = true
	conn.closeMu.Unlock()

	err := conn.Ping([]byte("test"))
	if !errors.Is(err, ErrClosed) {
		t.Errorf("Ping() after close error = %v, want ErrClosed", err)
	}
}

// TestConn_PongAfterClose tests Pong after connection is closed.
func TestConn_PongAfterClose(t *testing.T) {
	conn, _ := mockConnWriter(t)

	// Close connection
	conn.closeMu.Lock()
	conn.closed = true
	conn.closeMu.Unlock()

	err := conn.Pong([]byte("test"))
	if !errors.Is(err, ErrClosed) {
		t.Errorf("Pong() after close error = %v, want ErrClosed", err)
	}
}

// TestConn_ReadTextError tests ReadText when Read fails.
func TestConn_ReadTextError(t *testing.T) {
	// Empty buffer will cause EOF error
	conn := mockConn(t, []*frame{}, false)

	_, err := conn.ReadText()
	if err == nil {
		t.Error("ReadText() on empty connection should return error")
	}
}

// TestConn_ReadJSONError tests ReadJSON when Read fails.
func TestConn_ReadJSONError(t *testing.T) {
	// Empty buffer will cause EOF error
	conn := mockConn(t, []*frame{}, false)

	var result map[string]string
	err := conn.ReadJSON(&result)
	if err == nil {
		t.Error("ReadJSON() on empty connection should return error")
	}
}

// TestConn_WriteError tests Write when connection is closed.
func TestConn_WriteError(t *testing.T) {
	conn, _ := mockConnWriter(t)

	// Close connection
	conn.closeMu.Lock()
	conn.closed = true
	conn.closeMu.Unlock()

	err := conn.Write(TextMessage, []byte("test"))
	if !errors.Is(err, ErrClosed) {
		t.Errorf("Write() after close error = %v, want ErrClosed", err)
	}
}
