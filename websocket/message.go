package websocket

import "errors"

// MessageType represents WebSocket message type.
//
// WebSocket supports two application message types (RFC 6455 Section 5.6):
// - Text (UTF-8 encoded text).
// - Binary (arbitrary binary data).
type MessageType int

const (
	// TextMessage represents a UTF-8 text message (opcode 0x1).
	// Text frames MUST contain valid UTF-8 data (RFC 6455 Section 8.1).
	TextMessage MessageType = 1

	// BinaryMessage represents a binary data message (opcode 0x2).
	// Binary frames can contain arbitrary binary data.
	BinaryMessage MessageType = 2
)

// String returns string representation of message type.
func (mt MessageType) String() string {
	switch mt {
	case TextMessage:
		return "Text"
	case BinaryMessage:
		return "Binary"
	default:
		return "Unknown"
	}
}

// CloseCode represents WebSocket close status codes (RFC 6455 Section 7.4).
//
// Close frames MAY contain a status code indicating the reason for closure.
// Status codes 1000-4999 are defined by the WebSocket protocol.
type CloseCode int

const (
	// CloseNormalClosure indicates normal closure (1000).
	// Connection purpose fulfilled.
	CloseNormalClosure CloseCode = 1000

	// CloseGoingAway indicates endpoint going away (1001).
	// Server shutting down or browser navigating away.
	CloseGoingAway CloseCode = 1001

	// CloseProtocolError indicates protocol error (1002).
	// Endpoint received frame it cannot understand.
	CloseProtocolError CloseCode = 1002

	// CloseUnsupportedData indicates unsupported data type (1003).
	// Endpoint received data type it cannot accept (e.g. text-only endpoint got binary).
	CloseUnsupportedData CloseCode = 1003

	// 1004 is reserved and MUST NOT be used.

	// CloseNoStatusReceived indicates no status code was received (1005).
	// This is a reserved value and MUST NOT be set in close frame.
	// Used internally when close frame has no status code.
	CloseNoStatusReceived CloseCode = 1005

	// CloseAbnormalClosure indicates abnormal closure (1006).
	// This is a reserved value and MUST NOT be set in close frame.
	// Used internally when connection closed without close frame (e.g. TCP error).
	CloseAbnormalClosure CloseCode = 1006

	// CloseInvalidFramePayloadData indicates invalid frame payload (1007).
	// Message payload contains invalid data (e.g. invalid UTF-8 in text frame).
	CloseInvalidFramePayloadData CloseCode = 1007

	// ClosePolicyViolation indicates policy violation (1008).
	// Endpoint received message violating its policy (generic status code).
	ClosePolicyViolation CloseCode = 1008

	// CloseMessageTooBig indicates message too large (1009).
	// Endpoint received message too big to process.
	CloseMessageTooBig CloseCode = 1009

	// CloseMandatoryExtension indicates missing extension (1010).
	// Client expected server to negotiate one or more extensions, but server didn't.
	CloseMandatoryExtension CloseCode = 1010

	// CloseInternalServerErr indicates internal server error (1011).
	// Server encountered unexpected condition preventing it from fulfilling request.
	CloseInternalServerErr CloseCode = 1011

	// CloseServiceRestart indicates service restart (1012).
	// Server is restarting.
	CloseServiceRestart CloseCode = 1012

	// CloseTryAgainLater indicates try again later (1013).
	// Server is temporarily unable to process request (e.g. overloaded).
	CloseTryAgainLater CloseCode = 1013

	// 1014 is reserved and MUST NOT be used.

	// CloseTLSHandshake indicates TLS handshake failure (1015).
	// This is a reserved value and MUST NOT be set in close frame.
	// Used internally when TLS handshake fails.
	CloseTLSHandshake CloseCode = 1015
)

// String returns string representation of close code.
//
//nolint:cyclop // 15 close codes per RFC 6455
func (cc CloseCode) String() string {
	switch cc {
	case CloseNormalClosure:
		return "Normal Closure"
	case CloseGoingAway:
		return "Going Away"
	case CloseProtocolError:
		return "Protocol Error"
	case CloseUnsupportedData:
		return "Unsupported Data"
	case CloseNoStatusReceived:
		return "No Status Received"
	case CloseAbnormalClosure:
		return "Abnormal Closure"
	case CloseInvalidFramePayloadData:
		return "Invalid Frame Payload Data"
	case ClosePolicyViolation:
		return "Policy Violation"
	case CloseMessageTooBig:
		return "Message Too Big"
	case CloseMandatoryExtension:
		return "Mandatory Extension"
	case CloseInternalServerErr:
		return "Internal Server Error"
	case CloseServiceRestart:
		return "Service Restart"
	case CloseTryAgainLater:
		return "Try Again Later"
	case CloseTLSHandshake:
		return "TLS Handshake"
	default:
		return "Unknown"
	}
}

// IsCloseError checks if error represents a WebSocket close frame.
//
// Returns true if the error is a clean close (close frame received).
// Returns false for other errors (network errors, protocol errors, etc.).
func IsCloseError(err error) bool {
	if err == nil {
		return false
	}
	// Check if error is ErrClosed (close frame sent/received)
	return errors.Is(err, ErrClosed)
}

// IsTemporaryError checks if error is temporary and operation can be retried.
//
// Returns true for transient network errors.
// Returns false for permanent errors (close frame, protocol errors).
func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	// Check for temporary network errors
	type temporary interface {
		Temporary() bool
	}

	if te, ok := err.(temporary); ok {
		return te.Temporary()
	}

	return false
}
