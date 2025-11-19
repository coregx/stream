package sse

import (
	"strings"
	"testing"
)

// TestEvent_String_SingleLine tests serialization of single-line data.
func TestEvent_String_SingleLine(t *testing.T) {
	event := NewEvent("hello")
	expected := "data: hello\n\n"
	if got := event.String(); got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

// TestEvent_String_MultiLine tests serialization of multi-line data.
func TestEvent_String_MultiLine(t *testing.T) {
	event := NewEvent("line1\nline2\nline3")
	expected := "data: line1\ndata: line2\ndata: line3\n\n"
	if got := event.String(); got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

// TestEvent_String_AllFields tests serialization with all fields populated.
func TestEvent_String_AllFields(t *testing.T) {
	event := NewEvent("test data").
		WithType("message").
		WithID("123").
		WithRetry(3000)

	result := event.String()

	// Check all fields present in correct order
	// Order: event, id, retry, data (per SSE spec)
	if !strings.Contains(result, "event: message\n") {
		t.Error("missing event type")
	}
	if !strings.Contains(result, "id: 123\n") {
		t.Error("missing id")
	}
	if !strings.Contains(result, "retry: 3000\n") {
		t.Error("missing retry")
	}
	if !strings.Contains(result, "data: test data\n") {
		t.Error("missing data")
	}
	if !strings.HasSuffix(result, "\n\n") {
		t.Error("missing double newline")
	}
}

// TestEvent_String_TypeOnly tests serialization with type field.
func TestEvent_String_TypeOnly(t *testing.T) {
	event := NewEvent("data").WithType("notification")
	result := event.String()

	if !strings.Contains(result, "event: notification\n") {
		t.Error("missing event type")
	}
	if !strings.Contains(result, "data: data\n") {
		t.Error("missing data")
	}
	if strings.Contains(result, "id:") {
		t.Error("unexpected id field")
	}
	if strings.Contains(result, "retry:") {
		t.Error("unexpected retry field")
	}
}

// TestEvent_String_IDOnly tests serialization with ID field.
func TestEvent_String_IDOnly(t *testing.T) {
	event := NewEvent("data").WithID("msg-999")
	result := event.String()

	if !strings.Contains(result, "id: msg-999\n") {
		t.Error("missing id")
	}
	if !strings.Contains(result, "data: data\n") {
		t.Error("missing data")
	}
	if strings.Contains(result, "event:") {
		t.Error("unexpected event type field")
	}
	if strings.Contains(result, "retry:") {
		t.Error("unexpected retry field")
	}
}

// TestEvent_String_RetryOnly tests serialization with retry field.
func TestEvent_String_RetryOnly(t *testing.T) {
	event := NewEvent("data").WithRetry(5000)
	result := event.String()

	if !strings.Contains(result, "retry: 5000\n") {
		t.Error("missing retry")
	}
	if !strings.Contains(result, "data: data\n") {
		t.Error("missing data")
	}
	if strings.Contains(result, "event:") {
		t.Error("unexpected event type field")
	}
	if strings.Contains(result, "id:") {
		t.Error("unexpected id field")
	}
}

// TestEvent_String_EmptyData tests serialization with empty data.
func TestEvent_String_EmptyData(t *testing.T) {
	event := NewEvent("")
	expected := "data: \n\n"
	if got := event.String(); got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

// TestEvent_String_SpecialCharacters tests data with special characters.
func TestEvent_String_SpecialCharacters(t *testing.T) {
	event := NewEvent("special: chars, with\ttabs and  spaces")
	result := event.String()

	if !strings.Contains(result, "data: special: chars, with\ttabs and  spaces\n") {
		t.Error("special characters not preserved")
	}
}

// TestEvent_Builder_Pattern tests builder pattern chaining.
func TestEvent_Builder_Pattern(t *testing.T) {
	event := NewEvent("data").WithType("ping").WithID("1").WithRetry(1000)

	if event.Type != "ping" {
		t.Errorf("expected ping, got %s", event.Type)
	}
	if event.ID != "1" {
		t.Errorf("expected 1, got %s", event.ID)
	}
	if event.Retry != 1000 {
		t.Errorf("expected 1000, got %d", event.Retry)
	}
	if event.Data != "data" {
		t.Errorf("expected data, got %s", event.Data)
	}
}

// TestEvent_Builder_PartialChain tests partial builder chaining.
func TestEvent_Builder_PartialChain(t *testing.T) {
	event := NewEvent("test").WithType("update")

	if event.Type != "update" {
		t.Errorf("expected update, got %s", event.Type)
	}
	if event.ID != "" {
		t.Error("expected empty ID")
	}
	if event.Retry != 0 {
		t.Error("expected zero Retry")
	}
}

// TestComment tests comment creation.
func TestComment(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{"simple", "keep-alive", ": keep-alive\n\n"},
		{"empty", "", ": \n\n"},
		{"multiword", "connection active", ": connection active\n\n"},
		{"special chars", "debug: test-123", ": debug: test-123\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Comment(tt.text); got != tt.expected {
				t.Errorf("Comment(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

// TestEvent_String_ComplexMultiline tests complex multi-line scenarios.
func TestEvent_String_ComplexMultiline(t *testing.T) {
	event := NewEvent("{\n  \"user\": \"Alice\",\n  \"action\": \"login\"\n}").
		WithType("json").
		WithID("event-42")

	result := event.String()

	// Check correct multi-line handling of JSON
	if !strings.Contains(result, "data: {\n") {
		t.Error("missing first data line")
	}
	if !strings.Contains(result, "data:   \"user\": \"Alice\",\n") {
		t.Error("missing second data line")
	}
	if !strings.Contains(result, "data:   \"action\": \"login\"\n") {
		t.Error("missing third data line")
	}
	if !strings.Contains(result, "data: }\n") {
		t.Error("missing fourth data line")
	}
}

// TestEvent_Immutability tests that builder pattern doesn't modify original.
func TestEvent_Immutability(t *testing.T) {
	event1 := NewEvent("original")
	event2 := event1.WithType("modified")

	// Both should point to same Event (builder pattern mutates)
	if event1 != event2 {
		t.Error("builder pattern should return same event")
	}
	if event1.Type != "modified" {
		t.Error("builder pattern should mutate the event")
	}
}

// BenchmarkEvent_String benchmarks event serialization.
func BenchmarkEvent_String(b *testing.B) {
	event := NewEvent("hello world").WithType("message").WithID("123")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = event.String()
	}
}

// BenchmarkEvent_String_Multiline benchmarks multi-line event serialization.
func BenchmarkEvent_String_Multiline(b *testing.B) {
	event := NewEvent("line1\nline2\nline3\nline4\nline5")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = event.String()
	}
}

// BenchmarkEvent_Builder benchmarks builder pattern.
func BenchmarkEvent_Builder(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewEvent("data").WithType("ping").WithID("1").WithRetry(1000)
	}
}

// BenchmarkComment benchmarks comment creation.
func BenchmarkComment(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Comment("keep-alive")
	}
}
