package main

import (
	"log"
	"net/http"

	"github.com/coregx/stream/websocket"
)

// handleWebSocket upgrades HTTP connection and echoes messages back to client.
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP to WebSocket
	conn, err := websocket.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		http.Error(w, "WebSocket upgrade failed", http.StatusBadRequest)
		return
	}
	defer conn.Close()

	log.Printf("Client connected from %s", r.RemoteAddr)

	// Echo loop: read message and write it back
	for {
		// Read next message (text or binary)
		msgType, data, err := conn.Read()
		if err != nil {
			if websocket.IsCloseError(err) {
				log.Printf("Client disconnected: %v", err)
			} else {
				log.Printf("Read error: %v", err)
			}
			break
		}

		log.Printf("Received %s message: %s", msgType, string(data))

		// Echo message back to client
		if err := conn.Write(msgType, data); err != nil {
			log.Printf("Write error: %v", err)
			break
		}
	}
}

func main() {
	http.HandleFunc("/ws", handleWebSocket)

	addr := ":8080"
	log.Printf("WebSocket echo server listening on %s", addr)
	log.Printf("Connect with: wscat -c ws://localhost:8080/ws")
	log.Fatal(http.ListenAndServe(addr, nil))
}
