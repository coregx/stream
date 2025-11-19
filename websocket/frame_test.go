package websocket

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestReadFrame_TextUnmasked tests reading an unmasked text frame.
// RFC 6455 Section 5.6: Text frames contain UTF-8 data.
func TestReadFrame_TextUnmasked(t *testing.T) {
	// Frame: FIN=1, opcode=text(0x1), unmasked, payload="Hello"
	data := []byte{
		0x81, // FIN=1, RSV=0, opcode=0x1 (text)
		0x05, // MASK=0, length=5
		'H', 'e', 'l', 'l', 'o',
	}

	r := bufio.NewReader(bytes.NewReader(data))
	f, err := readFrame(r)

	if err != nil {
		t.Fatalf("readFrame failed: %v", err)
	}

	if !f.fin {
		t.Error("expected FIN=1")
	}
	if f.opcode != opcodeText {
		t.Errorf("expected opcode text(0x1), got 0x%X", f.opcode)
	}
	if f.masked {
		t.Error("expected unmasked frame")
	}
	if string(f.payload) != "Hello" {
		t.Errorf("expected payload 'Hello', got '%s'", f.payload)
	}
}

// TestReadFrame_TextMasked tests reading a masked text frame.
// RFC 6455 Section 5.3: Client-to-server frames must be masked.
func TestReadFrame_TextMasked(t *testing.T) {
	payload := []byte("Hello")
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}

	// Apply mask to payload.
	masked := make([]byte, len(payload))
	copy(masked, payload)
	applyMask(masked, mask)

	// Frame: FIN=1, opcode=text(0x1), masked
	data := []byte{
		0x81,                               // FIN=1, RSV=0, opcode=0x1 (text)
		0x85,                               // MASK=1, length=5
		mask[0], mask[1], mask[2], mask[3], // Masking key
	}
	data = append(data, masked...)

	r := bufio.NewReader(bytes.NewReader(data))
	f, err := readFrame(r)

	if err != nil {
		t.Fatalf("readFrame failed: %v", err)
	}

	if !f.masked {
		t.Error("expected masked frame")
	}
	if f.mask != mask {
		t.Errorf("expected mask %v, got %v", mask, f.mask)
	}
	if string(f.payload) != "Hello" {
		t.Errorf("expected unmasked payload 'Hello', got '%s'", f.payload)
	}
}

// TestReadFrame_Binary tests reading a binary frame.
// RFC 6455 Section 5.6: Binary frames contain arbitrary data.
func TestReadFrame_Binary(t *testing.T) {
	payload := []byte{0x00, 0xFF, 0xAA, 0x55}

	data := []byte{
		0x82, // FIN=1, RSV=0, opcode=0x2 (binary)
		0x04, // MASK=0, length=4
	}
	data = append(data, payload...)

	r := bufio.NewReader(bytes.NewReader(data))
	f, err := readFrame(r)

	if err != nil {
		t.Fatalf("readFrame failed: %v", err)
	}

	if f.opcode != opcodeBinary {
		t.Errorf("expected opcode binary(0x2), got 0x%X", f.opcode)
	}
	if !bytes.Equal(f.payload, payload) {
		t.Errorf("expected payload %v, got %v", payload, f.payload)
	}
}

// TestReadFrame_Fragmented tests reading fragmented frames.
// RFC 6455 Section 5.4: Messages may be fragmented.
func TestReadFrame_Fragmented(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantFIN bool
		wantOp  byte
	}{
		{
			name: "first fragment (FIN=0)",
			data: []byte{
				0x01, // FIN=0, RSV=0, opcode=0x1 (text)
				0x03, // MASK=0, length=3
				'H', 'e', 'l',
			},
			wantFIN: false,
			wantOp:  opcodeText,
		},
		{
			name: "continuation (FIN=0)",
			data: []byte{
				0x00, // FIN=0, RSV=0, opcode=0x0 (continuation)
				0x02, // MASK=0, length=2
				'l', 'o',
			},
			wantFIN: false,
			wantOp:  opcodeContinuation,
		},
		{
			name: "final continuation (FIN=1)",
			data: []byte{
				0x80, // FIN=1, RSV=0, opcode=0x0 (continuation)
				0x01, // MASK=0, length=1
				'!',
			},
			wantFIN: true,
			wantOp:  opcodeContinuation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := bufio.NewReader(bytes.NewReader(tt.data))
			f, err := readFrame(r)

			if err != nil {
				t.Fatalf("readFrame failed: %v", err)
			}

			if f.fin != tt.wantFIN {
				t.Errorf("expected FIN=%v, got FIN=%v", tt.wantFIN, f.fin)
			}
			if f.opcode != tt.wantOp {
				t.Errorf("expected opcode 0x%X, got 0x%X", tt.wantOp, f.opcode)
			}
		})
	}
}

