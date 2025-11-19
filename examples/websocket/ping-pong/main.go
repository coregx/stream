package main

import (
	"log"
	"net/http"
	"time"

	"github.com/coregx/stream/websocket"
)

// handleWebSocket demonstrates keep-alive using Ping/Pong mechanism.
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("Client connected from %s", r.RemoteAddr)

	// Start ping goroutine (sends Ping every 30 seconds)
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// Health check ticker (verify connection every 60 seconds)
	healthTicker := time.NewTicker(60 * time.Second)
	defer healthTicker.Stop()

	done := make(chan struct{})

	// Goroutine: Send periodic pings
	go func() {
		for {
			select {
			case <-pingTicker.C:
				log.Printf("Sending ping to %s", r.RemoteAddr)
				if err := conn.Ping([]byte("heartbeat")); err != nil {
					log.Printf("Ping failed: %v", err)
					close(done)
					return
				}

			case <-healthTicker.C:
				log.Printf("Connection healthy: %s", r.RemoteAddr)

			case <-done:
				return
			}
		}
	}()

	// Main goroutine: Read messages (Pong handled automatically)
	for {
		msgType, data, err := conn.Read()
		if err != nil {
			if websocket.IsCloseError(err) {
				log.Printf("Client disconnected cleanly: %v", err)
			} else {
				log.Printf("Read error: %v", err)
			}
			close(done)
			break
		}

		log.Printf("Received %s message: %s", msgType, string(data))

		// Echo back
		if err := conn.Write(msgType, data); err != nil {
			log.Printf("Write error: %v", err)
			close(done)
			break
		}
	}

	log.Printf("Connection closed: %s", r.RemoteAddr)
}

func main() {
	http.HandleFunc("/ws", handleWebSocket)

	addr := ":8080"
	log.Printf("WebSocket ping-pong server listening on %s", addr)
	log.Printf("Features:")
	log.Printf("  - Ping every 30 seconds (keep-alive)")
	log.Printf("  - Health check every 60 seconds")
	log.Printf("  - Automatic Pong response (built-in)")
	log.Printf("")
	log.Printf("Connect with: wscat -c ws://localhost:8080/ws")
	log.Fatal(http.ListenAndServe(addr, nil))
}
