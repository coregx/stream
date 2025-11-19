package websocket

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf8"
)

// Maximum payload sizes (implementation limits).
const (
	// maxControlPayload is the maximum payload length for control frames.
	// RFC 6455 Section 5.5: Control frames must have payload <= 125 bytes.
	maxControlPayload = 125

	// maxFramePayload is the maximum payload length for data frames.
	// Default: 32 MB (configurable in production).
	maxFramePayload = 32 * 1024 * 1024

	// Payload length encoding thresholds (RFC 6455 Section 5.2).
	payloadLen7Bit  = 125 // 0-125: stored in 7 bits
	payloadLen16Bit = 126 // 126: followed by 16-bit length
	payloadLen64Bit = 127 // 127: followed by 64-bit length
)

// frame represents a WebSocket frame as defined in RFC 6455 Section 5.2.
//
// Frame structure:
//
//	 0                   1                   2                   3
//	 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//	+-+-+-+-+-------+-+-------------+-------------------------------+
//	|F|R|R|R| opcode|M| Payload len |    Extended payload length    |
//	|I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
//	|N|V|V|V|       |S|             |   (if payload len==126/127)   |
//	| |1|2|3|       |K|             |                               |
//	+-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
//	|     Extended payload length continued, if payload len == 127  |
//	+ - - - - - - - - - - - - - - - +-------------------------------+
//	|                               |Masking-key, if MASK set to 1  |
//	+-------------------------------+-------------------------------+
//	| Masking-key (continued)       |          Payload Data         |
//	+-------------------------------- - - - - - - - - - - - - - - - +
//	:                     Payload Data continued ...                :
//	+ - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
//	|                     Payload Data continued ...                |
//	+---------------------------------------------------------------+
type frame struct {
	// fin indicates this is the final fragment (FIN bit).
	// RFC 6455 Section 5.2: If true, this is the last fragment of a message.
	fin bool

	// rsv1, rsv2, rsv3 are reserved bits for extensions.
	// RFC 6455 Section 5.2: Must be 0 unless extension negotiated.
	rsv1, rsv2, rsv3 bool

	// opcode is the frame operation code (4 bits).
	// RFC 6455 Section 5.2: Defines frame type (text, binary, control).
	opcode byte

	// masked indicates if payload is masked (MASK bit).
	// RFC 6455 Section 5.3: Client-to-server frames MUST be masked.
	masked bool

	// mask is the 32-bit masking key.
	// RFC 6455 Section 5.3: Used to XOR payload data.
	mask [4]byte

	// payload is the frame payload data.
	// For text frames: must be valid UTF-8.
	// For control frames: length must be <= 125 bytes.
	payload []byte
}