// TestReadFrame_ControlFrames tests reading control frames.
// RFC 6455 Section 5.5: Control frames (close, ping, pong).
func TestReadFrame_ControlFrames(t *testing.T) {
	tests := []struct {
		name   string
		opcode byte
		data   []byte
	}{
		{
			name:   "close",
			opcode: opcodeClose,
			data:   []byte{0x88, 0x00}, // FIN=1, opcode=0x8, length=0
		},
		{
			name:   "ping",
			opcode: opcodePing,
			data: []byte{
				0x89, // FIN=1, opcode=0x9
				0x04, // length=4
				'p', 'i', 'n', 'g',
			},
		},
		{
			name:   "pong",
			opcode: opcodePong,
			data: []byte{
				0x8A, // FIN=1, opcode=0xA
				0x04, // length=4
				'p', 'o', 'n', 'g',
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := bufio.NewReader(bytes.NewReader(tt.data))
			f, err := readFrame(r)

			if err != nil {
				t.Fatalf("readFrame failed: %v", err)
			}

			if f.opcode != tt.opcode {
				t.Errorf("expected opcode 0x%X, got 0x%X", tt.opcode, f.opcode)
			}
			if !f.fin {
				t.Error("control frames must have FIN=1")
			}
		})
	}
}

// TestReadFrame_ExtendedLength16 tests 16-bit extended payload length.
// RFC 6455 Section 5.2: Length 126 triggers 16-bit encoding.
func TestReadFrame_ExtendedLength16(t *testing.T) {
	payloadLen := 1000
	payload := bytes.Repeat([]byte("A"), payloadLen)

	data := []byte{
		0x81, // FIN=1, opcode=0x1 (text)
		126,  // MASK=0, length=126 (triggers 16-bit)
	}

	// Write 16-bit length.
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(payloadLen))
	data = append(data, lenBuf...)
	data = append(data, payload...)

	r := bufio.NewReader(bytes.NewReader(data))
	f, err := readFrame(r)

	if err != nil {
		t.Fatalf("readFrame failed: %v", err)
	}

	if len(f.payload) != payloadLen {
		t.Errorf("expected payload length %d, got %d", payloadLen, len(f.payload))
	}
}

// TestReadFrame_ExtendedLength64 tests 64-bit extended payload length.
// RFC 6455 Section 5.2: Length 127 triggers 64-bit encoding.
func TestReadFrame_ExtendedLength64(t *testing.T) {
	payloadLen := 70000
	payload := bytes.Repeat([]byte("B"), payloadLen)

	data := []byte{
		0x82, // FIN=1, opcode=0x2 (binary)
		127,  // MASK=0, length=127 (triggers 64-bit)
	}

	// Write 64-bit length.
	lenBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(lenBuf, uint64(payloadLen))
	data = append(data, lenBuf...)
	data = append(data, payload...)

	r := bufio.NewReader(bytes.NewReader(data))
	f, err := readFrame(r)

	if err != nil {
		t.Fatalf("readFrame failed: %v", err)
	}

	if len(f.payload) != payloadLen {
		t.Errorf("expected payload length %d, got %d", payloadLen, len(f.payload))
	}
}

// TestReadFrame_InvalidOpcode tests invalid opcode detection.
// RFC 6455 Section 5.2: Opcodes 0x3-0x7 and 0xB-0xF are reserved.
func TestReadFrame_InvalidOpcode(t *testing.T) {
	invalidOpcodes := []byte{0x3, 0x4, 0x5, 0x6, 0x7, 0xB, 0xC, 0xD, 0xE, 0xF}

	for _, opcode := range invalidOpcodes {
		t.Run("opcode_0x"+string(opcode), func(t *testing.T) {
			data := []byte{
				0x80 | opcode, // FIN=1, invalid opcode
				0x00,          // MASK=0, length=0
			}

			r := bufio.NewReader(bytes.NewReader(data))
			_, err := readFrame(r)

			if !errors.Is(err, ErrInvalidOpcode) {
				t.Errorf("expected ErrInvalidOpcode, got %v", err)
			}
		})
	}
}

