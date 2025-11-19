package websocket

// This file exports internal types and functions for testing.
// It is only compiled during tests (build tag: test).

import (
	"bufio"
	"net"
)

// Test exports for frame operations.

// FrameForTest is an exported version of frame for testing.
type FrameForTest struct {
	Fin     bool
	Rsv1    bool
	Rsv2    bool
	Rsv3    bool
	Opcode  byte
	Masked  bool
	Mask    [4]byte
	Payload []byte
}

// ReadFrameForTest reads a frame (exported for testing).
func ReadFrameForTest(r *bufio.Reader) (*FrameForTest, error) {
	f, err := readFrame(r)
	if err != nil {
		return nil, err
	}

	return &FrameForTest{
		Fin:     f.fin,
		Rsv1:    f.rsv1,
		Rsv2:    f.rsv2,
		Rsv3:    f.rsv3,
		Opcode:  f.opcode,
		Masked:  f.masked,
		Mask:    f.mask,
		Payload: f.payload,
	}, nil
}

// WriteFrameForTest writes a frame (exported for testing).
func WriteFrameForTest(w *bufio.Writer, ft *FrameForTest) error {
	f := &frame{
		fin:     ft.Fin,
		rsv1:    ft.Rsv1,
		rsv2:    ft.Rsv2,
		rsv3:    ft.Rsv3,
		opcode:  ft.Opcode,
		masked:  ft.Masked,
		mask:    ft.Mask,
		payload: ft.Payload,
	}

	return writeFrame(w, f)
}

// GetReaderForTest returns internal reader from Conn (exported for testing).
//
// Use this to access low-level frame operations in tests.
// For normal operations, use conn.Read() / conn.ReadText() / conn.ReadJSON().
func GetReaderForTest(conn *Conn) *bufio.Reader {
	return conn.reader
}

// GetWriterForTest returns internal writer from Conn (exported for testing).
//
// Use this to access low-level frame operations in tests.
// For normal operations, use conn.Write() / conn.WriteText() / conn.WriteJSON().
func GetWriterForTest(conn *Conn) *bufio.Writer {
	return conn.writer
}

// ApplyMaskForTest applies XOR mask to payload (exported for testing).
func ApplyMaskForTest(data []byte, mask [4]byte) {
	applyMask(data, mask)
}

// WriteFrameNoValidationForTest writes a frame without UTF-8 validation.
//
// Used for testing edge cases (invalid UTF-8, protocol violations).
// Normal code should use WriteFrameForTest which validates.
func WriteFrameNoValidationForTest(w *bufio.Writer, ft *FrameForTest) error {
	f := &frame{
		fin:     ft.Fin,
		rsv1:    ft.Rsv1,
		rsv2:    ft.Rsv2,
		rsv3:    ft.Rsv3,
		opcode:  ft.Opcode,
		masked:  ft.Masked,
		mask:    ft.Mask,
		payload: ft.Payload,
	}

	return writeFrameNoValidation(w, f)
}

// Opcode constants for testing.
const (
	OpcodeContinuationForTest = opcodeContinuation
	OpcodeTextForTest         = opcodeText
	OpcodeBinaryForTest       = opcodeBinary
	OpcodeCloseForTest        = opcodeClose
	OpcodePingForTest         = opcodePing
	OpcodePongForTest         = opcodePong
)

// NewConnForTest creates a Conn from a raw net.Conn for testing.
//
// This is used by test clients that perform manual WebSocket handshakes.
// isServer: true for server-side connections, false for client-side.
func NewConnForTest(conn net.Conn, reader *bufio.Reader, isServer bool) *Conn {
	return &Conn{
		conn:     conn,
		reader:   reader,
		writer:   bufio.NewWriter(conn),
		isServer: isServer,
	}
}
