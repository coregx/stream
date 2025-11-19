package websocket

import (
	"bufio"
	"bytes"
	"testing"
)

// TestRFC_ControlFramesDuringFragmentation verifies RFC 6455 Section 5.5.
//
// "Control frames (see Section 5.5) MAY be injected in the middle of
// a fragmented message.  Control frames themselves MUST NOT be fragmented.".
func TestRFC_ControlFramesDuringFragmentation(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	// Write first fragment (FIN=0, opcode=text)
	f1 := &frame{
		fin:     false,
		opcode:  opcodeText,
		masked:  false,
		payload: []byte("Hello, "),
	}
	if err := writeFrame(w, f1); err != nil {
		t.Fatalf("Write first fragment failed: %v", err)
	}

	// Inject PING control frame during fragmentation
	ping := &frame{
		fin:     true,
		opcode:  opcodePing,
		masked:  false,
		payload: []byte("ping"),
	}
	if err := writeFrame(w, ping); err != nil {
		t.Fatalf("Write PING failed: %v", err)
	}

	// Write continuation frame (FIN=0, opcode=continuation)
	f2 := &frame{
		fin:     false,
		opcode:  opcodeContinuation,
		masked:  false,
		payload: []byte("World"),
	}
	if err := writeFrame(w, f2); err != nil {
		t.Fatalf("Write continuation failed: %v", err)
	}

	// Write final continuation (FIN=1, opcode=continuation)
	f3 := &frame{
		fin:     true,
		opcode:  opcodeContinuation,
		masked:  false,
		payload: []byte("!"),
	}
	if err := writeFrame(w, f3); err != nil {
		t.Fatalf("Write final continuation failed: %v", err)
	}

	w.Flush()

	// Read back frames
	r := bufio.NewReader(&buf)

	// Read first fragment
	frame1, err := readFrame(r)
	if err != nil {
		t.Fatalf("Read fragment 1 failed: %v", err)
	}
	if frame1.fin {
		t.Error("First fragment should have FIN=0")
	}
	if frame1.opcode != opcodeText {
		t.Errorf("First fragment opcode = %d, want %d", frame1.opcode, opcodeText)
	}

	// Read PING (control frame during fragmentation)
	pingFrame, err := readFrame(r)
	if err != nil {
		t.Fatalf("Read PING failed: %v", err)
	}
	if !pingFrame.fin {
		t.Error("PING should have FIN=1")
	}
	if pingFrame.opcode != opcodePing {
		t.Errorf("PING opcode = %d, want %d", pingFrame.opcode, opcodePing)
	}

	// Read continuation frame
	frame2, err := readFrame(r)
	if err != nil {
		t.Fatalf("Read continuation failed: %v", err)
	}
	if frame2.opcode != opcodeContinuation {
		t.Errorf("Continuation opcode = %d, want %d", frame2.opcode, opcodeContinuation)
	}

	// Read final continuation
	frame3, err := readFrame(r)
	if err != nil {
		t.Fatalf("Read final continuation failed: %v", err)
	}
	if !frame3.fin {
		t.Error("Final continuation should have FIN=1")
	}
}

// TestRFC_PayloadLengthBoundaries tests all payload length encoding types.
//
// RFC 6455 Section 5.2:
// - 0-125: stored in 7 bits
// - 126-65535: 7 bits = 126, followed by 16-bit length
// - 65536+: 7 bits = 127, followed by 64-bit length.
func TestRFC_PayloadLengthBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"zero length", 0},
		{"7-bit max (125)", 125},
		{"16-bit threshold (126)", 126},
		{"16-bit mid (1000)", 1000},
		{"16-bit max (65535)", 65535},
		{"64-bit threshold (65536)", 65536},
		// Note: Don't test huge payloads in unit tests (memory constraints)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := make([]byte, tt.length)
			for i := range payload {
				payload[i] = byte(i % 256)
			}

			// Write frame
			var buf bytes.Buffer
			w := bufio.NewWriter(&buf)

			f := &frame{
				fin:     true,
				opcode:  opcodeBinary,
				masked:  false,
				payload: payload,
			}

			if err := writeFrame(w, f); err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			w.Flush()

			// Read frame back
			r := bufio.NewReader(&buf)
			readBack, err := readFrame(r)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			if len(readBack.payload) != tt.length {
				t.Errorf("Payload length = %d, want %d", len(readBack.payload), tt.length)
			}

			// Verify payload content
			for i := range payload {
				if readBack.payload[i] != payload[i] {
					t.Errorf("Payload mismatch at byte %d: got %d, want %d", i, readBack.payload[i], payload[i])
					break
				}
			}
		})
	}
}

