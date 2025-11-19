package sse

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// createHubTestConn creates a test SSE connection for hub tests.
func createHubTestConn(tb testing.TB) *Conn {
	tb.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)
	conn, err := Upgrade(w, r)
	if err != nil {
		tb.Fatalf("failed to create test connection: %v", err)
	}
	return conn
}

func TestNewHub(t *testing.T) {
	hub := NewHub[string]()

	if hub == nil {
		t.Fatal("NewHub returned nil")
	}

	if hub.clients == nil {
		t.Error("clients map not initialized")
	}

	if hub.broadcast == nil {
		t.Error("broadcast channel not initialized")
	}

	if hub.register == nil {
		t.Error("register channel not initialized")
	}

	if hub.unregister == nil {
		t.Error("unregister channel not initialized")
	}

	if hub.done == nil {
		t.Error("done channel not initialized")
	}

	if hub.closed {
		t.Error("hub should not be closed initially")
	}

	if hub.Clients() != 0 {
		t.Errorf("Clients() = %d, want 0", hub.Clients())
	}
}

func TestHub_RegisterUnregister(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	conn := createHubTestConn(t)

	// Register client
	err := hub.Register(conn)
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	// Wait for registration to process
	time.Sleep(10 * time.Millisecond)

	if got := hub.Clients(); got != 1 {
		t.Errorf("Clients() = %d, want 1", got)
	}

	// Unregister client
	err = hub.Unregister(conn)
	if err != nil {
		t.Fatalf("Unregister() error = %v, want nil", err)
	}

	// Wait for unregistration to process
	time.Sleep(10 * time.Millisecond)

	if got := hub.Clients(); got != 0 {
		t.Errorf("Clients() = %d, want 0", got)
	}
}

func TestHub_RegisterMultipleClients(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	// Register multiple clients
	conns := make([]*Conn, 5)
	for i := 0; i < 5; i++ {
		conns[i] = createHubTestConn(t)
		err := hub.Register(conns[i])
		if err != nil {
			t.Fatalf("Register() error = %v, want nil", err)
		}
	}

	// Wait for registration to process
	time.Sleep(20 * time.Millisecond)

	if got := hub.Clients(); got != 5 {
		t.Errorf("Clients() = %d, want 5", got)
	}

	// Unregister all
	for _, conn := range conns {
		err := hub.Unregister(conn)
		if err != nil {
			t.Fatalf("Unregister() error = %v, want nil", err)
		}
	}

	// Wait for unregistration to process
	time.Sleep(20 * time.Millisecond)

	if got := hub.Clients(); got != 0 {
		t.Errorf("Clients() = %d, want 0", got)
	}
}

func TestHub_BroadcastString(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)
	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}

	err = hub.Register(conn)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Wait for registration
	time.Sleep(10 * time.Millisecond)

	// Broadcast message
	err = hub.Broadcast("Hello, World!")
	if err != nil {
		t.Fatalf("Broadcast() error = %v, want nil", err)
	}

	// Wait for broadcast to process
	time.Sleep(50 * time.Millisecond)

	// Stop Hub before reading (prevents race)
	_ = hub.Close()

	// Check response (basic check - detailed in conn_test.go)
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty response body")
	}
}

func TestHub_BroadcastToMultipleClients(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	// Create multiple clients
	numClients := 10
	writers := make([]*httptest.ResponseRecorder, numClients)
	conns := make([]*Conn, numClients)

	for i := 0; i < numClients; i++ {
		w := httptest.NewRecorder()
		writers[i] = w
		r := httptest.NewRequest("GET", "/events", http.NoBody)
		conn, err := Upgrade(w, r)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		conns[i] = conn
		err = hub.Register(conn)
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	// Wait for all registrations
	time.Sleep(50 * time.Millisecond)

	if got := hub.Clients(); got != numClients {
		t.Errorf("Clients() = %d, want %d", got, numClients)
	}

	// Broadcast message
	testMsg := "broadcast-test"
	err := hub.Broadcast(testMsg)
	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}

	// Wait for broadcast to process
	time.Sleep(100 * time.Millisecond)

	// Stop Hub before reading (prevents race)
	_ = hub.Close()

	// Verify all clients received the message
	for i, w := range writers {
		body := w.Body.String()
		if body == "" {
			t.Errorf("client %d: expected non-empty response body", i)
		}
	}
}

