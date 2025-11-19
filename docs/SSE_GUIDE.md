# Server-Sent Events (SSE) Guide

Complete guide to using Server-Sent Events with `github.com/coregx/stream/sse`.

## Table of Contents

- [What is SSE?](#what-is-sse)
- [When to Use SSE](#when-to-use-sse)
- [Quick Start](#quick-start)
- [Event Structure](#event-structure)
- [Connection Lifecycle](#connection-lifecycle)
- [Broadcasting with Hub](#broadcasting-with-hub)
- [Best Practices](#best-practices)
- [Error Handling](#error-handling)
- [Production Considerations](#production-considerations)
- [Advanced Patterns](#advanced-patterns)
- [Comparison with WebSocket](#comparison-with-websocket)

---

## What is SSE?

**Server-Sent Events (SSE)** is a standard for servers to push real-time updates to clients over HTTP. It's part of the HTML5 specification and uses the `text/event-stream` content type.

### Key Characteristics

- **Unidirectional**: Server → Client only
- **Text-based**: UTF-8 encoded event stream
- **HTTP-based**: Works over standard HTTP/HTTPS
- **Auto-reconnect**: Browsers automatically reconnect on disconnect
- **EventSource API**: Native browser support (no libraries needed)

### SSE Event Format

```
event: eventType
id: uniqueId
retry: 3000
data: line 1
data: line 2

```

Each event ends with a blank line (`\n\n`).

---

## When to Use SSE

### ✅ Perfect For

- **Live notifications** (new message, alert, status update)
- **Real-time dashboards** (metrics, analytics, monitoring)
- **Activity streams** (social feeds, logs, events)
- **Live scores/tickers** (sports, stocks, news)
- **Progress updates** (file upload, job status)
- **Server logs streaming** (tail -f over HTTP)

### ❌ Not Ideal For

- **Bidirectional communication** (use WebSocket)
- **Binary data** (use WebSocket or chunked transfer)
- **High-frequency updates** (>10 events/sec → WebSocket)
- **Client → Server messaging** (POST or WebSocket)

### SSE vs WebSocket

| Feature | SSE | WebSocket |
|---------|-----|-----------|
| Direction | Server → Client | Bidirectional |
| Protocol | HTTP | TCP (ws://) |
| Data | Text (UTF-8) | Binary + Text |
| Browser API | EventSource | WebSocket |
| Reconnect | Automatic | Manual |
| Overhead | Lower | Higher |
| Complexity | Simpler | More complex |

**Rule of thumb**: Use SSE for server-push notifications, WebSocket for real-time collaboration.

---

## Quick Start

### 1. Simple SSE Endpoint

```go
package main

import (
    "net/http"
    "time"
    "github.com/coregx/stream/sse"
)

func main() {
    http.HandleFunc("/events", handleSSE)
    http.ListenAndServe(":8080", nil)
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
    // Upgrade to SSE
    conn, err := sse.Upgrade(w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer conn.Close()

    // Send events
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            conn.SendData("ping")
        case <-conn.Done():
            return
        }
    }
}
```

### 2. Test with curl

```bash
curl http://localhost:8080/events
```

Output:
```
: connected

data: ping

data: ping

data: ping
...
```

### 3. Test with JavaScript

```javascript
const eventSource = new EventSource('http://localhost:8080/events');

eventSource.onmessage = (e) => {
    console.log('Received:', e.data);
};

eventSource.onerror = (err) => {
    console.error('Error:', err);
};
```

---

## Event Structure

### Creating Events

The `Event` type represents a Server-Sent Event:

```go
type Event struct {
    Type  string  // Event type (optional)
    ID    string  // Event ID (optional)
    Data  string  // Event data (required)
    Retry int     // Reconnect time in ms (optional)
}
```

### Builder Pattern

```go
event := sse.NewEvent("Hello, World!").
    WithType("greeting").
    WithID("msg-1").
    WithRetry(3000)

// Serializes to:
// event: greeting
// id: msg-1
// retry: 3000
// data: Hello, World!
//
```

### Event Fields

#### Data (Required)

The message payload. Can be multi-line:

```go
event := sse.NewEvent("line1\nline2\nline3")

// Becomes:
// data: line1
// data: line2
// data: line3
//
```

#### Type (Optional)

Event type for client-side filtering:

```go
event := sse.NewEvent("New order!").WithType("order")
```

JavaScript:
```javascript
eventSource.addEventListener('order', (e) => {
    console.log('Order event:', e.data);
});
```

#### ID (Optional)

Unique event ID for reconnection tracking:

```go
event := sse.NewEvent("data").WithID("evt-123")
```

On reconnect, client sends `Last-Event-ID: evt-123` header.

#### Retry (Optional)

Milliseconds to wait before reconnecting:

```go
event := sse.NewEvent("data").WithRetry(5000) // 5 seconds
```

### Comments (Keep-Alive)

Comments keep the connection alive:

```go
comment := sse.Comment("keep-alive")
// Output: : keep-alive\n\n
```

Clients ignore comments, but they prevent timeouts.

---

## Connection Lifecycle

### Upgrade

The `Upgrade()` function converts an HTTP response to SSE:

```go
conn, err := sse.Upgrade(w, r)
if err != nil {
    // ResponseWriter doesn't support flushing
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
defer conn.Close()
```

**What Upgrade does**:
1. Checks `http.Flusher` support
2. Sets SSE headers (`Content-Type`, `Cache-Control`, etc.)
3. Sends initial `: connected` comment
4. Creates `Conn` with context

### Sending Events

Three ways to send events:

#### 1. Send(event)

Full control with Event builder:

```go
event := sse.NewEvent("Order #123 shipped").
    WithType("notification").
    WithID("notif-456")

conn.Send(event)
```

#### 2. SendData(string)

Simple data-only event:

```go
conn.SendData("Hello, World!")
```

Equivalent to:
```go
conn.Send(sse.NewEvent("Hello, World!"))
```

#### 3. SendJSON(any)

JSON-encode and send:

```go
user := map[string]string{"name": "Alice", "status": "online"}
conn.SendJSON(user)
```

Sends:
```
data: {"name":"Alice","status":"online"}

```

### Context Cancellation

Connections respect context cancellation:

```go
// Use request context (default)
conn, err := sse.Upgrade(w, r)

// Or custom context
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()
conn, err := sse.UpgradeWithContext(ctx, w, r)
```

When context is canceled, `conn.Done()` closes.

### Connection Closing

Always close connections:

```go
defer conn.Close()
```

`Close()` is idempotent (safe to call multiple times).

### Done Channel

Wait for disconnection:

```go
<-conn.Done()
```

Example with select:
```go
ticker := time.NewTicker(1 * time.Second)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        if err := conn.SendData("ping"); err != nil {
            return
        }
    case <-conn.Done():
        log.Println("Client disconnected")
        return
    }
}
```

---

## Broadcasting with Hub

`Hub[T]` manages broadcasting events to multiple clients.

### Creating a Hub

```go
hub := sse.NewHub[string]()
go hub.Run()
defer hub.Close()
```

**Generic type parameter** `T`:
- `Hub[string]` - broadcast strings
- `Hub[Message]` - broadcast custom types
- `Hub[any]` - broadcast any type

### Hub Lifecycle

```go
// Create
hub := sse.NewHub[Message]()

// Start (in goroutine)
go hub.Run()

// Use
hub.Register(conn)
hub.Broadcast(msg)

// Cleanup
defer hub.Close()
```

### Registering Connections

```go
conn, err := sse.Upgrade(w, r)
if err != nil {
    return
}
defer conn.Close()

// Register with hub
hub.Register(conn)
defer hub.Unregister(conn)

// Wait for disconnection
<-conn.Done()
```

### Broadcasting

```go
// String hub
hub := sse.NewHub[string]()
hub.Broadcast("Hello, everyone!")

// Custom type hub
type Message struct {
    User string
    Text string
}

hub := sse.NewHub[Message]()
hub.Broadcast(Message{User: "Alice", Text: "Hi!"})
```

### Custom Types with Hub

Custom types must implement `fmt.Stringer` OR be JSON-serializable:

```go
type Message struct {
    User string `json:"user"`
    Text string `json:"text"`
}

// Option 1: Implement Stringer
func (m Message) String() string {
    data, _ := json.Marshal(m)
    return string(data)
}

// Option 2: Let Hub JSON-encode (automatic)
hub := sse.NewHub[Message]()
hub.Broadcast(Message{User: "Alice", Text: "Hi"})
```

### Hub Metrics

Track active clients:

```go
count := hub.Clients()
log.Printf("Active clients: %d", count)
```

### Automatic Cleanup

Hub automatically removes failed clients:

```go
// Client disconnects → removed from hub
// Send fails → client unregistered
// Hub closes → all clients disconnected
```

---

## Best Practices

### 1. Always Close Connections

```go
conn, err := sse.Upgrade(w, r)
if err != nil {
    return
}
defer conn.Close()
```

### 2. Use Context for Timeouts

```go
ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
defer cancel()

conn, err := sse.UpgradeWithContext(ctx, w, r)
```

### 3. Send Keep-Alive Comments

Prevent proxy/firewall timeouts:

```go
ticker := time.NewTicker(30 * time.Second)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        // Send comment
        io.WriteString(w, ": keep-alive\n\n")
        flusher.Flush()
    case <-conn.Done():
        return
    }
}
```

### 4. Use Event IDs for Resumption

```go
eventID := 0

for msg := range messages {
    eventID++
    event := sse.NewEvent(msg).WithID(fmt.Sprintf("evt-%d", eventID))
    conn.Send(event)
}

// On reconnect, check Last-Event-ID header
lastID := r.Header.Get("Last-Event-ID")
if lastID != "" {
    // Resume from lastID
}
```

### 5. Limit Connection Duration

```go
ctx, cancel := context.WithTimeout(r.Context(), 1*time.Hour)
defer cancel()

conn, err := sse.UpgradeWithContext(ctx, w, r)
```

### 6. Monitor Hub Size

```go
const maxClients = 1000

if hub.Clients() >= maxClients {
    http.Error(w, "Too many clients", http.StatusServiceUnavailable)
    return
}

hub.Register(conn)
```

### 7. Use Structured Logging

```go
import "log/slog"

slog.Info("Client connected",
    "remote", r.RemoteAddr,
    "clients", hub.Clients(),
)
```

### 8. Graceful Shutdown

```go
// Close hub first (disconnects clients gracefully)
hub.Close()

// Then shutdown HTTP server
server.Shutdown(ctx)
```

---

## Error Handling

### Connection Errors

```go
conn, err := sse.Upgrade(w, r)
if err != nil {
    if errors.Is(err, sse.ErrNoFlusher) {
        // ResponseWriter doesn't support flushing
        log.Println("Incompatible server or proxy")
    }
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
```

### Send Errors

```go
if err := conn.Send(event); err != nil {
    if errors.Is(err, sse.ErrConnectionClosed) {
        // Connection already closed
        return
    }
    log.Printf("Send error: %v", err)
    return
}
```

### Hub Errors

```go
if err := hub.Register(conn); err != nil {
    if errors.Is(err, sse.ErrHubClosed) {
        // Hub is shutting down
        http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
        return
    }
}
```

### Handling Client Disconnects

```go
for {
    select {
    case msg := <-messages:
        if err := conn.SendData(msg); err != nil {
            // Client disconnected or send failed
            log.Printf("Client error: %v", err)
            return
        }
    case <-conn.Done():
        // Clean disconnect
        log.Println("Client disconnected")
        return
    }
}
```

---

## Production Considerations

### Reverse Proxy Configuration

#### Nginx

```nginx
location /events {
    proxy_pass http://backend;
    proxy_set_header Connection '';
    proxy_http_version 1.1;
    chunked_transfer_encoding off;
    proxy_buffering off;
    proxy_cache off;
}
```

#### Apache

```apache
<Location /events>
    ProxyPass http://backend/events
    ProxyPassReverse http://backend/events
    SetEnv proxy-sendcl 1
</Location>
```

### CORS for Cross-Origin

```go
func handleSSE(w http.ResponseWriter, r *http.Request) {
    // CORS headers
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Last-Event-ID")

    if r.Method == "OPTIONS" {
        return
    }

    conn, err := sse.Upgrade(w, r)
    // ...
}
```

### Authentication

```go
func handleSSE(w http.ResponseWriter, r *http.Request) {
    // Validate JWT token
    token := r.Header.Get("Authorization")
    if !validateToken(token) {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    conn, err := sse.Upgrade(w, r)
    // ...
}
```

### Rate Limiting

```go
type RateLimiter struct {
    clients map[string]*rate.Limiter
    mu      sync.Mutex
}

func (rl *RateLimiter) Allow(ip string) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    limiter, ok := rl.clients[ip]
    if !ok {
        limiter = rate.NewLimiter(10, 20) // 10 req/s, burst 20
        rl.clients[ip] = limiter
    }

    return limiter.Allow()
}
```

### Metrics and Monitoring

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    sseConnections = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "sse_connections_active",
        Help: "Number of active SSE connections",
    })

    sseEvents = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "sse_events_sent_total",
        Help: "Total number of SSE events sent",
    })
)

func init() {
    prometheus.MustRegister(sseConnections)
    prometheus.MustRegister(sseEvents)
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
    conn, err := sse.Upgrade(w, r)
    if err != nil {
        return
    }
    defer conn.Close()

    sseConnections.Inc()
    defer sseConnections.Dec()

    for event := range events {
        conn.Send(event)
        sseEvents.Inc()
    }
}
```

### Connection Limits

```go
const maxConnections = 10000

var activeConnections atomic.Int64

func handleSSE(w http.ResponseWriter, r *http.Request) {
    if activeConnections.Load() >= maxConnections {
        http.Error(w, "Too many connections", http.StatusServiceUnavailable)
        return
    }

    activeConnections.Add(1)
    defer activeConnections.Add(-1)

    // ...
}
```

---

## Advanced Patterns

### Multi-Room Broadcasting

```go
type ChatServer struct {
    rooms map[string]*sse.Hub[Message]
    mu    sync.RWMutex
}

func (cs *ChatServer) GetRoom(name string) *sse.Hub[Message] {
    cs.mu.Lock()
    defer cs.mu.Unlock()

    hub, ok := cs.rooms[name]
    if !ok {
        hub = sse.NewHub[Message]()
        go hub.Run()
        cs.rooms[name] = hub
    }
    return hub
}

func (cs *ChatServer) handleEvents(w http.ResponseWriter, r *http.Request) {
    room := r.URL.Query().Get("room")
    if room == "" {
        room = "default"
    }

    hub := cs.GetRoom(room)
    conn, _ := sse.Upgrade(w, r)
    hub.Register(conn)
    defer hub.Unregister(conn)

    <-conn.Done()
}
```

### Event Replay (History)

```go
type EventLog struct {
    events []sse.Event
    mu     sync.RWMutex
}

func (el *EventLog) Add(event *sse.Event) {
    el.mu.Lock()
    el.events = append(el.events, *event)
    el.mu.Unlock()
}

func (el *EventLog) ReplayFrom(conn *sse.Conn, lastID string) {
    el.mu.RLock()
    defer el.mu.RUnlock()

    found := false
    for _, event := range el.events {
        if event.ID == lastID {
            found = true
            continue
        }
        if found || lastID == "" {
            conn.Send(&event)
        }
    }
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
    conn, _ := sse.Upgrade(w, r)
    defer conn.Close()

    // Replay missed events
    lastID := r.Header.Get("Last-Event-ID")
    eventLog.ReplayFrom(conn, lastID)

    // Continue with live events
    // ...
}
```

### Filtered Broadcasting

```go
type FilteredHub struct {
    hub      *sse.Hub[Message]
    filters  map[*sse.Conn]func(Message) bool
    mu       sync.RWMutex
}

func (fh *FilteredHub) RegisterWithFilter(conn *sse.Conn, filter func(Message) bool) {
    fh.hub.Register(conn)
    fh.mu.Lock()
    fh.filters[conn] = filter
    fh.mu.Unlock()
}

func (fh *FilteredHub) Broadcast(msg Message) {
    fh.mu.RLock()
    defer fh.mu.RUnlock()

    for conn, filter := range fh.filters {
        if filter(msg) {
            conn.SendJSON(msg)
        }
    }
}
```

### Backpressure Handling

```go
func handleSSE(w http.ResponseWriter, r *http.Request) {
    conn, _ := sse.Upgrade(w, r)
    defer conn.Close()

    // Buffered channel for backpressure
    events := make(chan string, 100)

    // Sender goroutine
    go func() {
        for event := range events {
            if err := conn.SendData(event); err != nil {
                return
            }
        }
    }()

    // Producer
    for msg := range messages {
        select {
        case events <- msg:
            // Sent successfully
        default:
            // Channel full, drop or handle
            log.Println("Slow client, dropping message")
        }
    }
}
```

---

## Comparison with WebSocket

### When to Choose SSE

✅ **Use SSE when**:
- Server pushes updates to client (one-way)
- Simple text-based events
- Built-in reconnection needed
- HTTP infrastructure (proxies, load balancers)
- Simpler implementation
- EventSource API convenience

### When to Choose WebSocket

✅ **Use WebSocket when**:
- Bidirectional communication needed
- Binary data transfer
- Very high frequency (>10 events/sec)
- Custom protocol required
- Lower latency critical
- Gaming, video, audio streaming

### Feature Comparison

| Feature | SSE | WebSocket |
|---------|-----|-----------|
| Protocol | HTTP | TCP (ws://) |
| Direction | Server→Client | Bidirectional |
| Data Format | Text (UTF-8) | Binary + Text |
| Reconnection | Automatic | Manual |
| Event Types | Built-in | Custom |
| Browser API | EventSource | WebSocket |
| Proxies | Works well | May need config |
| Overhead | Low | Higher |
| Latency | ~100ms | ~10ms |
| Max Connections | ~6 per domain | Unlimited |

---

## Examples

See the [examples/](../examples/) directory for complete working examples:

- **[sse-basic](../examples/sse-basic/)** - Simple time updates
- **[sse-chat](../examples/sse-chat/)** - Multi-client chat with Hub

---

## API Reference

See [pkg.go.dev/github.com/coregx/stream/sse](https://pkg.go.dev/github.com/coregx/stream/sse) for full API documentation.

---

## Further Reading

- [SSE Specification](https://html.spec.whatwg.org/multipage/server-sent-events.html)
- [EventSource API](https://developer.mozilla.org/en-US/docs/Web/API/EventSource)
- [Using Server-Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events)
- [RFC 2616 - HTTP/1.1](https://tools.ietf.org/html/rfc2616)

---

*Last updated: 2025-01-18*
*Version: v0.1.0-alpha*
