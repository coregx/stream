// Package main demonstrates SSE chat room using Hub for broadcasting.
//
// This example shows:
//   - Using Hub[T] for broadcasting to multiple clients
//   - Custom message types with JSON
//   - POST endpoint for sending messages
//   - GET endpoint for receiving events
//   - Multiple concurrent clients
//   - Graceful shutdown
//
// Run with: go run main.go
//
// Test with:
//
//	Terminal 1: curl http://localhost:8080/events
//	Terminal 2: curl http://localhost:8080/events
//	Terminal 3: curl -X POST -d '{"user":"Alice","text":"Hello!"}' http://localhost:8080/messages
package main

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coregx/stream/sse"
)

// Message represents a chat message.
type Message struct {
	User      string    `json:"user"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// String implements fmt.Stringer for Message.
// This allows Hub[Message] to broadcast messages as strings.
func (m Message) String() string {
	data, _ := json.Marshal(m)
	return string(data)
}

// ChatServer manages the chat room using SSE Hub.
type ChatServer struct {
	hub    *sse.Hub[Message]
	server *http.Server
}

// NewChatServer creates a new chat server.
func NewChatServer(addr string) *ChatServer {
	hub := sse.NewHub[Message]()

	mux := http.NewServeMux()
	cs := &ChatServer{
		hub: hub,
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}

	// Register routes
	mux.HandleFunc("/events", cs.handleEvents)
	mux.HandleFunc("/messages", cs.handleMessages)
	mux.HandleFunc("/", cs.handleIndex)

	return cs
}

// Start starts the chat server.
func (cs *ChatServer) Start() error {
	// Start hub
	go cs.hub.Run()

	// Start server
	slog.Info("Starting SSE chat server", "addr", cs.server.Addr)
	slog.Info("Connect with: curl http://localhost:8080/events")
	slog.Info("Send messages: curl -X POST -d '{\"user\":\"Alice\",\"text\":\"Hello!\"}' http://localhost:8080/messages")
	slog.Info("Or visit: http://localhost:8080/ in your browser")

	return cs.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (cs *ChatServer) Shutdown(ctx context.Context) error {
	// Close hub first (disconnects clients)
	if err := cs.hub.Close(); err != nil {
		slog.Error("Failed to close hub", "error", err)
	}

	// Shutdown HTTP server
	return cs.server.Shutdown(ctx)
}

// handleIndex serves a simple HTML page for browser testing.
func (cs *ChatServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <title>SSE Chat Demo</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        #messages { border: 1px solid #ccc; height: 400px; overflow-y: auto; padding: 10px; margin: 20px 0; }
        .message { margin: 10px 0; padding: 10px; background: #f5f5f5; border-radius: 5px; }
        .user { font-weight: bold; color: #0066cc; }
        .time { color: #666; font-size: 0.9em; }
        #form { display: flex; gap: 10px; }
        input { flex: 1; padding: 10px; }
        button { padding: 10px 20px; background: #0066cc; color: white; border: none; cursor: pointer; }
        button:hover { background: #0052a3; }
        #status { color: #666; margin: 10px 0; }
    </style>
</head>
<body>
    <h1>SSE Chat Room</h1>
    <div id="status">Connecting...</div>
    <div id="messages"></div>
    <div id="form">
        <input type="text" id="user" placeholder="Your name" value="Anonymous">
        <input type="text" id="message" placeholder="Type a message...">
        <button onclick="sendMessage()">Send</button>
    </div>

    <script>
        const messagesDiv = document.getElementById('messages');
        const statusDiv = document.getElementById('status');
        const userInput = document.getElementById('user');
        const messageInput = document.getElementById('message');

        // Connect to SSE
        const eventSource = new EventSource('/events');

        eventSource.onopen = () => {
            statusDiv.textContent = 'Connected! ' + new Date().toLocaleTimeString();
            statusDiv.style.color = 'green';
        };

        eventSource.onmessage = (e) => {
            const msg = JSON.parse(e.data);
            const div = document.createElement('div');
            div.className = 'message';
            div.innerHTML = '<span class="user">' + msg.user + '</span>: ' + msg.text +
                          ' <span class="time">(' + new Date(msg.timestamp).toLocaleTimeString() + ')</span>';
            messagesDiv.appendChild(div);
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        };

        eventSource.onerror = () => {
            statusDiv.textContent = 'Disconnected';
            statusDiv.style.color = 'red';
        };

        function sendMessage() {
            const user = userInput.value || 'Anonymous';
            const text = messageInput.value;
            if (!text) return;

            fetch('/messages', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ user: user, text: text })
            });

            messageInput.value = '';
        }

        messageInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') sendMessage();
        });
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}

// handleEvents handles SSE connections for receiving messages.
func (cs *ChatServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Upgrade to SSE
	conn, err := sse.Upgrade(w, r)
	if err != nil {
		slog.Error("Failed to upgrade to SSE", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Register with hub
	if err := cs.hub.Register(conn); err != nil {
		slog.Error("Failed to register connection", "error", err)
		return
	}
	defer cs.hub.Unregister(conn)

	clientCount := cs.hub.Clients()
	slog.Info("Client connected", "remote", r.RemoteAddr, "total", clientCount)

	// Send welcome message
	welcome := Message{
		User:      "System",
		Text:      fmt.Sprintf("Welcome! %d clients connected.", clientCount),
		Timestamp: time.Now(),
	}
	conn.SendJSON(welcome)

	// Wait for disconnection
	<-conn.Done()

	clientCount = cs.hub.Clients()
	slog.Info("Client disconnected", "remote", r.RemoteAddr, "remaining", clientCount)
}

// handleMessages handles POST requests to send messages to all clients.
func (cs *ChatServer) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Parse message
	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Add timestamp
	msg.Timestamp = time.Now()

	// Validate
	if msg.User == "" {
		msg.User = "Anonymous"
	}
	if msg.Text == "" {
		http.Error(w, "Text is required", http.StatusBadRequest)
		return
	}

	// Broadcast to all clients
	if err := cs.hub.Broadcast(msg); err != nil {
		slog.Error("Failed to broadcast message", "error", err)
		http.Error(w, "Failed to broadcast", http.StatusInternalServerError)
		return
	}

	slog.Info("Message broadcasted", "user", msg.User, "text", msg.Text, "clients", cs.hub.Clients())

	// Send success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"status":"ok","clients":%d}`, cs.hub.Clients())
}

func main() {
	// Create chat server
	server := NewChatServer(":8080")

	// Start server in background
	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	slog.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("Server stopped")
}