// TestRFC_MaskingRequirement tests RFC 6455 Section 5.1.
//
// "A client MUST mask all frames that it sends to the server."
// "A server MUST NOT mask any frames that it sends to the client.".
func TestRFC_MaskingRequirement(t *testing.T) {
	t.Run("client frame must be masked", func(t *testing.T) {
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)

		// Client frame (masked)
		f := &frame{
			fin:     true,
			opcode:  opcodeText,
			masked:  true,
			mask:    [4]byte{0x12, 0x34, 0x56, 0x78},
			payload: []byte("test"),
		}

		if err := writeFrame(w, f); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		w.Flush()

		// Verify mask bit is set in wire format
		data := buf.Bytes()
		if len(data) < 2 {
			t.Fatal("Frame too short")
		}

		maskBit := data[1] & 0x80
		if maskBit == 0 {
			t.Error("Client frame must have mask bit set")
		}
	})

	t.Run("server frame must not be masked", func(t *testing.T) {
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)

		// Server frame (unmasked)
		f := &frame{
			fin:     true,
			opcode:  opcodeText,
			masked:  false,
			payload: []byte("test"),
		}

		if err := writeFrame(w, f); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		w.Flush()

		// Verify mask bit is NOT set in wire format
		data := buf.Bytes()
		if len(data) < 2 {
			t.Fatal("Frame too short")
		}

		maskBit := data[1] & 0x80
		if maskBit != 0 {
			t.Error("Server frame must NOT have mask bit set")
		}
	})
}

// TestRFC_UTF8Validation tests RFC 6455 Section 8.1.
//
// "Text frames (data frames with opcode 0x1) contain UTF-8-encoded data.".
func TestRFC_UTF8Validation_Extended(t *testing.T) {
	tests := []struct {
		name      string
		payload   []byte
		wantError bool
	}{
		{
			name:      "valid ASCII",
			payload:   []byte("Hello, World!"),
			wantError: false,
		},
		{
			name:      "valid UTF-8 multi-byte",
			payload:   []byte("Привет мир! 你好世界!"),
			wantError: false,
		},
		{
			name:      "valid emoji",
			payload:   []byte("Hello 👋 World 🌍"),
			wantError: false,
		},
		{
			name:      "invalid UTF-8 - unexpected continuation",
			payload:   []byte{0x80, 0x81, 0x82}, // Invalid: continuation bytes without start
			wantError: true,
		},
		{
			name:      "invalid UTF-8 - incomplete sequence",
			payload:   []byte{0xC2}, // Incomplete 2-byte sequence
			wantError: true,
		},
		{
			name:      "invalid UTF-8 - overlong encoding",
			payload:   []byte{0xC0, 0x80}, // Overlong encoding of NULL
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := bufio.NewWriter(&buf)

			f := &frame{
				fin:     true,
				opcode:  opcodeText,
				masked:  false,
				payload: tt.payload,
			}

			if err := writeFrame(w, f); err != nil {
				if !tt.wantError {
					t.Errorf("Write failed unexpectedly: %v", err)
				}
				return
			}

			if tt.wantError {
				t.Error("Expected write to fail with invalid UTF-8, but it succeeded")
			}

			w.Flush()

			// Try to read back
			r := bufio.NewReader(&buf)
			_, err := readFrame(r)
			if tt.wantError && err == nil {
				t.Error("Expected read to fail with invalid UTF-8, but it succeeded")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Read failed unexpectedly: %v", err)
			}
		})
	}
}

