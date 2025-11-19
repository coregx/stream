package websocket

import "errors"

// Protocol error types defined by RFC 6455 Section 7.4.1.

var (
	// ErrProtocolError indicates a violation of the WebSocket protocol.
	// RFC 6455 Section 7.4.1: Status code 1002.
	//
	// Causes:
	//   - Invalid frame format
	//   - Unexpected RSV bits
	//   - Invalid opcode sequence
	ErrProtocolError = errors.New("websocket: protocol error")

	// ErrInvalidUTF8 indicates text frame contains invalid UTF-8.
	// RFC 6455 Section 8.1: Text frames must contain valid UTF-8.
	// Status code 1007.
	ErrInvalidUTF8 = errors.New("websocket: invalid UTF-8 in text frame")

	// ErrFrameTooLarge indicates frame exceeds maximum allowed size.
	// Implementation-specific limit (not defined in RFC).
	//
	// Typical limits:
	//   - Control frames: 125 bytes (RFC requirement)
	//   - Data frames: configurable (e.g., 32 MB)
	ErrFrameTooLarge = errors.New("websocket: frame too large")

	// ErrReservedBits indicates RSV1/RSV2/RSV3 bits are set.
	// RFC 6455 Section 5.2: Reserved bits must be 0 unless extension negotiated.
	// Status code 1002 (protocol error).
	ErrReservedBits = errors.New("websocket: reserved bits must be 0")

	// ErrInvalidOpcode indicates an unknown or reserved opcode.
	// RFC 6455 Section 5.2: Opcodes 0x3-0x7 and 0xB-0xF are reserved.
	// Status code 1002 (protocol error).
	ErrInvalidOpcode = errors.New("websocket: invalid opcode")

	// ErrControlFragmented indicates a control frame with FIN=0.
	// RFC 6455 Section 5.5: Control frames must NOT be fragmented.
	// Status code 1002 (protocol error).
	ErrControlFragmented = errors.New("websocket: control frame must not be fragmented")

	// ErrControlTooLarge indicates control frame payload > 125 bytes.
	// RFC 6455 Section 5.5: Control frame payload length must be <= 125.
	// Status code 1002 (protocol error).
	ErrControlTooLarge = errors.New("websocket: control frame payload too large")

	// ErrUnexpectedContinuation indicates continuation frame without initial frame.
	// RFC 6455 Section 5.4: Continuation requires prior data frame with FIN=0.
	// Status code 1002 (protocol error).
	ErrUnexpectedContinuation = errors.New("websocket: unexpected continuation frame")

	// ErrMaskRequired indicates client frame without masking.
	// RFC 6455 Section 5.3: Client-to-server frames MUST be masked.
	// Status code 1002 (protocol error).
	ErrMaskRequired = errors.New("websocket: client frames must be masked")

	// ErrMaskUnexpected indicates server frame with masking.
	// RFC 6455 Section 5.3: Server-to-client frames MUST NOT be masked.
	// Status code 1002 (protocol error).
	ErrMaskUnexpected = errors.New("websocket: server frames must not be masked")

	// Handshake error types (RFC 6455 Section 4).

	// ErrInvalidMethod indicates HTTP method is not GET.
	// RFC 6455 Section 4.1: Handshake MUST use GET method.
	ErrInvalidMethod = errors.New("websocket: method must be GET")

	// ErrMissingUpgrade indicates missing or invalid Upgrade header.
	// RFC 6455 Section 4.2.1: Must contain "websocket" (case-insensitive).
	ErrMissingUpgrade = errors.New("websocket: missing or invalid Upgrade header")

	// ErrMissingConnection indicates missing or invalid Connection header.
	// RFC 6455 Section 4.2.1: Must contain "Upgrade" (case-insensitive).
	ErrMissingConnection = errors.New("websocket: missing or invalid Connection header")

	// ErrMissingSecKey indicates missing Sec-WebSocket-Key header.
	// RFC 6455 Section 4.2.1: Required for handshake.
	ErrMissingSecKey = errors.New("websocket: missing Sec-WebSocket-Key header")

	// ErrInvalidVersion indicates unsupported WebSocket version.
	// RFC 6455 Section 4.4: Only version 13 is supported.
	ErrInvalidVersion = errors.New("websocket: unsupported WebSocket version")

	// ErrOriginDenied indicates origin check failed.
	// Application-level security check (not RFC requirement).
	ErrOriginDenied = errors.New("websocket: origin check failed")

	// ErrHijackFailed indicates HTTP connection cannot be hijacked.
	// Required for upgrading to WebSocket protocol.
	ErrHijackFailed = errors.New("websocket: cannot hijack connection")

	// Connection error types (runtime errors).

	// ErrClosed indicates connection is already closed.
	// Returned when attempting to read/write on closed connection.
	ErrClosed = errors.New("websocket: connection closed")

	// ErrInvalidMessageType indicates invalid message type for operation.
	// For example, calling ReadText() on binary message.
	ErrInvalidMessageType = errors.New("websocket: invalid message type")

	// ErrMessageTooLarge indicates message exceeds maximum size.
	// Configurable via UpgradeOptions.MaxMessageSize (default: 32 MB).
	// Status code 1009 (message too big).
	ErrMessageTooLarge = errors.New("websocket: message too large")
)
