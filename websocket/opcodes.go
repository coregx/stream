// Package websocket implements RFC 6455 WebSocket protocol for real-time bidirectional communication.
//
// This package provides frame-level parsing and writing according to RFC 6455 Section 5.
// It handles:
//   - Text and binary data frames
//   - Control frames (close, ping, pong)
//   - Fragmentation and continuation
//   - Client-to-server masking
//   - Payload length encoding (7-bit, 16-bit, 64-bit)
//
// RFC Reference: https://datatracker.ietf.org/doc/html/rfc6455
package websocket

// Opcode values defined in RFC 6455 Section 5.2.
//
// Opcodes 0x0-0x2 are data frames, 0x8-0xA are control frames.
// Opcodes 0x3-0x7 and 0xB-0xF are reserved for future use.
const (
	// opcodeContinuation indicates a continuation frame (RFC 6455 Section 5.4).
	// Used for fragmented messages where FIN=0 in previous frame.
	opcodeContinuation = 0x0

	// opcodeText indicates a text data frame (RFC 6455 Section 5.6).
	// Payload must be valid UTF-8.
	opcodeText = 0x1

	// opcodeBinary indicates a binary data frame (RFC 6455 Section 5.6).
	// Payload is arbitrary binary data.
	opcodeBinary = 0x2

	// opcodeClose indicates a close control frame (RFC 6455 Section 5.5.1).
	// Initiates WebSocket closing handshake.
	opcodeClose = 0x8

	// opcodePing indicates a ping control frame (RFC 6455 Section 5.5.2).
	// Used for keepalive and latency measurement.
	opcodePing = 0x9

	// opcodePong indicates a pong control frame (RFC 6455 Section 5.5.3).
	// Response to ping frame with identical payload.
	opcodePong = 0xA
)

// isControlFrame returns true if the opcode is a control frame (0x8-0xF).
//
// RFC 6455 Section 5.5: Control frames are identified by opcodes where
// the most significant bit of the opcode is 1.
//
// Control frames:
//   - Must NOT be fragmented (FIN must be 1)
//   - May be interleaved with fragmented messages
//   - Payload length must be <= 125 bytes
func isControlFrame(opcode byte) bool {
	return opcode&0x08 != 0
}

// isDataFrame returns true if the opcode is a data frame (0x0-0x2).
//
// Data frames:
//   - May be fragmented (FIN=0 with continuation frames)
//   - No maximum payload length
//   - Text frames must contain valid UTF-8
func isDataFrame(opcode byte) bool {
	return opcode == opcodeContinuation ||
		opcode == opcodeText ||
		opcode == opcodeBinary
}

// isValidOpcode returns true if the opcode is defined in RFC 6455.
//
// Valid opcodes:
//   - 0x0: Continuation
//   - 0x1: Text
//   - 0x2: Binary
//   - 0x8: Close
//   - 0x9: Ping
//   - 0xA: Pong
//
// Opcodes 0x3-0x7 and 0xB-0xF are reserved.
func isValidOpcode(opcode byte) bool {
	switch opcode {
	case opcodeContinuation, opcodeText, opcodeBinary,
		opcodeClose, opcodePing, opcodePong:
		return true
	default:
		return false
	}
}
