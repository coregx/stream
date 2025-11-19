package sse

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockResponseWriter is a mock that doesn't implement http.Flusher.
type mockResponseWriter struct {
	header http.Header
	body   strings.Builder
	status int
}

func newMockResponseWriter() *mockResponseWriter {
	return &mockResponseWriter{
		header: make(http.Header),
	}
}

func (m *mockResponseWriter) Header() http.Header {
	return m.header
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	return m.body.Write(b)
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.status = statusCode
}

// TestUpgrade_Success tests successful SSE upgrade.
func TestUpgrade_Success(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	// Verify headers
	if got := w.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", got, "text/event-stream")
	}
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
	}
	if got := w.Header().Get("Connection"); got != "keep-alive" {
		t.Errorf("Connection = %q, want %q", got, "keep-alive")
	}
	if got := w.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q, want %q", got, "no")
	}

	// Verify initial connection comment
	body := w.Body.String()
	if !strings.HasPrefix(body, ": connected\n\n") {
		t.Errorf("body doesn't start with connection comment, got: %q", body)
	}

	// Verify connection is not nil
	if conn == nil {
		t.Error("expected non-nil connection")
	}
}

// TestUpgrade_NoFlusher tests upgrade failure when ResponseWriter doesn't support flushing.
func TestUpgrade_NoFlusher(t *testing.T) {
	w := newMockResponseWriter()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if !errors.Is(err, ErrNoFlusher) {
		t.Errorf("expected ErrNoFlusher, got: %v", err)
	}
	if conn != nil {
		t.Error("expected nil connection on error")
	}
}

// TestUpgradeWithContext_CustomContext tests upgrade with custom context.
func TestUpgradeWithContext_CustomContext(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := UpgradeWithContext(ctx, w, r)
	if err != nil {
		t.Fatalf("UpgradeWithContext failed: %v", err)
	}
	defer conn.Close()

	if conn == nil {
		t.Error("expected non-nil connection")
	}
}

// TestConn_Send tests sending an event.
func TestConn_Send(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	event := NewEvent("test data").WithType("message").WithID("123")
	err = conn.Send(event)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: message\n") {
		t.Error("body missing event type")
	}
	if !strings.Contains(body, "id: 123\n") {
		t.Error("body missing id")
	}
	if !strings.Contains(body, "data: test data\n") {
		t.Error("body missing data")
	}
}

// TestConn_Send_Closed tests sending on a closed connection.
func TestConn_Send_Closed(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// Close connection
	conn.Close()

	// Attempt to send
	event := NewEvent("data")
	err = conn.Send(event)
	if !errors.Is(err, ErrConnectionClosed) {
		t.Errorf("expected ErrConnectionClosed, got: %v", err)
	}
}

// TestConn_SendData tests sending data-only event.
func TestConn_SendData(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	err = conn.SendData("simple message")
	if err != nil {
		t.Errorf("SendData failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "data: simple message\n") {
		t.Error("body missing data")
	}
}

// TestConn_SendData_Closed tests SendData on closed connection.
func TestConn_SendData_Closed(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	conn.Close()

	err = conn.SendData("data")
	if !errors.Is(err, ErrConnectionClosed) {
		t.Errorf("expected ErrConnectionClosed, got: %v", err)
	}
}

// TestConn_SendJSON tests sending JSON event.
func TestConn_SendJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	data := map[string]any{
		"user":   "Alice",
		"action": "login",
		"count":  42,
	}

	err = conn.SendJSON(data)
	if err != nil {
		t.Errorf("SendJSON failed: %v", err)
	}

	body := w.Body.String()
	// JSON field order may vary, so check for individual fields
	if !strings.Contains(body, `"user":"Alice"`) {
		t.Error("body missing user field")
	}
	if !strings.Contains(body, `"action":"login"`) {
		t.Error("body missing action field")
	}
	if !strings.Contains(body, `"count":42`) {
		t.Error("body missing count field")
	}
}

// TestConn_SendJSON_MarshalError tests SendJSON with unmarshalable data.
func TestConn_SendJSON_MarshalError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	// Channels cannot be marshaled to JSON
	invalidData := make(chan int)

	err = conn.SendJSON(invalidData)
	if err == nil {
		t.Error("expected error for unmarshalable data")
	}
	if !strings.Contains(err.Error(), "marshal") {
		t.Errorf("expected marshal error, got: %v", err)
	}
}

// TestConn_SendJSON_Closed tests SendJSON on closed connection.
func TestConn_SendJSON_Closed(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	conn.Close()

	err = conn.SendJSON(map[string]string{"test": "data"})
	if !errors.Is(err, ErrConnectionClosed) {
		t.Errorf("expected ErrConnectionClosed, got: %v", err)
	}
}