// TestReadFrame_ReservedBits tests reserved bit validation.
// RFC 6455 Section 5.2: RSV bits must be 0 unless extension negotiated.
func TestReadFrame_ReservedBits(t *testing.T) {
	tests := []struct {
		name  string
		byte0 byte
	}{
		{"RSV1", 0xC1}, // FIN=1, RSV1=1, opcode=0x1
		{"RSV2", 0xA1}, // FIN=1, RSV2=1, opcode=0x1
		{"RSV3", 0x91}, // FIN=1, RSV3=1, opcode=0x1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte{tt.byte0, 0x00}

			r := bufio.NewReader(bytes.NewReader(data))
			_, err := readFrame(r)

			if !errors.Is(err, ErrReservedBits) {
				t.Errorf("expected ErrReservedBits, got %v", err)
			}
		})
	}
}

// TestReadFrame_ControlFragmented tests control frame fragmentation error.
// RFC 6455 Section 5.5: Control frames must NOT be fragmented.
func TestReadFrame_ControlFragmented(t *testing.T) {
	// Control frame with FIN=0 (invalid).
	data := []byte{
		0x08, // FIN=0, opcode=0x8 (close) - INVALID!
		0x00, // MASK=0, length=0
	}

	r := bufio.NewReader(bytes.NewReader(data))
	_, err := readFrame(r)

	if !errors.Is(err, ErrControlFragmented) {
		t.Errorf("expected ErrControlFragmented, got %v", err)
	}
}

// TestReadFrame_ControlTooLarge tests control frame size limit.
// RFC 6455 Section 5.5: Control frames must have payload <= 125 bytes.
func TestReadFrame_ControlTooLarge(t *testing.T) {
	// Control frame with 126-byte payload (invalid).
	data := []byte{
		0x88,       // FIN=1, opcode=0x8 (close)
		126,        // MASK=0, length=126 (triggers 16-bit)
		0x00, 0x7E, // 126 bytes - EXCEEDS 125 LIMIT!
	}
	data = append(data, make([]byte, 126)...)

	r := bufio.NewReader(bytes.NewReader(data))
	_, err := readFrame(r)

	if !errors.Is(err, ErrControlTooLarge) {
		t.Errorf("expected ErrControlTooLarge, got %v", err)
	}
}

// TestReadFrame_InvalidUTF8 tests UTF-8 validation for text frames.
// RFC 6455 Section 8.1: Text frames must contain valid UTF-8.
func TestReadFrame_InvalidUTF8(t *testing.T) {
	// Invalid UTF-8 sequence.
	invalidUTF8 := []byte{0xFF, 0xFE, 0xFD}

	data := []byte{
		0x81, // FIN=1, opcode=0x1 (text)
		0x03, // MASK=0, length=3
	}
	data = append(data, invalidUTF8...)

	r := bufio.NewReader(bytes.NewReader(data))
	_, err := readFrame(r)

	if !errors.Is(err, ErrInvalidUTF8) {
		t.Errorf("expected ErrInvalidUTF8, got %v", err)
	}
}

