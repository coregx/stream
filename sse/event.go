// Package sse implements Server-Sent Events (SSE) according to the text/event-stream specification.
//
// SSE provides a simple, unidirectional protocol for sending real-time updates from server to client
// over HTTP. It's ideal for scenarios like live notifications, dashboards, and activity streams.
package sse

import (
	"fmt"
	"strings"
)

// Event represents a Server-Sent Event.
//
// An Event consists of optional type, ID, retry fields, and required data field.
// Events are serialized according to the text/event-stream specification.
//
// Example:
//
//	event := sse.NewEvent("Hello, World!").
//	    WithType("greeting").
//	    WithID("msg-1").
//	    WithRetry(3000)
//
//	fmt.Println(event.String())
//	// Output:
//	// event: greeting
//	// id: msg-1
//	// retry: 3000
//	// data: Hello, World!
//	//
type Event struct {
	// Type is the event type (optional).
	// If empty, client receives generic "message" event.
	// Maps to "event:" field in SSE format.
	Type string

	// ID is the event ID for Last-Event-ID tracking (optional).
	// Client sends this in reconnect via Last-Event-ID header.
	// Maps to "id:" field in SSE format.
	ID string

	// Data is the event data (required).
	// Can be single line or multi-line.
	// Maps to "data:" field(s) in SSE format.
	Data string

	// Retry is the reconnection time in milliseconds (optional).
	// Tells client how long to wait before reconnecting on disconnect.
	// Maps to "retry:" field in SSE format.
	Retry int
}

// NewEvent creates a new Event with the specified data.
//
// The returned Event can be further customized using builder methods:
// WithType, WithID, WithRetry.
//
// Example:
//
//	event := sse.NewEvent("User logged in")
func NewEvent(data string) *Event {
	return &Event{Data: data}
}

// WithType sets the event type.
//
// Example:
//
//	event := sse.NewEvent("data").WithType("notification")
func (e *Event) WithType(typ string) *Event {
	e.Type = typ
	return e
}

// WithID sets the event ID.
//
// This is used for client reconnection tracking via Last-Event-ID header.
//
// Example:
//
//	event := sse.NewEvent("data").WithID("msg-123")
func (e *Event) WithID(id string) *Event {
	e.ID = id
	return e
}

// WithRetry sets the reconnection retry time in milliseconds.
//
// Example:
//
//	event := sse.NewEvent("data").WithRetry(3000) // 3 seconds
func (e *Event) WithRetry(ms int) *Event {
	e.Retry = ms
	return e
}

// String serializes the Event to SSE text/event-stream format.
//
// The format follows the SSE specification:
//   - Each field starts with field name + colon + space
//   - Multi-line data becomes multiple "data:" lines
//   - Message ends with double newline (\n\n)
//
// Example:
//
//	event := sse.NewEvent("line1\nline2").WithType("multiline")
//	fmt.Println(event.String())
//	// Output:
//	// event: multiline
//	// data: line1
//	// data: line2
//	//
func (e *Event) String() string {
	var b strings.Builder

	// Event type (optional)
	if e.Type != "" {
		b.WriteString("event: ")
		b.WriteString(e.Type)
		b.WriteByte('\n')
	}

	// Event ID (optional)
	if e.ID != "" {
		b.WriteString("id: ")
		b.WriteString(e.ID)
		b.WriteByte('\n')
	}

	// Retry (optional)
	if e.Retry > 0 {
		b.WriteString("retry: ")
		b.WriteString(fmt.Sprintf("%d", e.Retry))
		b.WriteByte('\n')
	}

	// Data (required) - handle multi-line
	lines := strings.Split(e.Data, "\n")
	for _, line := range lines {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteByte('\n')
	}

	// End with double newline
	b.WriteByte('\n')

	return b.String()
}

// Comment creates an SSE comment for keep-alive or debugging.
//
// Comments start with colon (:) and are ignored by clients.
// They're commonly used to keep the connection alive and prevent timeouts.
//
// Example:
//
//	keepAlive := sse.Comment("keep-alive")
//	fmt.Println(keepAlive)
//	// Output:
//	// : keep-alive
//	//
func Comment(text string) string {
	return ": " + text + "\n\n"
}
