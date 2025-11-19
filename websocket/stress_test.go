package websocket

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestStress_LargeMessages tests handling of large messages (fragmented).
func TestStress_LargeMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// Setup echo server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		// Echo server
		for {
			msgType, data, err := conn.Read()
			if err != nil {
				break
			}
			if err := conn.Write(msgType, data); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	conn, resp, err := Dial(context.Background(), wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer conn.Close()

	// Test different large message sizes
	testCases := []struct {
		name string
		size int
	}{
		{"64KB", 64 * 1024},
		{"256KB", 256 * 1024},
		{"1MB", 1024 * 1024},
		{"5MB", 5 * 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate random data
			largeData := make([]byte, tc.size)
			if _, err := rand.Read(largeData); err != nil {
				t.Fatalf("Failed to generate random data: %v", err)
			}

			startTime := time.Now()

			// Send large message
			if err := conn.Write(BinaryMessage, largeData); err != nil {
				t.Fatalf("Write error: %v", err)
			}

			// Receive echo
			_, receivedData, err := conn.Read()
			if err != nil {
				t.Fatalf("Read error: %v", err)
			}

			duration := time.Since(startTime)

			// Verify data integrity
			if !bytes.Equal(largeData, receivedData) {
				t.Errorf("Data mismatch: sent %d bytes, received %d bytes", len(largeData), len(receivedData))
			}

			throughput := float64(tc.size) / duration.Seconds() / (1024 * 1024)
			t.Logf("%s: duration=%v, throughput=%.2f MB/s", tc.name, duration, throughput)
		})
	}
}

// TestStress_RapidConnectDisconnect tests rapid connection cycling.
func TestStress_RapidConnectDisconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Setup server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		hub.Register(conn)
		defer hub.Unregister(conn)

		// Keep connection alive briefly
		for {
			if _, _, err := conn.Read(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	const (
		numClients = 50
		iterations = 10
		totalConns = numClients * iterations
	)

	var (
		successfulConns atomic.Int32
		errors          atomic.Int32
		wg              sync.WaitGroup
	)

	startTime := time.Now()
	startGoroutines := runtime.NumGoroutine()

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			wsURL := "ws" + server.URL[4:]

			// Connect and disconnect multiple times
			for j := 0; j < iterations; j++ {
				conn, resp, err := Dial(context.Background(), wsURL, nil)
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close() // Close immediately, not deferred in loop
				}
				if err != nil {
					errors.Add(1)
					t.Errorf("Client %d iteration %d: dial error: %v", clientID, j, err)
					continue
				}

				// Send one message
				msg := []byte(fmt.Sprintf("client-%d-iter-%d", clientID, j))
				if err := conn.Write(TextMessage, msg); err != nil {
					errors.Add(1)
					t.Errorf("Client %d iteration %d: write error: %v", clientID, j, err)
					conn.Close()
					continue
				}

				successfulConns.Add(1)

				// Immediately close
				conn.Close()

				// Small delay between iterations
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Wait for cleanup
	time.Sleep(500 * time.Millisecond)

	endGoroutines := runtime.NumGoroutine()
	goroutineLeak := endGoroutines - startGoroutines

	successful := successfulConns.Load()
	errorCount := errors.Load()
	clientCount := hub.ClientCount()

	t.Logf("Rapid connect/disconnect completed in %v", duration)
	t.Logf("Total connections: %d", totalConns)
	t.Logf("Successful: %d, Errors: %d", successful, errorCount)
	t.Logf("Connection rate: %.0f conn/s", float64(successful)/duration.Seconds())
	t.Logf("Goroutines: start=%d, end=%d, leak=%d", startGoroutines, endGoroutines, goroutineLeak)
	t.Logf("Final Hub ClientCount: %d", clientCount)

	if successful != totalConns {
		t.Errorf("Successful connections = %d, want %d", successful, totalConns)
	}

	if errorCount > 0 {
		t.Errorf("Got %d errors", errorCount)
	}

	// Allow some goroutine overhead (e.g., hub goroutine, test goroutines)
	// but fail if there's significant leak (>20 goroutines)
	if goroutineLeak > 20 {
		t.Errorf("Goroutine leak detected: %d extra goroutines", goroutineLeak)
	}

	// Hub should have no clients after all disconnects
	if clientCount != 0 {
		t.Errorf("Hub still has %d clients, want 0", clientCount)
	}
}

// TestStress_ConcurrentBroadcast tests concurrent broadcasting from multiple goroutines.
func TestStress_ConcurrentBroadcast(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Setup server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		hub.Register(conn)
		defer hub.Unregister(conn)

		for {
			if _, _, err := conn.Read(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	const (
		numClients         = 50
		numBroadcasters    = 10
		msgsPerBroadcaster = 100
		totalBroadcasts    = numBroadcasters * msgsPerBroadcaster
	)

	// Connect clients
	clientReceived := make([]atomic.Int32, numClients)
	var clientWG sync.WaitGroup
	clientWG.Add(numClients)

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer clientWG.Done()

			wsURL := "ws" + server.URL[4:]
			conn, resp, err := Dial(context.Background(), wsURL, nil)
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				t.Errorf("Client %d: dial error: %v", clientID, err)
				return
			}
			defer conn.Close()

			for {
				if _, _, err := conn.Read(); err != nil {
					break
				}
				clientReceived[clientID].Add(1)
			}
		}(i)
	}

	// Wait for clients to connect
	time.Sleep(500 * time.Millisecond)

	connectedClients := hub.ClientCount()
	if connectedClients != numClients {
		t.Errorf("Connected clients = %d, want %d", connectedClients, numClients)
	}

	// Concurrent broadcasters
	var broadcasterWG sync.WaitGroup
	broadcasterWG.Add(numBroadcasters)

	startTime := time.Now()

	for i := 0; i < numBroadcasters; i++ {
		go func(broadcasterID int) {
			defer broadcasterWG.Done()

			for j := 0; j < msgsPerBroadcaster; j++ {
				msg := []byte(fmt.Sprintf("broadcaster-%d-msg-%d", broadcasterID, j))
				hub.Broadcast(msg)
			}
		}(i)
	}

	broadcasterWG.Wait()
	broadcastDuration := time.Since(startTime)

	t.Logf("Broadcast phase completed in %v", broadcastDuration)
	t.Logf("Broadcast throughput: %.0f msg/s", float64(totalBroadcasts)/broadcastDuration.Seconds())

	// Wait for message delivery
	time.Sleep(2 * time.Second)

	// Close hub to terminate clients
	hub.Close()

	// Wait for clients to exit
	done := make(chan struct{})
	go func() {
		clientWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Clients did not exit within timeout")
	}

	totalDuration := time.Since(startTime)
	t.Logf("Total test duration: %v", totalDuration)

	// Verify message delivery
	var totalReceived int32
	minReceived := int32(totalBroadcasts)
	var maxReceived int32

	for i := 0; i < numClients; i++ {
		received := clientReceived[i].Load()
		totalReceived += received

		if received < minReceived {
			minReceived = received
		}
		if received > maxReceived {
			maxReceived = received
		}
	}

	expectedTotal := int32(numClients * totalBroadcasts)
	receivedPercent := float64(totalReceived) / float64(expectedTotal) * 100

	t.Logf("Message delivery:")
	t.Logf("  Total: %d/%d (%.1f%%)", totalReceived, expectedTotal, receivedPercent)
	t.Logf("  Per client: min=%d, max=%d", minReceived, maxReceived)

	// Allow 95% delivery threshold due to concurrent stress
	if receivedPercent < 95.0 {
		t.Errorf("Message delivery rate %.1f%%, want >= 95%%", receivedPercent)
	}

	// Check for message consistency - all clients should receive similar counts
	avgReceived := float64(totalReceived) / float64(numClients)
	for i := 0; i < numClients; i++ {
		received := clientReceived[i].Load()
		deviation := float64(received) / avgReceived
		if deviation < 0.8 || deviation > 1.2 {
			t.Errorf("Client %d: received %d messages (%.1f%% of avg), inconsistent delivery", i, received, deviation*100)
		}
	}
}