// TestConn_Close tests closing a connection.
func TestConn_Close(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// First close should succeed
	err = conn.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Second close should be no-op
	err = conn.Close()
	if err != nil {
		t.Errorf("second Close failed: %v", err)
	}
}

// TestConn_Close_MultipleCalls tests that Close is idempotent.
func TestConn_Close_MultipleCalls(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// Close multiple times
	for i := 0; i < 5; i++ {
		err = conn.Close()
		if err != nil {
			t.Errorf("Close call %d failed: %v", i+1, err)
		}
	}
}

// TestConn_Done tests Done channel notification.
func TestConn_Done(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// Start goroutine to close after delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		conn.Close()
	}()

	// Wait for Done signal
	select {
	case <-conn.Done():
		// Expected
	case <-time.After(200 * time.Millisecond):
		t.Error("Done channel not closed")
	}
}

// TestConn_ContextCancellation tests that context cancellation closes connection.
func TestConn_ContextCancellation(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	ctx, cancel := context.WithCancel(context.Background())

	conn, err := UpgradeWithContext(ctx, w, r)
	if err != nil {
		t.Fatalf("UpgradeWithContext failed: %v", err)
	}
	defer conn.Close()

	// Cancel context
	cancel()

	// Wait for connection to close
	select {
	case <-conn.Done():
		// Expected
	case <-time.After(200 * time.Millisecond):
		t.Error("connection not closed after context cancellation")
	}

	// Verify sends fail
	err = conn.SendData("test")
	if !errors.Is(err, ErrConnectionClosed) {
		t.Errorf("expected ErrConnectionClosed after context cancel, got: %v", err)
	}
}

// TestConn_ContextTimeout tests that context timeout closes connection.
func TestConn_ContextTimeout(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	conn, err := UpgradeWithContext(ctx, w, r)
	if err != nil {
		t.Fatalf("UpgradeWithContext failed: %v", err)
	}
	defer conn.Close()

	// Wait for timeout
	select {
	case <-conn.Done():
		// Expected
	case <-time.After(200 * time.Millisecond):
		t.Error("connection not closed after timeout")
	}
}

// TestConn_MultipleEvents tests sending multiple events.
func TestConn_MultipleEvents(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	// Send multiple events
	for i := 0; i < 5; i++ {
		event := NewEvent("message").WithID(string(rune('0' + i)))
		err := conn.Send(event)
		if err != nil {
			t.Errorf("Send %d failed: %v", i, err)
		}
	}

	body := w.Body.String()
	// Count occurrences of "data: message"
	count := strings.Count(body, "data: message\n")
	if count != 5 {
		t.Errorf("expected 5 events, found %d", count)
	}
}

// TestConn_EmptyEvent tests sending empty data.
func TestConn_EmptyEvent(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	err = conn.SendData("")
	if err != nil {
		t.Errorf("SendData with empty string failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "data: \n") {
		t.Error("body missing empty data line")
	}
}

// TestConn_LargeData tests sending large data.
func TestConn_LargeData(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	// Create large data (10KB)
	largeData := strings.Repeat("x", 10*1024)

	err = conn.SendData(largeData)
	if err != nil {
		t.Errorf("SendData with large data failed: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, largeData) {
		t.Error("body missing large data")
	}
}

// TestConn_ThreadSafety tests concurrent Send operations.
func TestConn_ThreadSafety(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	// Send events concurrently
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			event := NewEvent("concurrent").WithID(string(rune('0' + n)))
			conn.Send(event)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// BenchmarkConn_Send benchmarks sending events.
func BenchmarkConn_Send(b *testing.B) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		b.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	event := NewEvent("benchmark data").WithType("test").WithID("123")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conn.Send(event)
	}
}

// BenchmarkConn_SendData benchmarks sending simple data.
func BenchmarkConn_SendData(b *testing.B) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		b.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conn.SendData("benchmark data")
	}
}

// BenchmarkConn_SendJSON benchmarks sending JSON events.
func BenchmarkConn_SendJSON(b *testing.B) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)

	conn, err := Upgrade(w, r)
	if err != nil {
		b.Fatalf("Upgrade failed: %v", err)
	}
	defer conn.Close()

	data := map[string]any{
		"user":   "Alice",
		"action": "login",
		"count":  42,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conn.SendJSON(data)
	}
}

// BenchmarkUpgrade benchmarks connection upgrade.
func BenchmarkUpgrade(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/events", http.NoBody)

		conn, err := Upgrade(w, r)
		if err != nil {
			b.Fatalf("Upgrade failed: %v", err)
		}
		conn.Close()
	}
}