func TestHub_BroadcastJSON(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)
	conn, err := Upgrade(w, r)
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}

	err = hub.Register(conn)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Wait for registration
	time.Sleep(10 * time.Millisecond)

	// Broadcast JSON
	data := map[string]string{"message": "hello", "status": "ok"}
	err = hub.BroadcastJSON(data)
	if err != nil {
		t.Fatalf("BroadcastJSON() error = %v, want nil", err)
	}

	// Wait for broadcast to process
	time.Sleep(50 * time.Millisecond)

	// Stop Hub before reading (prevents race)
	_ = hub.Close()

	// Check response
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty response body")
	}
}

func TestHub_BroadcastClosed(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()

	// Close hub
	err := hub.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Try to broadcast
	err = hub.Broadcast("test")
	if !errors.Is(err, ErrHubClosed) {
		t.Errorf("Broadcast() error = %v, want ErrHubClosed", err)
	}
}

func TestHub_RegisterClosed(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()

	// Close hub
	err := hub.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Try to register
	conn := createHubTestConn(t)
	err = hub.Register(conn)
	if !errors.Is(err, ErrHubClosed) {
		t.Errorf("Register() error = %v, want ErrHubClosed", err)
	}
}

func TestHub_UnregisterClosed(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()

	conn := createHubTestConn(t)

	// Close hub
	err := hub.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Try to unregister
	err = hub.Unregister(conn)
	if !errors.Is(err, ErrHubClosed) {
		t.Errorf("Unregister() error = %v, want ErrHubClosed", err)
	}
}

func TestHub_CloseMultipleTimes(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()

	// Close multiple times
	for i := 0; i < 3; i++ {
		err := hub.Close()
		if err != nil {
			t.Errorf("Close() #%d error = %v, want nil", i, err)
		}
	}

	if got := hub.Clients(); got != 0 {
		t.Errorf("Clients() = %d, want 0", got)
	}
}

func TestHub_CloseClosesAllClients(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()

	// Register multiple clients
	numClients := 5
	conns := make([]*Conn, numClients)
	for i := 0; i < numClients; i++ {
		conns[i] = createHubTestConn(t)
		err := hub.Register(conns[i])
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	// Wait for registration
	time.Sleep(20 * time.Millisecond)

	if got := hub.Clients(); got != numClients {
		t.Errorf("Clients() = %d, want %d", got, numClients)
	}

	// Close hub
	err := hub.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify all clients are closed (check Done() channel)
	for i, conn := range conns {
		select {
		case <-conn.Done():
			// Client is closed as expected
		case <-time.After(100 * time.Millisecond):
			t.Errorf("client %d: expected Done() to be closed", i)
		}
	}

	if got := hub.Clients(); got != 0 {
		t.Errorf("Clients() = %d, want 0", got)
	}
}

func TestHub_UnregisterNonExistentClient(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	// Create client but don't register it
	conn := createHubTestConn(t)

	// Unregister non-existent client (should not panic)
	err := hub.Unregister(conn)
	if err != nil {
		t.Fatalf("Unregister() error = %v, want nil", err)
	}

	// Wait for unregistration to process
	time.Sleep(10 * time.Millisecond)

	if got := hub.Clients(); got != 0 {
		t.Errorf("Clients() = %d, want 0", got)
	}
}

func TestHub_ConcurrentOperations(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	var wg sync.WaitGroup
	numGoroutines := 10
	operationsPerGoroutine := 10

	// Concurrent registrations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				conn := createHubTestConn(t)
				_ = hub.Register(conn)
			}
		}()
	}
	wg.Wait()

	// Wait for registrations to process
	time.Sleep(100 * time.Millisecond)

	expectedClients := numGoroutines * operationsPerGoroutine
	if got := hub.Clients(); got != expectedClients {
		t.Errorf("Clients() = %d, want %d", got, expectedClients)
	}

	// Concurrent broadcasts
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(_ int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				_ = hub.Broadcast("concurrent-test")
			}
		}(i)
	}
	wg.Wait()

	// Concurrent Clients() calls
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				_ = hub.Clients()
			}
		}()
	}
	wg.Wait()
}