// readFrame reads a WebSocket frame from the buffered reader.
//
// RFC 6455 Section 5.2: Base Framing Protocol.
//
// Steps:
//  1. Read 2-byte header (FIN, RSV, opcode, MASK, payload length)
//  2. Read extended payload length if needed (16-bit or 64-bit)
//  3. Read masking key if MASK=1 (4 bytes)
//  4. Read payload data
//  5. Unmask payload if masked
//  6. Validate UTF-8 for text frames
//  7. Validate control frame constraints
//
// Returns:
//   - frame: parsed frame structure
//   - error: validation or I/O error
func readFrame(r *bufio.Reader) (*frame, error) {
	// Step 1: Read 2-byte header.
	// Byte 0: FIN(1) RSV(3) Opcode(4)
	// Byte 1: MASK(1) PayloadLen(7)
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	f := &frame{
		fin:    header[0]&0x80 != 0,
		rsv1:   header[0]&0x40 != 0,
		rsv2:   header[0]&0x20 != 0,
		rsv3:   header[0]&0x10 != 0,
		opcode: header[0] & 0x0F,
		masked: header[1]&0x80 != 0,
	}

	// Validate opcode.
	if !isValidOpcode(f.opcode) {
		return nil, fmt.Errorf("%w: 0x%X", ErrInvalidOpcode, f.opcode)
	}

	// Validate reserved bits (must be 0 unless extension negotiated).
	// RFC 6455 Section 5.2: RSV bits reserved for extensions.
	if f.rsv1 || f.rsv2 || f.rsv3 {
		return nil, ErrReservedBits
	}

	// Validate control frame constraints.
	// RFC 6455 Section 5.5: Control frames must NOT be fragmented.
	if isControlFrame(f.opcode) && !f.fin {
		return nil, ErrControlFragmented
	}

	// Step 2: Read payload length (7-bit, 16-bit, or 64-bit).
	payloadLen := uint64(header[1] & 0x7F)

	switch payloadLen {
	case payloadLen16Bit:
		// 16-bit extended payload length.
		buf := make([]byte, 2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("read 16-bit length: %w", err)
		}
		payloadLen = uint64(binary.BigEndian.Uint16(buf))
	case payloadLen64Bit:
		// 64-bit extended payload length.
		buf := make([]byte, 8)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("read 64-bit length: %w", err)
		}
		payloadLen = binary.BigEndian.Uint64(buf)
		// RFC 6455 Section 5.2: Most significant bit must be 0.
		if payloadLen&(1<<63) != 0 {
			return nil, ErrProtocolError
		}
	}

	// Validate control frame payload length.
	// RFC 6455 Section 5.5: Control frames must have payload <= 125 bytes.
	if isControlFrame(f.opcode) && payloadLen > maxControlPayload {
		return nil, ErrControlTooLarge
	}

	// Validate data frame payload length (implementation limit).
	if payloadLen > maxFramePayload {
		return nil, fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, payloadLen)
	}

	// Step 3: Read masking key if MASK=1.
	// RFC 6455 Section 5.3: Client-to-server frames MUST be masked.
	if f.masked {
		if _, err := io.ReadFull(r, f.mask[:]); err != nil {
			return nil, fmt.Errorf("read mask: %w", err)
		}
	}

	// Step 4: Read payload data.
	if payloadLen > 0 {
		f.payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, f.payload); err != nil {
			return nil, fmt.Errorf("read payload: %w", err)
		}

		// Step 5: Unmask payload if masked.
		// RFC 6455 Section 5.3: Client applies masking-key to payload.
		if f.masked {
			applyMask(f.payload, f.mask)
		}
	}

	// Step 6: Validate UTF-8 for text frames.
	// RFC 6455 Section 8.1: Text frames must contain valid UTF-8.
	if f.opcode == opcodeText && !utf8.Valid(f.payload) {
		return nil, ErrInvalidUTF8
	}

	return f, nil
}

// writeFrame writes a WebSocket frame to the buffered writer.
//
// RFC 6455 Section 5.2: Base Framing Protocol.
//
// Steps:
//  1. Write 2-byte header (FIN, RSV, opcode, MASK, payload length)
//  2. Write extended payload length if needed
//  3. Write masking key if MASK=1
//  4. Apply mask to payload copy
//  5. Write masked payload
//  6. Flush buffer
//
// Returns:
//   - error: validation or I/O error
func writeFrame(w *bufio.Writer, f *frame) error {
	// Validate opcode.
	if !isValidOpcode(f.opcode) {
		return fmt.Errorf("%w: 0x%X", ErrInvalidOpcode, f.opcode)
	}

	// Validate control frame constraints.
	if isControlFrame(f.opcode) {
		if !f.fin {
			return ErrControlFragmented
		}
		if len(f.payload) > maxControlPayload {
			return ErrControlTooLarge
		}
	}

	// Validate UTF-8 for text frames.
	if f.opcode == opcodeText && !utf8.Valid(f.payload) {
		return ErrInvalidUTF8
	}

	// Validate payload length (implementation limit).
	if len(f.payload) > maxFramePayload {
		return fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, len(f.payload))
	}

	// Step 1: Write 2-byte header.
	header := make([]byte, 2)

	// Byte 0: FIN(1) RSV(3) Opcode(4)
	if f.fin {
		header[0] |= 0x80
	}
	if f.rsv1 {
		header[0] |= 0x40
	}
	if f.rsv2 {
		header[0] |= 0x20
	}
	if f.rsv3 {
		header[0] |= 0x10
	}
	header[0] |= f.opcode & 0x0F

	// Byte 1: MASK(1) PayloadLen(7)
	if f.masked {
		header[1] |= 0x80
	}

	payloadLen := uint64(len(f.payload))

	// Determine payload length encoding.
	//nolint:gosec // G602: False positive - header is always length 2
	switch {
	case payloadLen <= payloadLen7Bit:
		// 7-bit length (0-125).
		header[1] |= byte(payloadLen)
	case payloadLen <= 0xFFFF:
		// 16-bit extended length.
		header[1] |= payloadLen16Bit
	default:
		// 64-bit extended length.
		header[1] |= payloadLen64Bit
	}

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Step 2: Write extended payload length if needed.
	if payloadLen > payloadLen7Bit && payloadLen <= 0xFFFF {
		// 16-bit extended length.
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, uint16(payloadLen))
		if _, err := w.Write(buf); err != nil {
			return fmt.Errorf("write 16-bit length: %w", err)
		}
	} else if payloadLen > 0xFFFF {
		// 64-bit extended length.
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, payloadLen)
		if _, err := w.Write(buf); err != nil {
			return fmt.Errorf("write 64-bit length: %w", err)
		}
	}

	// Step 3: Write masking key if MASK=1.
	if f.masked {
		if _, err := w.Write(f.mask[:]); err != nil {
			return fmt.Errorf("write mask: %w", err)
		}
	}

	// Step 4-5: Apply mask and write payload.
	if len(f.payload) > 0 {
		// Make a copy to avoid modifying original payload.
		payload := make([]byte, len(f.payload))
		copy(payload, f.payload)

		if f.masked {
			applyMask(payload, f.mask)
		}

		if _, err := w.Write(payload); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}

	// Step 6: Flush buffer.
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}

	return nil
}

