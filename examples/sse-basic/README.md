# SSE Basic Example

A simple Server-Sent Events (SSE) example that sends time updates to clients.

## What It Does

This example demonstrates the basics of SSE:

- Creates an HTTP server with an SSE endpoint at `/events`
- Sends the current time to connected clients every second
- Uses `Event` type with ID and type fields
- Handles graceful shutdown on Ctrl+C
- Properly closes connections when clients disconnect

## Running

```bash
go run main.go
```

The server starts on http://localhost:8080

## Testing

### Using curl

```bash
curl http://localhost:8080/events
```

You should see output like:

```
: connected

event: time
id: evt-1
data: 2025-01-18T04:30:00Z

event: time
id: evt-2
data: 2025-01-18T04:30:01Z

event: time
id: evt-3
data: 2025-01-18T04:30:02Z
...
```

### Using JavaScript (browser)

Open your browser's developer console and run:

```javascript
const eventSource = new EventSource('http://localhost:8080/events');

eventSource.addEventListener('time', (e) => {
  console.log('Time update:', e.data, 'ID:', e.lastEventId);
});

eventSource.onerror = (err) => {
  console.error('EventSource error:', err);
};
```

### Using httpie

```bash
http --stream GET http://localhost:8080/events
```

## Code Walkthrough

### 1. Upgrade to SSE

```go
conn, err := sse.Upgrade(w, r)
if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
defer conn.Close()
```

This upgrades the HTTP connection to SSE, setting proper headers and preparing for streaming.

### 2. Create Events

```go
event := sse.NewEvent(now).
    WithType("time").
    WithID(fmt.Sprintf("evt-%d", eventID))
```

Events are built using the builder pattern with optional type and ID.

### 3. Send Events

```go
if err := conn.Send(event); err != nil {
    return
}
```

Send events to the client. Errors indicate the client disconnected.

### 4. Handle Disconnection

```go
case <-conn.Done():
    return
```

The `Done()` channel signals when the connection is closed.

## Key Concepts

### Event IDs

Event IDs enable client reconnection tracking:
- Client stores the last event ID received
- On reconnect, client sends `Last-Event-ID` header
- Server can resume from that point

### Event Types

Event types let clients listen for specific events:
- Default type is `"message"`
- Custom types like `"time"` enable targeted handling
- Clients use `addEventListener('time', handler)` to filter

### Connection Lifecycle

1. Client connects → `Upgrade()` sets headers
2. Server sends events → `Send()` writes to stream
3. Client disconnects → `Done()` channel closes
4. Server cleans up → `defer conn.Close()`

## Next Steps

See the [sse-chat example](../sse-chat/) for broadcasting to multiple clients using `Hub[T]`.

See [docs/SSE_GUIDE.md](../../docs/SSE_GUIDE.md) for comprehensive SSE documentation.