// TestWriteFrame_Text tests writing a text frame.
func TestWriteFrame_Text(t *testing.T) {
	f := &frame{
		fin:     true,
		opcode:  opcodeText,
		masked:  false,
		payload: []byte("Hello"),
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	if err := writeFrame(w, f); err != nil {
		t.Fatalf("writeFrame failed: %v", err)
	}

	// Verify written data.
	data := buf.Bytes()
	expected := []byte{
		0x81, // FIN=1, RSV=0, opcode=0x1
		0x05, // MASK=0, length=5
		'H', 'e', 'l', 'l', 'o',
	}

	if !bytes.Equal(data, expected) {
		t.Errorf("expected %v, got %v", expected, data)
	}
}

// TestWriteFrame_Binary tests writing a binary frame.
func TestWriteFrame_Binary(t *testing.T) {
	payload := []byte{0x00, 0xFF, 0xAA, 0x55}

	f := &frame{
		fin:     true,
		opcode:  opcodeBinary,
		masked:  false,
		payload: payload,
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	if err := writeFrame(w, f); err != nil {
		t.Fatalf("writeFrame failed: %v", err)
	}

	data := buf.Bytes()
	expected := []byte{0x82, 0x04}
	expected = append(expected, payload...)

	if !bytes.Equal(data, expected) {
		t.Errorf("expected %v, got %v", expected, data)
	}
}

// TestWriteFrame_Masked tests writing a masked frame.
func TestWriteFrame_Masked(t *testing.T) {
	payload := []byte("Test")
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}

	f := &frame{
		fin:     true,
		opcode:  opcodeText,
		masked:  true,
		mask:    mask,
		payload: payload,
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	if err := writeFrame(w, f); err != nil {
		t.Fatalf("writeFrame failed: %v", err)
	}

	data := buf.Bytes()

	// Verify header.
	if data[0] != 0x81 {
		t.Errorf("expected header byte 0x81, got 0x%02X", data[0])
	}
	if data[1] != 0x84 { // MASK=1, length=4
		t.Errorf("expected header byte 0x84, got 0x%02X", data[1])
	}

	// Verify mask.
	if !bytes.Equal(data[2:6], mask[:]) {
		t.Errorf("expected mask %v, got %v", mask, data[2:6])
	}

	// Verify masked payload.
	masked := make([]byte, len(payload))
	copy(masked, payload)
	applyMask(masked, mask)

	if !bytes.Equal(data[6:], masked) {
		t.Errorf("expected masked payload %v, got %v", masked, data[6:])
	}
}

// TestWriteFrame_ControlFrames tests writing control frames.
func TestWriteFrame_ControlFrames(t *testing.T) {
	tests := []struct {
		name    string
		opcode  byte
		payload []byte
	}{
		{"close", opcodeClose, []byte{}},
		{"ping", opcodePing, []byte("ping")},
		{"pong", opcodePong, []byte("pong")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &frame{
				fin:     true,
				opcode:  tt.opcode,
				masked:  false,
				payload: tt.payload,
			}

			var buf bytes.Buffer
			w := bufio.NewWriter(&buf)

			if err := writeFrame(w, f); err != nil {
				t.Fatalf("writeFrame failed: %v", err)
			}

			// Verify opcode in header.
			data := buf.Bytes()
			opcode := data[0] & 0x0F
			if opcode != tt.opcode {
				t.Errorf("expected opcode 0x%X, got 0x%X", tt.opcode, opcode)
			}
		})
	}
}

// TestWriteFrame_ExtendedLength16 tests 16-bit extended length encoding.
func TestWriteFrame_ExtendedLength16(t *testing.T) {
	payloadLen := 1000
	payload := bytes.Repeat([]byte("A"), payloadLen)

	f := &frame{
		fin:     true,
		opcode:  opcodeText,
		masked:  false,
		payload: payload,
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	if err := writeFrame(w, f); err != nil {
		t.Fatalf("writeFrame failed: %v", err)
	}

	data := buf.Bytes()

	// Verify 16-bit length encoding.
	if data[1] != 126 {
		t.Errorf("expected length indicator 126, got %d", data[1])
	}

	// Verify length value.
	length := binary.BigEndian.Uint16(data[2:4])
	if length != uint16(payloadLen) {
		t.Errorf("expected length %d, got %d", payloadLen, length)
	}
}

// TestWriteFrame_ExtendedLength64 tests 64-bit extended length encoding.
func TestWriteFrame_ExtendedLength64(t *testing.T) {
	payloadLen := 70000
	payload := bytes.Repeat([]byte("B"), payloadLen)

	f := &frame{
		fin:     true,
		opcode:  opcodeBinary,
		masked:  false,
		payload: payload,
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	if err := writeFrame(w, f); err != nil {
		t.Fatalf("writeFrame failed: %v", err)
	}

	data := buf.Bytes()

	// Verify 64-bit length encoding.
	if data[1] != 127 {
		t.Errorf("expected length indicator 127, got %d", data[1])
	}

	// Verify length value.
	length := binary.BigEndian.Uint64(data[2:10])
	if length != uint64(payloadLen) {
		t.Errorf("expected length %d, got %d", payloadLen, length)
	}
}

// TestApplyMask tests masking/unmasking algorithm.
// RFC 6455 Section 5.3: XOR masking is reversible.
func TestApplyMask(t *testing.T) {
	original := []byte("Hello, WebSocket!")
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}

	// Make a copy.
	data := make([]byte, len(original))
	copy(data, original)

	// Apply mask (encrypt).
	applyMask(data, mask)

	// Verify data changed.
	if bytes.Equal(data, original) {
		t.Error("expected data to change after masking")
	}

	// Apply mask again (decrypt).
	applyMask(data, mask)

	// Verify data restored.
	if !bytes.Equal(data, original) {
		t.Errorf("expected data to restore to original, got '%s'", data)
	}
}

// TestApplyMask_EmptyData tests masking empty payload.
func TestApplyMask_EmptyData(t *testing.T) {
	var data []byte
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}

	// Should not panic.
	applyMask(data, mask)

	if len(data) != 0 {
		t.Error("expected empty data to remain empty")
	}
}

// TestRoundTrip tests write→read roundtrip.
func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		frame *frame
	}{
		{
			name: "text unmasked",
			frame: &frame{
				fin:     true,
				opcode:  opcodeText,
				masked:  false,
				payload: []byte("Hello, World!"),
			},
		},
		{
			name: "text masked",
			frame: &frame{
				fin:     true,
				opcode:  opcodeText,
				masked:  true,
				mask:    [4]byte{0xAA, 0xBB, 0xCC, 0xDD},
				payload: []byte("Masked message"),
			},
		},
		{
			name: "binary",
			frame: &frame{
				fin:     true,
				opcode:  opcodeBinary,
				masked:  false,
				payload: []byte{0x00, 0xFF, 0xAA, 0x55, 0x12, 0x34},
			},
		},
		{
			name: "ping",
			frame: &frame{
				fin:     true,
				opcode:  opcodePing,
				masked:  false,
				payload: []byte("ping"),
			},
		},
		{
			name: "empty close",
			frame: &frame{
				fin:     true,
				opcode:  opcodeClose,
				masked:  false,
				payload: []byte{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write frame.
			var buf bytes.Buffer
			w := bufio.NewWriter(&buf)

			if err := writeFrame(w, tt.frame); err != nil {
				t.Fatalf("writeFrame failed: %v", err)
			}

			// Read frame.
			r := bufio.NewReader(&buf)
			f, err := readFrame(r)

			if err != nil {
				t.Fatalf("readFrame failed: %v", err)
			}

			// Verify roundtrip.
			if f.fin != tt.frame.fin {
				t.Errorf("FIN: expected %v, got %v", tt.frame.fin, f.fin)
			}
			if f.opcode != tt.frame.opcode {
				t.Errorf("opcode: expected 0x%X, got 0x%X", tt.frame.opcode, f.opcode)
			}
			if f.masked != tt.frame.masked {
				t.Errorf("masked: expected %v, got %v", tt.frame.masked, f.masked)
			}
			if !bytes.Equal(f.payload, tt.frame.payload) {
				t.Errorf("payload: expected %v, got %v", tt.frame.payload, f.payload)
			}
		})
	}
}

