# WebSocket Ping-Pong (Keep-Alive)

Demonstrates WebSocket keep-alive mechanism using Ping/Pong control frames.

## Features

- **Periodic pings**: Server sends Ping every 30 seconds
- **Automatic Pong**: `conn.Read()` auto-responds to Ping frames
- **Health monitoring**: Detect dead connections
- **RFC 6455 compliant**: Control frames per Section 5.5

## Run

```bash
cd examples/websocket/ping-pong
go run main.go
```

Server listens on `http://localhost:8080/ws`.

## Test

### Using wscat

```bash
wscat -c ws://localhost:8080/ws
```

**Server logs** (every 30 seconds):
```
Sending ping to 127.0.0.1:xxxxx
```

**Client receives**: Nothing visible (Pong handled by wscat automatically)

### Using JavaScript (Browser)

```html
<!DOCTYPE html>
<script>
  const ws = new WebSocket('ws://localhost:8080/ws');

  ws.onopen = () => console.log('Connected');

  // Ping/Pong handled automatically by browser
  // No onping/onpong events in browser WebSocket API

  ws.onmessage = (event) => {
    console.log('Message:', event.data);
  };

  ws.onclose = () => console.log('Disconnected');

  // Keep connection open by sending periodic messages
  setInterval(() => {
    if (ws.readyState === WebSocket.OPEN) {
      ws.send('Still alive!');
    }
  }, 45000); // Every 45 seconds
</script>
```

### Using Go client

```go
package main

import (
    "log"
    "net/http"
    "time"
    "github.com/coregx/stream/websocket"
)

func main() {
    // Create custom HTTP client for upgrade
    req, _ := http.NewRequest("GET", "ws://localhost:8080/ws", nil)
    req.Header.Set("Upgrade", "websocket")
    req.Header.Set("Connection", "Upgrade")
    req.Header.Set("Sec-WebSocket-Version", "13")
    req.Header.Set("Sec-WebSocket-Key", "x3JJHMbDL1EzLkh9GBhXDw==")

    // (Full client implementation requires handshake parsing)
    // For production, use gorilla/websocket or nhooyr/websocket
}
```

## RFC 6455 Ping/Pong

### Control Frames (Section 5.5)

**Ping (0x9)**:
- Sent by server to check if client is alive
- May contain application data (max 125 bytes)
- Client MUST respond with Pong

**Pong (0xA)**:
- Response to Ping
- MUST echo Ping application data
- Can be sent unsolicited (heartbeat)

### stream Implementation

**Automatic Pong Response**:
```go
// conn.Read() handles Ping automatically:
for {
    frame := readFrame(conn)
    if frame.opcode == opcodePing {
        conn.Pong(frame.payload) // Auto-respond
        continue
    }
    // ... handle data frames
}
```

**Manual Ping**:
```go
// Send Ping every 30 seconds
ticker := time.NewTicker(30 * time.Second)
for range ticker.C {
    conn.Ping([]byte("heartbeat"))
}
```

## Use Cases

### 1. Detect Dead Connections

```go
// If Ping fails, connection is dead
if err := conn.Ping(nil); err != nil {
    log.Printf("Connection dead: %v", err)
    conn.Close()
}
```

### 2. Keep NAT/Firewall Alive

Many routers close idle TCP connections after 60-120 seconds.
Pinging every 30s keeps connection active.

### 3. Idle Timeout

```go
// Close connection if no Pong received in 90 seconds
timeout := time.AfterFunc(90*time.Second, func() {
    conn.CloseWithCode(websocket.CloseGoingAway, "timeout")
})

// Reset timer on each successful Read()
defer timeout.Stop()
```

## What It Demonstrates

- **Ping interval**: Standard keep-alive pattern (30-60s)
- **Auto-Pong**: stream handles Pong automatically in Read()
- **Goroutine pattern**: Separate goroutine for periodic pings
- **Connection health**: Detect failures early
- **Production pattern**: Real-world keep-alive implementation

## Advanced: Custom Ping Handler

By default, `conn.Read()` auto-responds to Ping.
For custom behavior (e.g., logging), modify internal frame loop:

```go
// Internal (not exposed in stream v0.1.0):
if frame.opcode == opcodePing {
    log.Printf("Received ping: %s", frame.payload)
    conn.Pong(frame.payload) // Custom logic before Pong
    continue
}
```

## Best Practices

1. **Ping interval**: 30-60 seconds (balance between responsiveness and overhead)
2. **Timeout**: 2x ping interval (e.g., 120s timeout for 60s pings)
3. **Application data**: Keep < 10 bytes (e.g., timestamp, sequence number)
4. **Goroutine cleanup**: Use channels/context to stop ping goroutine on close
5. **Error handling**: Ping failure = close connection immediately

## Performance

Ping/Pong has minimal overhead:
- Frame size: 2-6 bytes header + payload (< 10 bytes)
- Network: ~10 bytes every 30s = 0.3 bytes/sec
- CPU: Negligible (frame parsing ~1 μs)

## Next Steps

- **Timeouts**: Add idle timeout detection
- **Reconnection**: Auto-reconnect on connection loss
- **Load balancing**: Use Ping to check backend health
- **Metrics**: Track Ping latency for monitoring
