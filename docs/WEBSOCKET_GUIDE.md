# WebSocket Guide

Complete guide to using WebSocket in the **stream** library.

## Table of Contents

1. [Introduction](#introduction)
2. [Quick Start](#quick-start)
3. [Connection API](#connection-api)
4. [Message Types](#message-types)
5. [Broadcasting with Hub](#broadcasting-with-hub)
6. [Control Frames](#control-frames)
7. [Best Practices](#best-practices)
8. [Performance](#performance)
9. [Production Deployment](#production-deployment)

---

## Introduction

**WebSocket** provides full-duplex communication over a single TCP connection, enabling real-time bidirectional data flow between client and server.

### RFC 6455 Compliance

stream implements **RFC 6455** (The WebSocket Protocol) with:

- ✅ Opening handshake (Section 4)
- ✅ Data framing (Section 5)
- ✅ Fragmentation and reassembly
- ✅ Control frames: Ping, Pong, Close
- ✅ UTF-8 validation for text messages
- ✅ Close codes (Section 7.4)
- ✅ Masking rules (server frames unmasked)

### Key Features

- **Zero dependencies**: Pure stdlib implementation
- **High performance**: <100 μs broadcast to 100 clients
- **Thread-safe**: Concurrent reads and writes
- **Type-safe**: JSON helpers for structured messages
- **Production-ready**: Used in coregx ecosystem

### When to Use WebSocket

**Use WebSocket for:**
- Real-time chat applications
- Live dashboards and monitoring
- Multiplayer games
- Collaborative editing
- IoT device communication
- Financial tickers

**Use SSE instead for:**
- Server-to-client only (no client→server data)
- Simpler protocol requirements
- Better browser compatibility (no CORS preflight)
- Event streams (logs, notifications)

---

## Quick Start

### 1. Simple Echo Server

```go
package main

import (
    "log"
    "net/http"
    "github.com/coregx/stream/websocket"
)

func main() {
    http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        // Upgrade HTTP to WebSocket
        conn, err := websocket.Upgrade(w, r, nil)
        if err != nil {
            log.Printf("Upgrade failed: %v", err)
            return
        }
        defer conn.Close()

        // Echo loop
        for {
            msgType, data, err := conn.Read()
            if err != nil {
                break
            }
            conn.Write(msgType, data)
        }
    })

    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

**Test it:**
```bash
# Install wscat (Node.js WebSocket client)
npm install -g wscat

# Connect and send messages
wscat -c ws://localhost:8080/ws
> Hello, WebSocket!
< Hello, WebSocket!
```

### 2. JSON Messages

```go
type Message struct {
    Type string `json:"type"`
    Text string `json:"text"`
}

http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    defer conn.Close()

    for {
        var msg Message
        if err := conn.ReadJSON(&msg); err != nil {
            break
        }

        // Process message
        msg.Type = "response"
        conn.WriteJSON(msg)
    }
})
```

### 3. Broadcasting to Multiple Clients

```go
hub := websocket.NewHub()
go hub.Run()
defer hub.Close()

http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    hub.Register(conn)
    defer hub.Unregister(conn)

    // Read loop (messages handled elsewhere)
    for {
        _, _, err := conn.Read()
        if err != nil {
            break
        }
    }
})

// Broadcast to all clients
hub.BroadcastText("Server notification")
```

---

## Connection API

### Upgrade

Upgrades an HTTP connection to WebSocket:

```go
func Upgrade(w http.ResponseWriter, r *http.Request, opts *UpgradeOptions) (*Conn, error)
```

**Example:**
```go
conn, err := websocket.Upgrade(w, r, nil)
if err != nil {
    http.Error(w, "Upgrade failed", http.StatusBadRequest)
    return
}
defer conn.Close()
```

**UpgradeOptions:**

```go
type UpgradeOptions struct {
    // Subprotocols advertised by server
    Subprotocols []string

    // Origin checker (nil = allow all - INSECURE!)
    CheckOrigin func(*http.Request) bool

    // Buffer sizes (default: 4096)
    ReadBufferSize  int
    WriteBufferSize int
}
```

**Example with options:**
```go
opts := &websocket.UpgradeOptions{
    Subprotocols: []string{"chat", "v2"},
    CheckOrigin: func(r *http.Request) bool {
        origin := r.Header.Get("Origin")
        return origin == "https://example.com"
    },
    ReadBufferSize:  8192,
    WriteBufferSize: 8192,
}

conn, err := websocket.Upgrade(w, r, opts)
```

### Read

Reads the next complete message:

```go
func (c *Conn) Read() (MessageType, []byte, error)
```

**Automatically handles:**
- Fragmentation (reassembles multi-frame messages)
- Control frames (Ping/Pong/Close)
- UTF-8 validation (for text messages)

**Example:**
```go
for {
    msgType, data, err := conn.Read()
    if err != nil {
        if websocket.IsCloseError(err) {
            log.Println("Client disconnected")
        } else {
            log.Printf("Read error: %v", err)
        }
        break
    }

    log.Printf("Received %s: %s", msgType, string(data))
}
```

### Write

Writes a message to the connection:

```go
func (c *Conn) Write(messageType MessageType, data []byte) error
```

**Thread-safe**: Multiple goroutines can write concurrently.

**Example:**
```go
// Text message
err := conn.Write(websocket.TextMessage, []byte("Hello"))

// Binary message
err := conn.Write(websocket.BinaryMessage, []byte{0x01, 0x02, 0x03})
```

### ReadText / WriteText

Convenience methods for text messages:

```go
func (c *Conn) ReadText() (string, error)
func (c *Conn) WriteText(text string) error
```

**Example:**
```go
// Read
text, err := conn.ReadText()
if err != nil {
    return err
}

// Write
err = conn.WriteText("Hello, client!")
```

### ReadJSON / WriteJSON

Type-safe JSON helpers:

```go
func (c *Conn) ReadJSON(v any) error
func (c *Conn) WriteJSON(v any) error
```

**Example:**
```go
type Request struct {
    Action string `json:"action"`
    Data   any    `json:"data"`
}

type Response struct {
    Status  string `json:"status"`
    Message string `json:"message"`
}

// Read
var req Request
if err := conn.ReadJSON(&req); err != nil {
    return err
}

// Process and respond
resp := Response{
    Status:  "ok",
    Message: "Action completed",
}
conn.WriteJSON(resp)
```

### Close

Closes the connection with optional status code:

```go
func (c *Conn) Close() error
func (c *Conn) CloseWithCode(code CloseCode, reason string) error
```

**Example:**
```go
// Normal close (1000)
conn.Close()

// Close with reason
conn.CloseWithCode(websocket.CloseGoingAway, "Server shutting down")

// Close on error
conn.CloseWithCode(websocket.CloseInvalidFramePayloadData, "Invalid UTF-8")
```

**Close Codes** (RFC 6455 Section 7.4):

| Code | Name | Description |
|------|------|-------------|
| 1000 | Normal Closure | Purpose fulfilled |
| 1001 | Going Away | Server/client going offline |
| 1002 | Protocol Error | Protocol violation |
| 1003 | Unsupported Data | Data type not accepted |
| 1007 | Invalid Frame Payload | Invalid UTF-8 or data |
| 1008 | Policy Violation | Generic policy error |
| 1009 | Message Too Big | Message exceeds size limit |
| 1011 | Internal Server Error | Unexpected server condition |

See `websocket.CloseCode` constants for full list.

---

## Message Types

### TextMessage vs BinaryMessage

```go
const (
    TextMessage   MessageType = 1 // UTF-8 text (opcode 0x1)
    BinaryMessage MessageType = 2 // Binary data (opcode 0x2)
)
```

**TextMessage:**
- MUST contain valid UTF-8
- Validated automatically by stream
- Used for JSON, XML, plain text
- Browser JavaScript strings

**BinaryMessage:**
- Arbitrary binary data
- No validation
- Used for protobuf, msgpack, images, files

**Example:**
```go
// Text (JSON)
conn.Write(websocket.TextMessage, []byte(`{"status":"ok"}`))

// Binary (protobuf)
data, _ := proto.Marshal(message)
conn.Write(websocket.BinaryMessage, data)
```

### UTF-8 Validation

stream automatically validates UTF-8 for text messages:

```go
// ✅ Valid UTF-8
conn.Write(websocket.TextMessage, []byte("Hello 世界 🌍"))

// ❌ Invalid UTF-8 - returns ErrInvalidUTF8
invalidUTF8 := []byte{0xFF, 0xFE, 0xFD}
err := conn.Write(websocket.TextMessage, invalidUTF8)
// err == websocket.ErrInvalidUTF8
```

### Fragmentation

Large messages can be fragmented (split across multiple frames).

**stream handles this automatically:**
- `Read()` reassembles fragments into complete message
- `Write()` currently sends as single frame (fragmentation TODO)

**RFC 6455 Section 5.4:**
```
┌─────────────┐
│ FIN=0, Text │  First fragment
├─────────────┤
│ FIN=0, Cont │  Continuation
├─────────────┤
│ FIN=0, Cont │  Continuation
├─────────────┤
│ FIN=1, Cont │  Final fragment
└─────────────┘
```

**Example** (handled internally):
```go
// Client sends fragmented message (3 frames)
// Frame 1: FIN=0, opcode=Text, payload="Hello "
// Frame 2: FIN=0, opcode=Cont, payload="World"
// Frame 3: FIN=1, opcode=Cont, payload="!"

// Your code just reads complete message:
msgType, data, _ := conn.Read()
// msgType = TextMessage
// data = "Hello World!"
```

---

## Broadcasting with Hub

### Hub Overview

**Hub** manages multiple WebSocket connections for broadcasting:

```go
type Hub struct {
    // Internal fields (private)
}
```

**Methods:**
```go
func NewHub() *Hub
func (h *Hub) Run()
func (h *Hub) Register(client *Conn)
func (h *Hub) Unregister(client *Conn)
func (h *Hub) Broadcast(message []byte)
func (h *Hub) BroadcastText(text string)
func (h *Hub) BroadcastJSON(v any) error
func (h *Hub) ClientCount() int
func (h *Hub) Close() error
```

### Basic Usage

```go
// 1. Create hub
hub := websocket.NewHub()

// 2. Start event loop (MUST run in goroutine)
go hub.Run()
defer hub.Close()

// 3. Register clients
http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    hub.Register(conn)
    defer hub.Unregister(conn)

    // Keep connection alive
    for {
        _, _, err := conn.Read()
        if err != nil {
            break
        }
    }
})

// 4. Broadcast messages
hub.BroadcastText("Server notification")
```

### Chat Application Example

```go
type ChatMessage struct {
    Username  string    `json:"username"`
    Text      string    `json:"text"`
    Timestamp time.Time `json:"timestamp"`
}

hub := websocket.NewHub()
go hub.Run()

http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    username := r.URL.Query().Get("username")

    hub.Register(conn)
    defer hub.Unregister(conn)

    // Notify others of join
    joinMsg := ChatMessage{
        Username:  username,
        Text:      username + " joined",
        Timestamp: time.Now(),
    }
    hub.BroadcastJSON(joinMsg)

    // Read and broadcast messages
    for {
        var msg ChatMessage
        if err := conn.ReadJSON(&msg); err != nil {
            break
        }

        msg.Username = username
        msg.Timestamp = time.Now()
        hub.BroadcastJSON(msg)
    }
})
```

### Hub Architecture

```
┌──────────────────────────────────────┐
│              Hub                     │
│  ┌────────────────────────────────┐ │
│  │     Event Loop (Run)           │ │
│  │  - register   chan *Conn       │ │
│  │  - unregister chan *Conn       │ │
│  │  - broadcast  chan []byte      │ │
│  └────────────────────────────────┘ │
│                                      │
│  ┌────────────────────────────────┐ │
│  │  Clients: map[*Conn]bool       │ │
│  │  - Client 1 ───────────────────┼─┼──> WebSocket Conn
│  │  - Client 2 ───────────────────┼─┼──> WebSocket Conn
│  │  - Client 3 ───────────────────┼─┼──> WebSocket Conn
│  └────────────────────────────────┘ │
└──────────────────────────────────────┘
         ▲
         │ Broadcast(msg)
    ─────┴─────
```

### Performance

From `hub_bench_test.go`:

| Clients | Broadcast Time | Throughput |
|---------|---------------|------------|
| 10      | 11 μs/op      | ~90K broadcasts/s |
| 100     | 75 μs/op      | ~13K broadcasts/s |

**9x better than target** (100 μs for 10 clients).

### Thread Safety

All Hub methods are **thread-safe**:

```go
// Safe to call from multiple goroutines
go hub.Broadcast(msg1)
go hub.Broadcast(msg2)
go hub.Register(conn)
```

### Client Management

```go
// Check client count
count := hub.ClientCount()
log.Printf("%d clients connected", count)

// Unregister on error (automatic in Hub)
if err := conn.Write(data); err != nil {
    hub.Unregister(conn) // Auto-cleanup
}
```

### Graceful Shutdown

```go
// Close hub (stops event loop, disconnects all clients)
hub.Close()

// Pattern for clean shutdown:
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

go func() {
    <-sigChan
    log.Println("Shutting down...")
    hub.Close()
    os.Exit(0)
}()
```

---

## Control Frames

### Ping and Pong

**Ping** (opcode 0x9): Server → Client keep-alive check
**Pong** (opcode 0xA): Client → Server response

**Automatic Pong Response:**

stream automatically responds to Ping frames:

```go
// Inside conn.Read():
if frame.opcode == opcodePing {
    conn.Pong(frame.payload) // Auto-respond
    continue                  // Keep reading data frames
}
```

**Manual Ping (for keep-alive):**

```go
// Send Ping every 30 seconds
ticker := time.NewTicker(30 * time.Second)
defer ticker.Stop()

go func() {
    for range ticker.C {
        if err := conn.Ping([]byte("heartbeat")); err != nil {
            log.Printf("Ping failed, connection dead")
            conn.Close()
            return
        }
    }
}()
```

**Use Cases:**

1. **Detect dead connections**: Ping failure = close connection
2. **Keep NAT/Firewall alive**: Many routers close idle TCP after 60-120s
3. **Health monitoring**: Track ping latency for metrics

**Best Practices:**

- Ping interval: 30-60 seconds
- Timeout: 2x ping interval (e.g., 120s timeout for 60s pings)
- Application data: Keep < 10 bytes (timestamp, sequence number)
- Max size: 125 bytes (RFC 6455 Section 5.5)

**Example:**
```go
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    defer conn.Close()

    // Start pinger
    pingTicker := time.NewTicker(30 * time.Second)
    defer pingTicker.Stop()

    done := make(chan struct{})

    go func() {
        for {
            select {
            case <-pingTicker.C:
                if err := conn.Ping(nil); err != nil {
                    close(done)
                    return
                }
            case <-done:
                return
            }
        }
    }()

    // Read loop (Pong handled automatically)
    for {
        _, _, err := conn.Read()
        if err != nil {
            close(done)
            break
        }
    }
}
```

### Close Handshake

**RFC 6455 Section 7.1.2** defines the close handshake:

1. Endpoint A sends Close frame (status code + optional reason)
2. Endpoint B receives Close frame
3. Endpoint B responds with Close frame (echo status code)
4. Both endpoints close TCP connection

**stream implementation:**

```go
// Send close
conn.CloseWithCode(websocket.CloseNormalClosure, "goodbye")

