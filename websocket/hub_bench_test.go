package websocket

import (
	"bufio"
	"io"
	"runtime"
	"testing"
)

// BenchmarkHub_Broadcast_10Clients benchmarks broadcasting to 10 clients.
func BenchmarkHub_Broadcast_10Clients(b *testing.B) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Create 10 mock clients
	const numClients = 10
	for i := 0; i < numClients; i++ {
		client := mockConnForHub(b)
		hub.Register(client)
	}

	// Wait for registration
	for hub.ClientCount() != numClients {
		runtime.Gosched() // Yield CPU while waiting
	}

	message := []byte("Benchmark message")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hub.Broadcast(message)
	}
}

// BenchmarkHub_Broadcast_100Clients benchmarks broadcasting to 100 clients.
func BenchmarkHub_Broadcast_100Clients(b *testing.B) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Create 100 mock clients
	const numClients = 100
	for i := 0; i < numClients; i++ {
		client := mockConnForHub(b)
		hub.Register(client)
	}

	// Wait for registration
	for hub.ClientCount() != numClients {
		runtime.Gosched() // Yield CPU while waiting
	}

	message := []byte("Benchmark message")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hub.Broadcast(message)
	}
}

// BenchmarkHub_Register benchmarks client registration.
func BenchmarkHub_Register(b *testing.B) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Pre-create clients
	clients := make([]*Conn, b.N)
	for i := 0; i < b.N; i++ {
		clients[i] = mockConnForHub(b)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hub.Register(clients[i])
	}
}

// BenchmarkHub_Unregister benchmarks client unregistration.
func BenchmarkHub_Unregister(b *testing.B) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Pre-create and register clients
	clients := make([]*Conn, b.N)
	for i := 0; i < b.N; i++ {
		clients[i] = mockConnForHub(b)
		hub.Register(clients[i])
	}

	// Wait for all registrations
	for hub.ClientCount() != b.N {
		runtime.Gosched() // Yield CPU while waiting
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hub.Unregister(clients[i])
	}
}

// mockConnForHub creates a basic mock Conn for benchmarking.
//
// Uses testing.B instead of testing.T for benchmark compatibility.
func mockConnForHub(b testing.TB) *Conn {
	b.Helper()

	// Create bufio.Writer that writes to discard (we don't need to verify writes)
	writer := bufio.NewWriter(io.Discard)

	conn := &Conn{
		conn:     nil,
		reader:   nil,
		writer:   writer,
		isServer: true,
	}

	return conn
}

// BenchmarkE2E_WebSocket_Roundtrip benchmarks end-to-end WebSocket roundtrip latency.
func BenchmarkE2E_WebSocket_Roundtrip(b *testing.B) {
	// Setup echo server
	server := newTestServer(b, func(w *Conn) {
		for {
			msgType, data, err := w.Read()
			if err != nil {
				break
			}
			if err := w.Write(msgType, data); err != nil {
				break
			}
		}
	})
	defer server.Close()

	// Connect client
	conn := dialTestServer(b, server)
	defer conn.Close()

	testMsg := []byte("benchmark message")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Send message
		if err := conn.Write(TextMessage, testMsg); err != nil {
			b.Fatalf("Write error: %v", err)
		}

		// Receive echo
		_, _, err := conn.Read()
		if err != nil {
			b.Fatalf("Read error: %v", err)
		}
	}
}

// BenchmarkE2E_Hub_BroadcastLatency benchmarks end-to-end Hub broadcast latency.
func BenchmarkE2E_Hub_BroadcastLatency(b *testing.B) {
	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Setup server
	server := newTestServer(b, func(w *Conn) {
		hub.Register(w)
		defer hub.Unregister(w)

		// Keep connection alive
		for {
			if _, _, err := w.Read(); err != nil {
				break
			}
		}
	})
	defer server.Close()

	// Connect 10 clients
	const numClients = 10
	clients := make([]*Conn, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = dialTestServer(b, server)
	}

	// Cleanup
	b.Cleanup(func() {
		for _, conn := range clients {
			if conn != nil {
				_ = conn.Close()
			}
		}
	})

	// Wait for registration
	for hub.ClientCount() != numClients {
		runtime.Gosched()
	}

	testMsg := []byte("broadcast message")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Broadcast message
		hub.Broadcast(testMsg)

		// Wait for all clients to receive (simplified - just read from one)
		if _, _, err := clients[0].Read(); err != nil {
			b.Fatalf("Read error: %v", err)
		}
	}
}

// BenchmarkE2E_LargeMessage benchmarks large message transfer.
func BenchmarkE2E_LargeMessage(b *testing.B) {
	// Setup echo server
	server := newTestServer(b, func(w *Conn) {
		for {
			msgType, data, err := w.Read()
			if err != nil {
				break
			}
			if err := w.Write(msgType, data); err != nil {
				break
			}
		}
	})
	defer server.Close()

	// Connect client
	conn := dialTestServer(b, server)
	defer conn.Close()

	// 1MB message
	largeMsg := make([]byte, 1024*1024)
	for i := range largeMsg {
		largeMsg[i] = byte(i % 256)
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(largeMsg)))

	for i := 0; i < b.N; i++ {
		// Send large message
		if err := conn.Write(BinaryMessage, largeMsg); err != nil {
			b.Fatalf("Write error: %v", err)
		}

		// Receive echo
		_, _, err := conn.Read()
		if err != nil {
			b.Fatalf("Read error: %v", err)
		}
	}
}

// BenchmarkE2E_ParallelClients benchmarks multiple clients in parallel.
func BenchmarkE2E_ParallelClients(b *testing.B) {
	// Setup echo server
	server := newTestServer(b, func(w *Conn) {
		for {
			msgType, data, err := w.Read()
			if err != nil {
				break
			}
			if err := w.Write(msgType, data); err != nil {
				break
			}
		}
	})
	defer server.Close()

	testMsg := []byte("parallel message")

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine gets its own connection
		conn := dialTestServer(b, server)
		defer conn.Close()

		for pb.Next() {
			// Send and receive
			if err := conn.Write(TextMessage, testMsg); err != nil {
				b.Errorf("Write error: %v", err)
				return
			}
			if _, _, err := conn.Read(); err != nil {
				b.Errorf("Read error: %v", err)
				return
			}
		}
	})
}
