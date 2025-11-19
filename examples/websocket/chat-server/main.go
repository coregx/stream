package main

import (
	"log"
	"net/http"
	"time"

	"github.com/coregx/stream/websocket"
)

// Message represents a chat message in JSON format.
type Message struct {
	Type      string    `json:"type"`      // "join", "message", "leave"
	Username  string    `json:"username"`  // Sender username
	Text      string    `json:"text"`      // Message text
	Timestamp time.Time `json:"timestamp"` // Message timestamp
}

var hub *websocket.Hub

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	conn, err := websocket.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	// Get username from query param (default: "Anonymous")
	username := r.URL.Query().Get("username")
	if username == "" {
		username = "Anonymous"
	}

	log.Printf("User %s connected from %s", username, r.RemoteAddr)

	// Register client with hub
	hub.Register(conn)
	defer func() {
		hub.Unregister(conn)
		log.Printf("User %s disconnected", username)
	}()

	// Broadcast join notification
	joinMsg := Message{
		Type:      "join",
		Username:  username,
		Text:      username + " joined the chat",
		Timestamp: time.Now(),
	}
	if err := hub.BroadcastJSON(joinMsg); err != nil {
		log.Printf("Broadcast error: %v", err)
	}

	// Read and broadcast messages
	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsCloseError(err) {
				// Clean disconnect - broadcast leave notification
				leaveMsg := Message{
					Type:      "leave",
					Username:  username,
					Text:      username + " left the chat",
					Timestamp: time.Now(),
				}
				_ = hub.BroadcastJSON(leaveMsg)
			} else {
				log.Printf("Read error from %s: %v", username, err)
			}
			break
		}

		// Add metadata and broadcast
		msg.Type = "message"
		msg.Username = username
		msg.Timestamp = time.Now()

		log.Printf("Message from %s: %s", username, msg.Text)

		if err := hub.BroadcastJSON(msg); err != nil {
			log.Printf("Broadcast error: %v", err)
		}
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	// Serve embedded HTML client
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeFile(w, r, "index.html")
}

func main() {
	// Create and start hub
	hub = websocket.NewHub()
	go hub.Run()
	defer hub.Close()

	// HTTP handlers
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/ws", handleWebSocket)

	addr := ":8080"
	log.Printf("Chat server listening on %s", addr)
	log.Printf("Open http://localhost:8080 in multiple browser tabs")
	log.Fatal(http.ListenAndServe(addr, nil))
}