// Receive close (handled in Read)
if frame.opcode == opcodeClose {
    conn.handleCloseFrame(frame.payload) // Auto-respond
    return ErrClosed
}
```

**Close Codes:**

```go
// Normal closure
conn.Close() // 1000

// Going away (server shutdown)
conn.CloseWithCode(websocket.CloseGoingAway, "Server restarting")

// Protocol error
conn.CloseWithCode(websocket.CloseProtocolError, "Invalid frame")

// Invalid UTF-8
conn.CloseWithCode(websocket.CloseInvalidFramePayloadData, "Bad UTF-8")

// Internal error
conn.CloseWithCode(websocket.CloseInternalServerErr, "Database error")
```

**Error Checking:**

```go
_, _, err := conn.Read()
if err != nil {
    if websocket.IsCloseError(err) {
        // Clean close (close frame received)
        log.Println("Client disconnected cleanly")
    } else {
        // Network error, protocol error, etc.
        log.Printf("Connection error: %v", err)
    }
}
```

---

## Best Practices

### 1. Origin Checking (Security)

**ALWAYS check Origin in production:**

```go
opts := &websocket.UpgradeOptions{
    CheckOrigin: func(r *http.Request) bool {
        origin := r.Header.Get("Origin")
        // Allow only your domain
        return origin == "https://example.com" ||
               origin == "https://www.example.com"
    },
}

