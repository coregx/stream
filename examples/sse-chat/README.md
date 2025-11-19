# SSE Chat Example

A real-time chat room using Server-Sent Events (SSE) with Hub for broadcasting to multiple clients.

## What It Does

This example demonstrates advanced SSE features:

- **Hub[T]** for broadcasting to multiple clients
- Custom message types with JSON serialization
- Multiple concurrent SSE connections
- POST endpoint for sending messages
- GET endpoint for receiving events
- HTML interface for browser testing
- Proper client registration/unregistration
- Graceful shutdown

## Running

```bash
go run main.go
```

The server starts on http://localhost:8080

## Testing

### Method 1: Browser (Easiest)

Open http://localhost:8080/ in multiple browser tabs/windows:

1. Enter your name
2. Type messages
3. See messages appear in all tabs in real-time

### Method 2: curl (Multiple Terminals)

Terminal 1 (Client 1):
```bash
curl http://localhost:8080/events
```

Terminal 2 (Client 2):
```bash
curl http://localhost:8080/events
```

Terminal 3 (Send messages):
```bash
# Send as Alice
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"user":"Alice","text":"Hello everyone!"}' \
  http://localhost:8080/messages

# Send as Bob
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"user":"Bob","text":"Hi Alice!"}' \
  http://localhost:8080/messages
```

Both client terminals will receive the messages in real-time!

### Method 3: JavaScript (Browser Console)

```javascript
// Connect to SSE
const eventSource = new EventSource('http://localhost:8080/events');

eventSource.onmessage = (e) => {
  const msg = JSON.parse(e.data);
  console.log(`${msg.user}: ${msg.text} (${msg.timestamp})`);
};

// Send a message
async function send(user, text) {
  await fetch('http://localhost:8080/messages', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user, text })
  });
}

send('Alice', 'Hello from console!');
```

## Architecture

### Message Type

```go
type Message struct {
    User      string    `json:"user"`
    Text      string    `json:"text"`
    Timestamp time.Time `json:"timestamp"`
}

// String implements fmt.Stringer for Hub broadcasting
func (m Message) String() string {
    data, _ := json.Marshal(m)
    return string(data)
}
```

Custom types work with `Hub[T]` by implementing `fmt.Stringer`.

### Hub Setup

```go
hub := sse.NewHub[Message]()
go hub.Run()
defer hub.Close()
```

The hub runs in a goroutine and manages all client connections.

### Client Registration

```go
conn, err := sse.Upgrade(w, r)
hub.Register(conn)
defer hub.Unregister(conn)

// Wait for disconnection
<-conn.Done()
```

Each SSE connection registers with the hub to receive broadcasts.

### Broadcasting

```go
msg := Message{
    User:      "Alice",
    Text:      "Hello!",
    Timestamp: time.Now(),
}

hub.Broadcast(msg)
```

Broadcasting sends the message to all registered clients automatically.

## Key Concepts

### Hub[T] Generic Broadcasting

The `Hub[T]` type is generic, allowing type-safe broadcasting:

- `Hub[string]` - broadcast strings
- `Hub[Message]` - broadcast custom types
- `Hub[any]` - broadcast any type (JSON-encoded)

### Type Conversion

Hub converts `T` to string for sending:

1. If `T` is `string` → sent as-is
2. If `T` implements `fmt.Stringer` → calls `String()`
3. Otherwise → JSON-encodes the value

### Automatic Cleanup

Hub automatically removes clients that fail to receive:

- Client disconnects → connection closed
- Send fails → client unregistered
- Hub closed → all clients disconnected

### Concurrency

Hub is fully concurrent:

- Thread-safe client registration
- Thread-safe broadcasting
- Non-blocking sends (buffered channels)
- Automatic cleanup on failures

## Production Considerations

### Scaling

For production, consider:

- **Multiple hub instances** (one per topic/room)
- **Connection limits** (max clients per hub)
- **Message queuing** (buffer for slow clients)
- **Metrics** (track client count, message rate)

### Keep-Alive

Add periodic keep-alive comments:

```go
ticker := time.NewTicker(30 * time.Second)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        io.WriteString(w, ": keep-alive\n\n")
        flusher.Flush()
    case <-conn.Done():
        return
    }
}
```

### Authentication

Add auth before registering:

```go
// Verify JWT token
token := r.Header.Get("Authorization")
if !validateToken(token) {
    http.Error(w, "Unauthorized", http.StatusUnauthorized)
    return
}

conn, err := sse.Upgrade(w, r)
hub.Register(conn)
```

### Monitoring

Track hub metrics:

```go
log.Printf("Active clients: %d", hub.Clients())
log.Printf("Message sent to %d clients", hub.Clients())
```

## Next Steps

- See [sse-basic example](../sse-basic/) for simpler SSE usage
- See [docs/SSE_GUIDE.md](../../docs/SSE_GUIDE.md) for comprehensive documentation
- Try building multi-room chat (multiple hubs)
- Add authentication and authorization
- Implement message history
- Add typing indicators