// TestWriteFrame_InvalidOpcode tests invalid opcode error.
func TestWriteFrame_InvalidOpcode(t *testing.T) {
	f := &frame{
		fin:    true,
		opcode: 0x3, // Invalid opcode
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	err := writeFrame(w, f)
	if !errors.Is(err, ErrInvalidOpcode) {
		t.Errorf("expected ErrInvalidOpcode, got %v", err)
	}
}

// TestWriteFrame_ControlFragmented tests control frame fragmentation error.
func TestWriteFrame_ControlFragmented(t *testing.T) {
	f := &frame{
		fin:    false, // FIN=0 - INVALID for control!
		opcode: opcodeClose,
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	err := writeFrame(w, f)
	if !errors.Is(err, ErrControlFragmented) {
		t.Errorf("expected ErrControlFragmented, got %v", err)
	}
}

// TestWriteFrame_ControlTooLarge tests control frame size limit.
func TestWriteFrame_ControlTooLarge(t *testing.T) {
	f := &frame{
		fin:     true,
		opcode:  opcodePing,
		payload: bytes.Repeat([]byte("A"), 126), // 126 > 125 - INVALID!
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	err := writeFrame(w, f)
	if !errors.Is(err, ErrControlTooLarge) {
		t.Errorf("expected ErrControlTooLarge, got %v", err)
	}
}

// TestWriteFrame_InvalidUTF8 tests UTF-8 validation for text frames.
func TestWriteFrame_InvalidUTF8(t *testing.T) {
	f := &frame{
		fin:     true,
		opcode:  opcodeText,
		payload: []byte{0xFF, 0xFE, 0xFD}, // Invalid UTF-8
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	err := writeFrame(w, f)
	if !errors.Is(err, ErrInvalidUTF8) {
		t.Errorf("expected ErrInvalidUTF8, got %v", err)
	}
}

// TestReadFrame_IncompleteHeader tests handling of incomplete header.
func TestReadFrame_IncompleteHeader(t *testing.T) {
	// Only 1 byte (need 2).
	data := []byte{0x81}

	r := bufio.NewReader(bytes.NewReader(data))
	_, err := readFrame(r)

	if err == nil {
		t.Error("expected error for incomplete header")
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("expected EOF error, got %v", err)
	}
}

// TestReadFrame_IncompletePayload tests handling of incomplete payload.
func TestReadFrame_IncompletePayload(t *testing.T) {
	// Header says 5 bytes, but only 3 provided.
	data := []byte{
		0x81,          // FIN=1, opcode=0x1
		0x05,          // length=5
		'H', 'e', 'l', // Only 3 bytes!
	}

	r := bufio.NewReader(bytes.NewReader(data))
	_, err := readFrame(r)

	if err == nil {
		t.Error("expected error for incomplete payload")
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("expected EOF error, got %v", err)
	}
}

// TestIsControlFrame tests control frame detection.
func TestIsControlFrame(t *testing.T) {
	tests := []struct {
		opcode byte
		want   bool
	}{
		{opcodeContinuation, false},
		{opcodeText, false},
		{opcodeBinary, false},
		{opcodeClose, true},
		{opcodePing, true},
		{opcodePong, true},
		{0x3, false}, // Reserved
		{0xB, true},  // Reserved control
	}

	for _, tt := range tests {
		got := isControlFrame(tt.opcode)
		if got != tt.want {
			t.Errorf("isControlFrame(0x%X): expected %v, got %v", tt.opcode, tt.want, got)
		}
	}
}

// TestIsDataFrame tests data frame detection.
func TestIsDataFrame(t *testing.T) {
	tests := []struct {
		opcode byte
		want   bool
	}{
		{opcodeContinuation, true},
		{opcodeText, true},
		{opcodeBinary, true},
		{opcodeClose, false},
		{opcodePing, false},
		{opcodePong, false},
	}

	for _, tt := range tests {
		got := isDataFrame(tt.opcode)
		if got != tt.want {
			t.Errorf("isDataFrame(0x%X): expected %v, got %v", tt.opcode, tt.want, got)
		}
	}
}

// TestIsValidOpcode tests opcode validation.
func TestIsValidOpcode(t *testing.T) {
	tests := []struct {
		opcode byte
		want   bool
	}{
		{opcodeContinuation, true},
		{opcodeText, true},
		{opcodeBinary, true},
		{opcodeClose, true},
		{opcodePing, true},
		{opcodePong, true},
		{0x3, false}, // Reserved
		{0x7, false}, // Reserved
		{0xB, false}, // Reserved control
		{0xF, false}, // Reserved control
	}

	for _, tt := range tests {
		got := isValidOpcode(tt.opcode)
		if got != tt.want {
			t.Errorf("isValidOpcode(0x%X): expected %v, got %v", tt.opcode, tt.want, got)
		}
	}
}

// Benchmarks

// BenchmarkReadFrame_Small benchmarks reading small frames (< 126 bytes).
func BenchmarkReadFrame_Small(b *testing.B) {
	payload := bytes.Repeat([]byte("A"), 100)
	data := []byte{0x81, 0x64} // FIN=1, opcode=text, length=100
	data = append(data, payload...)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := bufio.NewReader(bytes.NewReader(data))
		_, err := readFrame(r)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadFrame_Medium benchmarks reading medium frames (126-65535 bytes).
func BenchmarkReadFrame_Medium(b *testing.B) {
	payloadLen := 1000
	payload := bytes.Repeat([]byte("B"), payloadLen)

	data := []byte{0x81, 126} // 16-bit length
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(payloadLen))
	data = append(data, lenBuf...)
	data = append(data, payload...)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := bufio.NewReader(bytes.NewReader(data))
		_, err := readFrame(r)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadFrame_Large benchmarks reading large frames (> 65535 bytes).
func BenchmarkReadFrame_Large(b *testing.B) {
	payloadLen := 100000
	payload := bytes.Repeat([]byte("C"), payloadLen)

	data := []byte{0x82, 127} // 64-bit length, binary
	lenBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(lenBuf, uint64(payloadLen))
	data = append(data, lenBuf...)
	data = append(data, payload...)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := bufio.NewReader(bytes.NewReader(data))
		_, err := readFrame(r)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteFrame_Small benchmarks writing small frames.
func BenchmarkWriteFrame_Small(b *testing.B) {
	f := &frame{
		fin:     true,
		opcode:  opcodeText,
		masked:  false,
		payload: bytes.Repeat([]byte("A"), 100),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)
		if err := writeFrame(w, f); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteFrame_Medium benchmarks writing medium frames.
func BenchmarkWriteFrame_Medium(b *testing.B) {
	f := &frame{
		fin:     true,
		opcode:  opcodeText,
		masked:  false,
		payload: bytes.Repeat([]byte("B"), 1000),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)
		if err := writeFrame(w, f); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteFrame_Large benchmarks writing large frames.
func BenchmarkWriteFrame_Large(b *testing.B) {
	f := &frame{
		fin:     true,
		opcode:  opcodeBinary,
		masked:  false,
		payload: bytes.Repeat([]byte("C"), 100000),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		w := bufio.NewWriter(&buf)
		if err := writeFrame(w, f); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkApplyMask benchmarks the masking algorithm.
func BenchmarkApplyMask(b *testing.B) {
	data := bytes.Repeat([]byte("Hello, WebSocket!"), 100)
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		applyMask(data, mask)
	}
}

// BenchmarkApplyMask_Large benchmarks masking large payloads.
func BenchmarkApplyMask_Large(b *testing.B) {
	data := bytes.Repeat([]byte("X"), 100000)
	mask := [4]byte{0xAA, 0xBB, 0xCC, 0xDD}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		applyMask(data, mask)
	}
}

// TestUTF8Validation tests UTF-8 validation edge cases.
func TestUTF8Validation(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		valid   bool
	}{
		{
			name:    "valid ASCII",
			payload: []byte("Hello, World!"),
			valid:   true,
		},
		{
			name:    "valid UTF-8 emoji",
			payload: []byte("Hello 👋 World 🌍"),
			valid:   true,
		},
		{
			name:    "valid UTF-8 Chinese",
			payload: []byte("你好世界"),
			valid:   true,
		},
		{
			name:    "invalid UTF-8 sequence",
			payload: []byte{0xFF, 0xFE, 0xFD},
			valid:   false,
		},
		{
			name:    "incomplete UTF-8 sequence",
			payload: []byte{0xE0, 0x80}, // Incomplete 3-byte sequence
			valid:   false,
		},
		{
			name:    "overlong encoding",
			payload: []byte{0xC0, 0x80}, // Overlong encoding of NULL
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := utf8.Valid(tt.payload)
			if valid != tt.valid {
				t.Errorf("expected valid=%v, got valid=%v for %v", tt.valid, valid, tt.payload)
			}

			// Test in frame context.
			data := []byte{0x81, byte(len(tt.payload))}
			data = append(data, tt.payload...)

			r := bufio.NewReader(bytes.NewReader(data))
			_, err := readFrame(r)

			if tt.valid && err != nil {
				t.Errorf("expected no error for valid UTF-8, got %v", err)
			}
			if !tt.valid && !errors.Is(err, ErrInvalidUTF8) {
				t.Errorf("expected ErrInvalidUTF8 for invalid UTF-8, got %v", err)
			}
		})
	}
}

// TestMaxPayloadLength tests maximum payload length enforcement.
func TestMaxPayloadLength(t *testing.T) {
	// Test data frame at limit.
	payloadLen := maxFramePayload
	data := []byte{0x82, 127} // Binary, 64-bit length
	lenBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(lenBuf, uint64(payloadLen))
	data = append(data, lenBuf...)
	// Don't actually create huge payload, just test header.

	r := bufio.NewReader(bytes.NewReader(data))
	_, err := readFrame(r)

	// Should fail on reading payload (EOF), but not on size validation.
	if err == nil {
		t.Error("expected error (EOF on payload)")
	}
	if errors.Is(err, ErrFrameTooLarge) {
		t.Error("should not reject at max size")
	}
}

// TestFragmentationSequence tests proper fragmentation handling.
func TestFragmentationSequence(t *testing.T) {
	// Simulate receiving fragmented message: text(FIN=0) → continuation(FIN=1).
	frames := [][]byte{
		{0x01, 0x03, 'H', 'e', 'l'}, // Text, FIN=0
		{0x80, 0x02, 'l', 'o'},      // Continuation, FIN=1
	}

	results := make([]string, 0, len(frames))

	for i, frameData := range frames {
		r := bufio.NewReader(bytes.NewReader(frameData))
		f, err := readFrame(r)

		if err != nil {
			t.Fatalf("frame %d: readFrame failed: %v", i, err)
		}

		results = append(results, string(f.payload))

		if i == 0 && f.fin {
			t.Error("first fragment should have FIN=0")
		}
		if i == 1 && !f.fin {
			t.Error("final fragment should have FIN=1")
		}
	}

	// Application would concatenate: "Hel" + "lo" = "Hello"
	combined := strings.Join(results, "")
	if combined != "Hello" {
		t.Errorf("expected combined 'Hello', got '%s'", combined)
	}
}

// TestReadFrame_MSBSet tests 64-bit length with MSB set (invalid).
func TestReadFrame_MSBSet(t *testing.T) {
	data := []byte{
		0x82,                                           // FIN=1, opcode=0x2 (binary)
		127,                                            // 64-bit length
		0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64, // MSB set (invalid!)
	}

	r := bufio.NewReader(bytes.NewReader(data))
	_, err := readFrame(r)

	if !errors.Is(err, ErrProtocolError) {
		t.Errorf("expected ErrProtocolError for MSB=1, got %v", err)
	}
}

// TestWriteFrame_EmptyPayload tests writing frames with empty payload.
func TestWriteFrame_EmptyPayload(t *testing.T) {
	f := &frame{
		fin:     true,
		opcode:  opcodeText,
		masked:  false,
		payload: []byte{},
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	if err := writeFrame(w, f); err != nil {
		t.Fatalf("writeFrame failed: %v", err)
	}

	// Verify: should be just 2-byte header.
	data := buf.Bytes()
	if len(data) != 2 {
		t.Errorf("expected 2 bytes for empty payload, got %d", len(data))
	}
	if data[1]&0x7F != 0 {
		t.Error("expected payload length 0")
	}
}

// TestReadFrame_IncompleteMask tests incomplete masking key.
func TestReadFrame_IncompleteMask(t *testing.T) {
	data := []byte{
		0x81,       // FIN=1, opcode=0x1
		0x85,       // MASK=1, length=5
		0x12, 0x34, // Only 2 bytes of mask (need 4!)
	}

	r := bufio.NewReader(bytes.NewReader(data))
	_, err := readFrame(r)

	if err == nil {
		t.Error("expected error for incomplete mask")
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("expected EOF error, got %v", err)
	}
}

// TestReadFrame_IncompleteExtendedLength tests incomplete extended length.
func TestReadFrame_IncompleteExtendedLength(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "16-bit length incomplete",
			data: []byte{
				0x81, // FIN=1, opcode=0x1
				126,  // 16-bit length
				0x00, // Only 1 byte (need 2!)
			},
		},
		{
			name: "64-bit length incomplete",
			data: []byte{
				0x81,                   // FIN=1, opcode=0x1
				127,                    // 64-bit length
				0x00, 0x00, 0x00, 0x00, // Only 4 bytes (need 8!)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := bufio.NewReader(bytes.NewReader(tt.data))
			_, err := readFrame(r)

			if err == nil {
				t.Error("expected error for incomplete extended length")
			}
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("expected EOF error, got %v", err)
			}
		})
	}
}

// TestWriteFrame_FrameTooLarge tests payload exceeding max size.
func TestWriteFrame_FrameTooLarge(t *testing.T) {
	// Create payload exceeding maxFramePayload.
	payloadLen := maxFramePayload + 1

	f := &frame{
		fin:     true,
		opcode:  opcodeBinary,
		masked:  false,
		payload: make([]byte, payloadLen),
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	err := writeFrame(w, f)
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("expected ErrFrameTooLarge, got %v", err)
	}
}

// TestWriteFrame_RSVBits tests writing frame with RSV bits set.
func TestWriteFrame_RSVBits(t *testing.T) {
	f := &frame{
		fin:     true,
		rsv1:    true,
		rsv2:    true,
		rsv3:    true,
		opcode:  opcodeText,
		masked:  false,
		payload: []byte("Test"),
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	if err := writeFrame(w, f); err != nil {
		t.Fatalf("writeFrame failed: %v", err)
	}

	// Verify RSV bits in header.
	data := buf.Bytes()
	if data[0]&0x40 == 0 {
		t.Error("expected RSV1=1")
	}
	if data[0]&0x20 == 0 {
		t.Error("expected RSV2=1")
	}
	if data[0]&0x10 == 0 {
		t.Error("expected RSV3=1")
	}

	// Note: readFrame would reject this due to ErrReservedBits validation.
}