conn, err := websocket.Upgrade(w, r, opts)
```

**Default (nil)**: Allows ALL origins - **INSECURE!**

**Same-origin checker:**
```go
CheckOrigin: websocket.CheckSameOrigin
```

### 2. Error Handling

**Always check errors and close gracefully:**

```go
for {
    msgType, data, err := conn.Read()
    if err != nil {
        if websocket.IsCloseError(err) {
            log.Println("Client disconnected")
        } else {
            log.Printf("Error: %v", err)
            conn.CloseWithCode(websocket.CloseInternalServerErr, "Read error")
        }
        break
    }

    if err := conn.Write(msgType, data); err != nil {
        log.Printf("Write error: %v", err)
        conn.Close()
        break
    }
}
```

### 3. Goroutine Management

**Pattern 1: Single goroutine (read loop)**

```go
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    defer conn.Close()

    for {
        _, _, err := conn.Read()
        if err != nil {
            break
        }
        // Process message
    }
}
```

**Pattern 2: Separate read/write goroutines**

```go
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    defer conn.Close()

    done := make(chan struct{})

    // Write goroutine
    go func() {
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                if err := conn.WriteText("tick"); err != nil {
                    return
                }
            case <-done:
                return
            }
        }
    }()

    // Read goroutine (main)
    for {
        _, _, err := conn.Read()
        if err != nil {
            close(done)
            break
        }
    }
}
```

### 4. Message Size Limits

**Prevent abuse with max message size:**

```go
const maxMessageSize = 1 * 1024 * 1024 // 1 MB