// writeFrameNoValidation writes a WebSocket frame without validation.
//
// Used ONLY for testing edge cases (invalid UTF-8, protocol violations).
// Production code should use writeFrame() which enforces RFC 6455 compliance.
//
// Skips validation of:
//   - Opcode validity
//   - Control frame constraints
//   - UTF-8 encoding
//   - Payload length limits
//
// Returns:
//   - error: only I/O errors
func writeFrameNoValidation(w *bufio.Writer, f *frame) error {
	// Step 1: Write 2-byte header.
	header := make([]byte, 2)

	// Byte 0: FIN(1) RSV(3) Opcode(4)
	if f.fin {
		header[0] |= 0x80
	}
	if f.rsv1 {
		header[0] |= 0x40
	}
	if f.rsv2 {
		header[0] |= 0x20
	}
	if f.rsv3 {
		header[0] |= 0x10
	}
	header[0] |= f.opcode & 0x0F

	// Byte 1: MASK(1) PayloadLen(7)
	if f.masked {
		header[1] |= 0x80
	}

	payloadLen := uint64(len(f.payload))

	// Determine payload length encoding.
	//nolint:gosec // G602: False positive - header is always length 2
	switch {
	case payloadLen <= payloadLen7Bit:
		// 7-bit length (0-125).
		header[1] |= byte(payloadLen)
	case payloadLen <= 0xFFFF:
		// 16-bit extended length.
		header[1] |= payloadLen16Bit
	default:
		// 64-bit extended length.
		header[1] |= payloadLen64Bit
	}

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Step 2: Write extended payload length if needed.
	if payloadLen > payloadLen7Bit && payloadLen <= 0xFFFF {
		// 16-bit extended length.
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, uint16(payloadLen))
		if _, err := w.Write(buf); err != nil {
			return fmt.Errorf("write 16-bit length: %w", err)
		}
	} else if payloadLen > 0xFFFF {
		// 64-bit extended length.
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, payloadLen)
		if _, err := w.Write(buf); err != nil {
			return fmt.Errorf("write 64-bit length: %w", err)
		}
	}

	// Step 3: Write masking key if MASK=1.
	if f.masked {
		if _, err := w.Write(f.mask[:]); err != nil {
			return fmt.Errorf("write mask: %w", err)
		}
	}

	// Step 4-5: Apply mask and write payload.
	if len(f.payload) > 0 {
		// Make a copy to avoid modifying original payload.
		payload := make([]byte, len(f.payload))
		copy(payload, f.payload)

		if f.masked {
			applyMask(payload, f.mask)
		}

		if _, err := w.Write(payload); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}

	// Step 6: Flush buffer.
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}

	return nil
}

// applyMask applies the WebSocket masking algorithm to data.
//
// RFC 6455 Section 5.3: Client-to-Server Masking.
//
// Algorithm:
//
//	transformed-octet-i = original-octet-i XOR masking-key-octet-j
//	where j = i MOD 4
//
// Properties:
//   - XOR is reversible: applying mask twice restores original
//   - Modifies data in-place for efficiency
//   - Works for both masking and unmasking
//
// Parameters:
//   - data: payload to mask/unmask (modified in-place)
//   - mask: 4-byte masking key
func applyMask(data []byte, mask [4]byte) {
	// XOR each byte with corresponding mask byte (cycling through 4 bytes).
	for i := range data {
		data[i] ^= mask[i%4]
	}
}
