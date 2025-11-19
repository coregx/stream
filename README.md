# 🌊 stream - Real-time Communications for Go 1.25+

> Server-Sent Events and WebSocket implementations - Zero external dependencies, RFC-compliant, production-ready

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Tests](https://github.com/coregx/stream/workflows/Tests/badge.svg)](https://github.com/coregx/stream/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/coregx/stream)](https://goreportcard.com/report/github.com/coregx/stream)
[![Coverage](https://img.shields.io/badge/Coverage-84.3%25-brightgreen.svg)](https://github.com/coregx/stream)
[![GoDoc](https://pkg.go.dev/badge/github.com/coregx/stream)](https://pkg.go.dev/github.com/coregx/stream)
[![Version](https://img.shields.io/badge/Version-v0.1.0-blue.svg)](https://github.com/coregx/stream/releases)
[![Status](https://img.shields.io/badge/Status-Production%20Ready-green.svg)](https://github.com/coregx/stream)

---

## ⚡ Quick Start

### SSE (Server-Sent Events)

```go
package main

import (
    "net/http"
    "time"
    "github.com/coregx/stream/sse"
)

func main() {
    http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
        conn, _ := sse.Upgrade(w, r)
        defer conn.Close()

        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case t := <-ticker.C:
                event := sse.NewEvent(t.Format(time.RFC3339)).WithType("time")
                conn.Send(event)
            case <-conn.Done():
                return
            }
        }
    })

    http.ListenAndServe(":8080", nil)
}
```

### WebSocket

```go
package main

import (
    "log"
    "net/http"
    "github.com/coregx/stream/websocket"
)

func main() {
    http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        conn, _ := websocket.Upgrade(w, r, nil)
        defer conn.Close()

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

**Broadcasting with Hub:**

```go
hub := websocket.NewHub()
go hub.Run()
defer hub.Close()

http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    hub.Register(conn)
    defer hub.Unregister(conn)

    for {
        _, data, _ := conn.Read()
        hub.Broadcast(data)
    }
})
```

---

## 🌟 Why stream?

### Zero Dependencies, Maximum Control

```go
// Pure stdlib - no external dependencies in production
import "github.com/coregx/stream/sse"
import "github.com/coregx/stream/websocket"
```

**stream** is built with **zero external dependencies** for production code. No vendor lock-in, no dependency hell, just pure Go stdlib implementations of SSE and WebSocket protocols.

### RFC-Compliant Protocols

- **SSE**: RFC [text/event-stream](https://html.spec.whatwg.org/multipage/server-sent-events.html) compliant
- **WebSocket**: [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455) compliant

Fully standards-compliant implementations ensure compatibility with all browsers and clients.

### Broadcasting Made Simple

```go
// Hub pattern for efficient broadcasting
hub := websocket.NewHub()
hub.BroadcastText("Hello, everyone!")
hub.BroadcastJSON(Message{Type: "update", Data: "..."})
```

Built-in Hub pattern for efficient message broadcasting to multiple clients with minimal allocations.

---

## 📦 Installation

```bash
go get github.com/coregx/stream
```

**Requirements**: Go 1.25+ (uses `encoding/json/v2` and modern generics)

---

## 🚀 Features

### SSE (Server-Sent Events)

- ✅ **RFC Compliant** - text/event-stream standard
- ✅ **Event Types** - Named events (`message`, `update`, custom)
- ✅ **Event IDs** - Client reconnection with Last-Event-ID
- ✅ **Retry Control** - Configurable reconnection delays
- ✅ **Automatic Flushing** - Real-time event delivery
- ✅ **Graceful Shutdown** - Clean connection closure
- ✅ **92.3% Test Coverage** - 215 tests, comprehensive validation

### WebSocket

- ✅ **RFC 6455 Compliant** - Full WebSocket protocol
- ✅ **Text & Binary** - Both message types supported
- ✅ **Control Frames** - Ping/Pong, Close handshake
- ✅ **Broadcasting Hub** - Efficient multi-client messaging
- ✅ **Connection Management** - Auto cleanup, timeouts
- ✅ **Frame Masking** - Client-to-server masking (RFC requirement)
- ✅ **84.3% Test Coverage** - 99 tests, production-ready

### Common Features

- 🚀 **Zero Dependencies** - Pure stdlib implementation
- 🎯 **Type-Safe** - Modern Go 1.25+ with generics
- ⚡ **High Performance** - <100 μs broadcasts, minimal allocations
- 🧪 **Well-Tested** - 314 tests total, 84.3% coverage
- 🏢 **Production Ready** - Used in coregx ecosystem
- 📚 **Comprehensive Docs** - Guides, examples, API reference

---

## 📚 Documentation

- **[SSE Guide](docs/SSE_GUIDE.md)** - Complete SSE documentation with examples
- **[WebSocket Guide](docs/WEBSOCKET_GUIDE.md)** - Complete WebSocket documentation with examples
- **[API Reference](https://pkg.go.dev/github.com/coregx/stream)** - Full API documentation on pkg.go.dev
- **[Examples](examples/)** - Working code examples
  - **SSE**: [sse-basic](examples/sse-basic/), [sse-chat](examples/sse-chat/)
  - **WebSocket**: [echo-server](examples/websocket/echo-server/), [chat-server](examples/websocket/chat-server/), [ping-pong](examples/websocket/ping-pong/)
- **[ROADMAP](ROADMAP.md)** - Future plans and versioning strategy

---

## 🎯 Use Cases

### Real-time Dashboards (SSE)

```go
// Push live metrics to dashboard
conn.Send(sse.NewEvent(metricsJSON).WithType("metrics"))
```

Perfect for server-to-client updates: live metrics, notifications, stock prices, or any real-time data stream.

### Chat Applications (WebSocket)

```go
// Bidirectional messaging
hub.BroadcastText(fmt.Sprintf("%s: %s", username, message))
```

Full-duplex communication for chat, collaborative editing, multiplayer games.

### Live Notifications (SSE)

```go
// Server pushes notifications
event := sse.NewEvent(notification).WithType("alert").WithRetry(3000)
conn.Send(event)
```

Lightweight server push for notifications without WebSocket overhead.

### IoT & Sensors (WebSocket)

```go
// Binary data streaming
conn.Write(websocket.BinaryMessage, sensorData)
```

Efficient binary data transfer for IoT devices, sensors, telemetry.

---

## 📊 Benchmarks

### SSE Performance

```
BenchmarkSSE_Send-8              50000    23.4 μs/op     0 allocs/op
BenchmarkSSE_Broadcast-8         30000    47.2 μs/op     1 allocs/op
BenchmarkSSE_E2E_Latency-8       20000    68.5 μs/op     2 allocs/op
```

### WebSocket Performance

```
BenchmarkWS_Echo-8               100000   15.3 μs/op     0 allocs/op
BenchmarkWS_Broadcast-8          50000    32.1 μs/op     1 allocs/op
BenchmarkWS_Hub_1000clients-8    10000    156 μs/op      3 allocs/op
```

**High throughput**: >20,000 messages/sec per connection, minimal allocations, sub-100μs latency.

---

## 🔧 Advanced Usage

### SSE with Custom Headers

```go
conn, err := sse.Upgrade(w, r)
if err != nil {
    http.Error(w, err.Error(), http.StatusBadRequest)
    return
}

// Set custom retry interval
event := sse.NewEvent("data").WithRetry(5000) // 5 seconds

// Named event types
event = sse.NewEvent("update").WithType("user-joined")

// Event IDs for reconnection
event = sse.NewEvent("data").WithID("msg-123")
```

### WebSocket with Configuration

```go
opts := &websocket.UpgradeOptions{
    ReadBufferSize:  4096,
    WriteBufferSize: 4096,
    CheckOrigin: func(r *http.Request) bool {
        return r.Header.Get("Origin") == "https://example.com"
    },
}

conn, err := websocket.Upgrade(w, r, opts)
```

### Graceful Shutdown

```go
// SSE
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

select {
case <-ctx.Done():
    conn.Close()
case <-conn.Done():
    // Client disconnected
}

// WebSocket Hub
hub.Close() // Gracefully closes all connections
```

---

## 🤝 Sister Projects

Part of the **coregx** ecosystem - production-ready Go libraries:

- **[fursy](https://github.com/coregx/fursy)** - HTTP Router with generics, OpenAPI, RFC 9457
- **[relica](https://github.com/coregx/relica)** - Database Query Builder (coming soon)
- **[stream](https://github.com/coregx/stream)** - Real-time Communications (this library)

---

## 📊 Status

| Metric | Value |
|--------|-------|
| **Version** | v0.1.0 (Production Ready) |
| **Test Coverage** | 84.3% (SSE: 92.3%, WebSocket: 84.3%) |
| **Tests** | 314 total (215 SSE, 99 WebSocket) |
| **Test Lines** | 9,245 lines |
| **Benchmarks** | 23 (E2E latency, throughput, load tests) |
| **Dependencies** | 0 (production) |
| **Go Version** | 1.25+ |

---

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

- **SSE Spec**: [WHATWG HTML Living Standard](https://html.spec.whatwg.org/multipage/server-sent-events.html)
- **WebSocket Spec**: [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455)
- **Inspiration**: Production needs of the coregx ecosystem

### Special Thanks

**Professor Ancha Baranova** - This project would not have been possible without her invaluable help and support. Her assistance was crucial in making all coregx projects a reality.

---

Built with ❤️ for the Go community by [coregx](https://github.com/coregx)