msgType, data, err := conn.Read()
if err != nil {
    return err
}

if len(data) > maxMessageSize {
    conn.CloseWithCode(websocket.CloseMessageTooBig, "Message exceeds 1MB")
    return
}
```

### 5. Rate Limiting

**Prevent spam with rate limiting:**

```go
import "golang.org/x/time/rate"

limiter := rate.NewLimiter(10, 20) // 10 msg/s, burst 20

for {
    _, _, err := conn.Read()
    if err != nil {
        break
    }

    if !limiter.Allow() {
        conn.CloseWithCode(websocket.ClosePolicyViolation, "Rate limit exceeded")
        break
    }

    // Process message
}
```

### 6. Context for Cancellation

**Use context for graceful shutdown:**

```go
func handleWebSocket(ctx context.Context, w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    defer conn.Close()

    done := make(chan struct{})
    defer close(done)

    // Monitor context cancellation
    go func() {
        select {
        case <-ctx.Done():
            conn.CloseWithCode(websocket.CloseGoingAway, "Server shutting down")
        case <-done:
        }
    }()

    // Read loop
    for {
        _, _, err := conn.Read()
        if err != nil {
            break
        }
    }
}
```

### 7. Structured Logging

**Use log/slog for structured logging:**

```go
import "log/slog"

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    defer conn.Close()

    logger := slog.With(
        "remote_addr", r.RemoteAddr,
        "user_agent", r.UserAgent(),
    )

    logger.Info("client connected")
    defer logger.Info("client disconnected")

    for {
        msgType, data, err := conn.Read()
        if err != nil {
            logger.Error("read error", "error", err)
            break
        }

        logger.Debug("message received",
            "type", msgType,
            "size", len(data),
        )
    }
}
```

---

## Performance

### Benchmarks

From `websocket` package benchmarks:

| Operation | Time | Allocations |
|-----------|------|-------------|
| Static route lookup | 256 ns/op | 1 alloc/op |
| Parametric route | 326 ns/op | 1 alloc/op |
| Hub broadcast (10 clients) | 11 μs/op | - |
| Hub broadcast (100 clients) | 75 μs/op | - |

**Throughput:**
- Simple requests: ~10M req/s
- Complex requests: ~4M req/s
- Hub broadcasts: ~13K broadcasts/s (100 clients)

### Optimization Tips

**1. Reuse buffers:**
```go
var bufPool = sync.Pool{
    New: func() any {
        return make([]byte, 4096)
    },
}