// TestRFC_FragmentationSequence tests RFC 6455 Section 5.4.
//
// "A fragmented message consists of a single frame with the FIN bit clear
// and an opcode other than 0, followed by zero or more frames with the FIN
// bit clear and the opcode set to 0, and terminated by a single frame with
// the FIN bit set and an opcode of 0.".
func TestRFC_FragmentationSequence(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	// Correct fragmentation sequence
	frames := []*frame{
		{fin: false, opcode: opcodeText, payload: []byte("Part 1")},
		{fin: false, opcode: opcodeContinuation, payload: []byte(" Part 2")},
		{fin: false, opcode: opcodeContinuation, payload: []byte(" Part 3")},
		{fin: true, opcode: opcodeContinuation, payload: []byte(" Part 4")},
	}

	for _, f := range frames {
		if err := writeFrame(w, f); err != nil {
			t.Fatalf("Write frame failed: %v", err)
		}
	}
	w.Flush()

	// Read back and verify sequence
	r := bufio.NewReader(&buf)

	// First frame: FIN=0, opcode=text
	f1, err := readFrame(r)
	if err != nil {
		t.Fatalf("Read first frame failed: %v", err)
	}
	if f1.fin || f1.opcode != opcodeText {
		t.Error("First frame should be FIN=0, opcode=text")
	}

	// Continuation frames: FIN=0, opcode=continuation
	for i := 1; i < 3; i++ {
		f, err := readFrame(r)
		if err != nil {
			t.Fatalf("Read continuation %d failed: %v", i, err)
		}
		if f.fin || f.opcode != opcodeContinuation {
			t.Errorf("Continuation %d should be FIN=0, opcode=continuation", i)
		}
	}

	// Final frame: FIN=1, opcode=continuation
	fFinal, err := readFrame(r)
	if err != nil {
		t.Fatalf("Read final frame failed: %v", err)
	}
	if !fFinal.fin || fFinal.opcode != opcodeContinuation {
		t.Error("Final frame should be FIN=1, opcode=continuation")
	}

	// Note: In a real implementation, you would assemble fragments as you read them
	// For this test, we just verify the sequence is correct
}

// TestRFC_CloseFramePayload tests RFC 6455 Section 5.5.1.
//
// "Close frames MAY contain a body that indicates a reason for closing.
// If there is a body, the first two bytes must be a 2-byte unsigned integer
// representing a status code.".
func TestRFC_CloseFramePayload(t *testing.T) {
	tests := []struct {
		name       string
		statusCode uint16
		reason     string
	}{
		{"normal closure", 1000, "Normal closure"},
		{"going away", 1001, "Going away"},
		{"protocol error", 1002, "Protocol error"},
		{"empty reason", 1000, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build close frame payload: 2-byte status code + optional reason
			var payload []byte
			payload = append(payload, byte(tt.statusCode>>8), byte(tt.statusCode&0xFF))
			payload = append(payload, []byte(tt.reason)...)

			var buf bytes.Buffer
			w := bufio.NewWriter(&buf)

			f := &frame{
				fin:     true,
				opcode:  opcodeClose,
				masked:  false,
				payload: payload,
			}

			if err := writeFrame(w, f); err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			w.Flush()

			// Read back
			r := bufio.NewReader(&buf)
			readBack, err := readFrame(r)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			if len(readBack.payload) < 2 {
				t.Fatal("Close frame should have at least 2 bytes for status code")
			}

			// Parse status code
			statusCode := uint16(readBack.payload[0])<<8 | uint16(readBack.payload[1])
			if statusCode != tt.statusCode {
				t.Errorf("Status code = %d, want %d", statusCode, tt.statusCode)
			}

			// Parse reason (if present)
			if len(readBack.payload) > 2 {
				reason := string(readBack.payload[2:])
				if reason != tt.reason {
					t.Errorf("Reason = %q, want %q", reason, tt.reason)
				}
			}
		})
	}
}