// TestStress_MemoryPressure tests behavior under memory pressure with many concurrent operations.
func TestStress_MemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	var memStatsBefore, memStatsAfter runtime.MemStats
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.ReadMemStats(&memStatsBefore)

	hub := NewHub()
	go hub.Run()
	defer hub.Close()

	// Setup server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		hub.Register(conn)
		defer hub.Unregister(conn)

		for {
			msgType, data, err := conn.Read()
			if err != nil {
				break
			}
			// Echo back
			if err := conn.Write(msgType, data); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	const (
		numClients  = 100
		numMessages = 1000
	)

	var wg sync.WaitGroup
	wg.Add(numClients)

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()

			wsURL := "ws" + server.URL[4:]
			conn, resp, err := Dial(context.Background(), wsURL, nil)
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				t.Errorf("Client %d: dial error: %v", clientID, err)
				return
			}
			defer conn.Close()

			// Send many messages rapidly
			for j := 0; j < numMessages; j++ {
				// Create reasonably sized messages (1KB each)
				msg := make([]byte, 1024)
				copy(msg, fmt.Sprintf("client-%d-msg-%d", clientID, j))

				if err := conn.Write(BinaryMessage, msg); err != nil {
					t.Errorf("Client %d: write error: %v", clientID, err)
					return
				}

				if _, _, err := conn.Read(); err != nil {
					t.Errorf("Client %d: read error: %v", clientID, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	runtime.GC()
	time.Sleep(500 * time.Millisecond)
	runtime.ReadMemStats(&memStatsAfter)

	// Memory metrics
	allocIncrease := memStatsAfter.Alloc - memStatsBefore.Alloc
	totalAllocIncrease := memStatsAfter.TotalAlloc - memStatsBefore.TotalAlloc

	t.Logf("Memory metrics:")
	t.Logf("  Alloc: before=%d, after=%d, increase=%d (%.2f MB)",
		memStatsBefore.Alloc, memStatsAfter.Alloc, allocIncrease, float64(allocIncrease)/(1024*1024))
	t.Logf("  TotalAlloc increase: %d (%.2f MB)", totalAllocIncrease, float64(totalAllocIncrease)/(1024*1024))
	t.Logf("  NumGC: %d", memStatsAfter.NumGC-memStatsBefore.NumGC)

	// Check for memory leaks - after GC, increase should be minimal
	// Allow up to 50MB increase for connection overhead
	maxAllowedIncrease := uint64(50 * 1024 * 1024)
	if allocIncrease > maxAllowedIncrease {
		t.Errorf("Memory leak suspected: alloc increased by %.2f MB", float64(allocIncrease)/(1024*1024))
	}
}

// TestStress_PingPongStorm tests handling of many ping/pong control frames.
// NOTE: Skipped - requires SetPongHandler() and WritePing() methods not yet implemented.
func TestStress_PingPongStorm(t *testing.T) {
	t.Skip("Requires SetPongHandler() and WritePing() methods - TODO")
}

// TestStress_ConnectionTimeout tests handling of connection timeouts and deadlines.
// NOTE: Skipped - requires SetReadDeadline() method not yet implemented.
func TestStress_ConnectionTimeout(t *testing.T) {
	t.Skip("Requires SetReadDeadline() method - TODO")
}
