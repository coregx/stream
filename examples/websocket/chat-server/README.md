# WebSocket Chat Server

Multi-client chat application demonstrating Hub-based broadcasting.

## Features

- **Hub broadcasting**: Single message to all connected clients
- **JSON messages**: Type-safe message structure
- **Web client**: Beautiful HTML/CSS/JS interface
- **User management**: Join/leave notifications
- **Real-time**: Instant message delivery to all clients
- **Automatic reconnection**: Graceful disconnect handling

## Run

```bash
cd examples/websocket/chat-server
go run main.go
```

Server listens on `http://localhost:8080`.

## Test

1. **Open browser**: Navigate to `http://localhost:8080`
2. **Enter username**: Type your name in the prompt
3. **Open multiple tabs**: Open the same URL in new tabs with different usernames
4. **Chat**: Type messages and see them appear in all tabs instantly

## Architecture

```
┌─────────────┐         ┌─────────────┐
│  Client 1   │◄────────┤             │
└─────────────┘         │             │
                        │     Hub     │
┌─────────────┐         │  (Manager)  │
│  Client 2   │◄────────┤             │
└─────────────┘         │             │
                        │             │
┌─────────────┐         │             │
│  Client 3   │◄────────┤             │
└─────────────┘         └─────────────┘
                              ▲
                              │
                        Broadcast()
```

## Message Format

All messages are JSON:

```json
{
  "type": "message",
  "username": "Alice",
  "text": "Hello, everyone!",
  "timestamp": "2025-01-18T10:30:00Z"
}
```

Message types:
- `join`: User connected
- `leave`: User disconnected
- `message`: Chat message

## Code Walkthrough

### 1. Create and Start Hub

```go
hub := websocket.NewHub()
go hub.Run()
defer hub.Close()
```

### 2. Register Clients

```go
conn, _ := websocket.Upgrade(w, r, nil)
hub.Register(conn)
defer hub.Unregister(conn)
```

### 3. Broadcast Messages

```go
// Option 1: Binary broadcast
hub.Broadcast([]byte("Hello"))

// Option 2: Text broadcast
hub.BroadcastText("Hello")

// Option 3: JSON broadcast (recommended)
msg := Message{Type: "message", Text: "Hello"}
hub.BroadcastJSON(msg)
```

### 4. Read JSON Messages

```go
var msg Message
if err := conn.ReadJSON(&msg); err != nil {
  // Handle error
}
```

## What It Demonstrates

- **Hub pattern**: Central broadcaster for multiple clients
- **JSON communication**: Type-safe message serialization
- **Goroutine-safe**: Hub methods are thread-safe
- **Auto-cleanup**: Failed writes auto-unregister clients
- **Non-blocking**: Broadcast queues messages asynchronously
- **Production pattern**: Real-world chat application structure

## Hub Performance

From `hub_bench_test.go`:

```
BenchmarkHub_Broadcast_10clients  - 11 μs/op (9x better than 100μs target)
BenchmarkHub_Broadcast_100clients - 75 μs/op (10M messages/s throughput)
```

## Client Count

```go
count := hub.ClientCount()
log.Printf("%d users online", count)
```

## Next Steps

- **Authentication**: Add JWT or session-based auth
- **Rooms**: Extend Hub to support multiple chat rooms
- **History**: Store messages in database
- **Typing indicators**: Use Hub for real-time status updates
- **File sharing**: Binary messages for file transfer
- **Production**: Add TLS, rate limiting, origin checking