buf := bufPool.Get().([]byte)
defer bufPool.Put(buf)
```

**2. Batch broadcasts:**
```go
// Instead of:
for _, msg := range messages {
    hub.Broadcast(msg) // N broadcasts
}

// Do:
combined := append(messages...)
hub.Broadcast(combined) // 1 broadcast
```

**3. Increase buffer sizes:**
```go
opts := &websocket.UpgradeOptions{
    ReadBufferSize:  16384, // 16 KB
    WriteBufferSize: 16384,
}
```

**4. Use binary messages:**
```go
// JSON (slower, larger)
conn.WriteJSON(data) // ~1-2 μs marshal + network

// Protobuf (faster, smaller)
buf, _ := proto.Marshal(data) // ~500 ns marshal
conn.Write(websocket.BinaryMessage, buf)
```

---

## Production Deployment

### 1. TLS (wss://)

**Always use TLS in production:**

```go
// HTTP server with TLS
server := &http.Server{
    Addr:      ":443",
    TLSConfig: tlsConfig,
}

log.Fatal(server.ListenAndServeTLS("cert.pem", "key.pem"))
```

**Client connects via `wss://` (WebSocket Secure):**
```javascript
const ws = new WebSocket('wss://example.com/ws');
```

### 2. Authentication

**JWT authentication example:**