func TestHub_LargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-scale test in short mode")
	}

	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	// Register 100 clients
	numClients := 100
	conns := make([]*Conn, numClients)
	for i := 0; i < numClients; i++ {
		conns[i] = createHubTestConn(t)
		err := hub.Register(conns[i])
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	// Wait for all registrations
	time.Sleep(200 * time.Millisecond)

	if got := hub.Clients(); got != numClients {
		t.Errorf("Clients() = %d, want %d", got, numClients)
	}

	// Broadcast to all
	err := hub.Broadcast("large-scale-test")
	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}

	// Wait for broadcast
	time.Sleep(200 * time.Millisecond)

	// Cleanup
	for _, conn := range conns {
		_ = hub.Unregister(conn)
	}

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	if got := hub.Clients(); got != 0 {
		t.Errorf("Clients() = %d, want 0 after cleanup", got)
	}
}

func TestHub_AutoRemoveFailedClients(t *testing.T) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	// Create client with canceled context (will fail on send)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)
	conn, err := UpgradeWithContext(ctx, w, r)
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}

	err = hub.Register(conn)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Wait for registration
	time.Sleep(10 * time.Millisecond)

	if got := hub.Clients(); got != 1 {
		t.Errorf("Clients() = %d, want 1", got)
	}

	// Broadcast will fail and auto-remove the client
	err = hub.Broadcast("test")
	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}

	// Wait for broadcast and auto-removal
	time.Sleep(50 * time.Millisecond)

	// Client should be auto-removed
	if got := hub.Clients(); got != 0 {
		t.Errorf("Clients() = %d, want 0 (auto-removed)", got)
	}
}

func TestHub_GenericTypes(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		hub := NewHub[int]()
		go hub.Run()
		defer func() { _ = hub.Close() }()

		conn := createHubTestConn(t)
		_ = hub.Register(conn)
		time.Sleep(10 * time.Millisecond)

		err := hub.Broadcast(42)
		if err != nil {
			t.Errorf("Broadcast() error = %v", err)
		}
	})

	t.Run("struct", func(t *testing.T) {
		type TestStruct struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}

		hub := NewHub[TestStruct]()
		go hub.Run()
		defer func() { _ = hub.Close() }()

		conn := createHubTestConn(t)
		_ = hub.Register(conn)
		time.Sleep(10 * time.Millisecond)

		err := hub.Broadcast(TestStruct{ID: 1, Name: "test"})
		if err != nil {
			t.Errorf("Broadcast() error = %v", err)
		}
	})
}

// Benchmarks

func BenchmarkHub_Broadcast(b *testing.B) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	// Register one client
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/events", http.NoBody)
	conn, err := Upgrade(w, r)
	if err != nil {
		b.Fatalf("Upgrade() error = %v", err)
	}
	_ = hub.Register(conn)
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = hub.Broadcast("benchmark-test")
	}
}

func BenchmarkHub_Register(b *testing.B) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	conns := make([]*Conn, b.N)
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/events", http.NoBody)
		conn, err := Upgrade(w, r)
		if err != nil {
			b.Fatalf("Upgrade() error = %v", err)
		}
		conns[i] = conn
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = hub.Register(conns[i])
	}
}

func BenchmarkHub_Clients(b *testing.B) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	// Register 100 clients
	for i := 0; i < 100; i++ {
		conn := createHubTestConn(b)
		_ = hub.Register(conn)
	}
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = hub.Clients()
	}
}

func BenchmarkHub_10Clients(b *testing.B) {
	benchmarkHubNClients(b, 10)
}

func BenchmarkHub_100Clients(b *testing.B) {
	benchmarkHubNClients(b, 100)
}

func BenchmarkHub_1000Clients(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping 1000 clients benchmark in short mode")
	}
	benchmarkHubNClients(b, 1000)
}

func benchmarkHubNClients(b *testing.B, numClients int) {
	hub := NewHub[string]()
	go hub.Run()
	defer func() { _ = hub.Close() }()

	// Register N clients
	for i := 0; i < numClients; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/events", http.NoBody)
		conn, err := Upgrade(w, r)
		if err != nil {
			b.Fatalf("Upgrade() error = %v", err)
		}
		_ = hub.Register(conn)
	}
	time.Sleep(time.Duration(numClients) * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = hub.Broadcast("benchmark-test")
	}
}
