# WebSocket Echo Server

Simple WebSocket server that echoes all received messages back to the client.

## Features

- HTTP to WebSocket upgrade using `websocket.Upgrade()`
- Read messages with `conn.Read()` - supports both text and binary
- Echo messages with `conn.Write()` - preserves message type
- Graceful connection handling with close frame detection

## Run

```bash
cd examples/websocket/echo-server
go run main.go
```

Server listens on `http://localhost:8080/ws`.

## Test

### Using wscat (Node.js)

```bash
npm install -g wscat
wscat -c ws://localhost:8080/ws
```

Type messages and see them echoed back:

```
> Hello, WebSocket!
< Hello, WebSocket!
> 👋 こんにちは
< 👋 こんにちは
```

### Using curl (HTTP upgrade)

```bash
curl -i -N \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  http://localhost:8080/ws
```

### Using JavaScript (Browser)

```html
<!DOCTYPE html>
<script>
  const ws = new WebSocket('ws://localhost:8080/ws');

  ws.onopen = () => {
    console.log('Connected');
    ws.send('Hello from browser!');
  };

  ws.onmessage = (event) => {
    console.log('Received:', event.data);
  };

  ws.onclose = () => console.log('Disconnected');
</script>
```

## What It Demonstrates

- **Upgrade handshake**: HTTP → WebSocket (RFC 6455 Section 4)
- **Message reading**: `conn.Read()` handles fragmentation, UTF-8 validation, control frames
- **Message writing**: `conn.Write()` preserves message type (text/binary)
- **Close handling**: `websocket.IsCloseError()` detects clean disconnection
- **Minimal code**: ~50 lines for full WebSocket echo server

## Code Walkthrough

```go
// 1. Upgrade HTTP connection
conn, err := websocket.Upgrade(w, r, nil)

// 2. Read message (blocking)
msgType, data, err := conn.Read()

// 3. Echo back (preserves type)
conn.Write(msgType, data)

// 4. Close gracefully
conn.Close()
```

## Next Steps

- **Chat server**: See `../chat-server/` for multi-client broadcasting
- **Ping/Pong**: See `../ping-pong/` for keep-alive mechanism
- **Production**: Add origin checking, TLS, authentication