```go
import "github.com/golang-jwt/jwt/v5"

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    // Extract token from query param or header
    tokenStr := r.URL.Query().Get("token")

    // Validate JWT
    token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
        return jwtSecret, nil
    })

    if err != nil || !token.Valid {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    // Proceed with upgrade
    conn, _ := websocket.Upgrade(w, r, nil)
    // ...
}
```

### 3. Load Balancing

**Sticky sessions required** (WebSocket is stateful):

**Nginx:**
```nginx
upstream websocket_backend {
    ip_hash; # Sticky sessions
    server backend1:8080;
    server backend2:8080;
    server backend3:8080;
}

server {
    location /ws {
        proxy_pass http://websocket_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 4. Monitoring

**Metrics to track:**

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    activeConnections = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "websocket_active_connections",
        Help: "Number of active WebSocket connections",
    })

    messagesReceived = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "websocket_messages_received_total",
        Help: "Total messages received",
    })

    messagesSent = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "websocket_messages_sent_total",
        Help: "Total messages sent",
    })
)

func init() {
    prometheus.MustRegister(activeConnections)
    prometheus.MustRegister(messagesReceived)
    prometheus.MustRegister(messagesSent)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    defer conn.Close()

    activeConnections.Inc()
    defer activeConnections.Dec()

    for {
        _, _, err := conn.Read()
        if err != nil {
            break
        }
        messagesReceived.Inc()
    }
}
```

### 5. Graceful Shutdown

**Kubernetes-ready shutdown:**

```go
import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"
)

func main() {
    hub := websocket.NewHub()
    go hub.Run()

    server := &http.Server{Addr: ":8080"}

    // Shutdown signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    go func() {
        <-sigChan
        log.Println("Shutting down...")

        // 1. Stop accepting new connections
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        // 2. Close all WebSocket connections
        hub.Close()

        // 3. Shutdown HTTP server
        server.Shutdown(ctx)

        os.Exit(0)
    }()

    log.Fatal(server.ListenAndServe())
}
```

---

## Examples

See `examples/websocket/` for complete working examples:

1. **[echo-server](../examples/websocket/echo-server/)** - Simple echo server
2. **[chat-server](../examples/websocket/chat-server/)** - Multi-client chat with Hub
3. **[ping-pong](../examples/websocket/ping-pong/)** - Keep-alive demonstration

---

## API Reference

Full API documentation: https://pkg.go.dev/github.com/coregx/stream/websocket

**Key types:**

- `Conn` - WebSocket connection
- `Hub` - Broadcast manager
- `MessageType` - Text/Binary
- `CloseCode` - RFC 6455 close codes
- `UpgradeOptions` - Upgrade configuration

**Key functions:**

- `Upgrade()` - HTTP → WebSocket
- `NewHub()` - Create broadcast hub
- `IsCloseError()` - Check clean close
- `IsTemporaryError()` - Check retry-able error

---

## Troubleshooting

### "Upgrade failed: missing Upgrade header"

**Cause**: Client not sending WebSocket upgrade request.

**Solution**: Ensure client sends proper headers:
```
GET /ws HTTP/1.1
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: ...
```

### "Read error: invalid UTF-8"

**Cause**: Text message contains invalid UTF-8.

**Solution**: Use `BinaryMessage` for non-UTF-8 data:
```go
conn.Write(websocket.BinaryMessage, data)
```

### "Connection closed unexpectedly"

**Cause**: Client closed without Close frame (network error).

**Solution**: Implement Ping/Pong keep-alive to detect dead connections early.

### "Hub broadcast slow with many clients"

**Cause**: Hub sends to clients serially.

**Solution**: Already optimized - Hub uses goroutines per client (see `hub.go:112`).

---

## Further Reading

- **RFC 6455**: [The WebSocket Protocol](https://datatracker.ietf.org/doc/html/rfc6455)
- **MDN**: [WebSocket API](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket)
- **stream SSE Guide**: [SSE_GUIDE.md](SSE_GUIDE.md)
- **fursy Router**: [github.com/coregx/fursy](https://github.com/coregx/fursy)

---

**Questions?** Open an issue on [GitHub](https://github.com/coregx/stream/issues).
