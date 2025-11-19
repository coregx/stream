# Security Policy

## Supported Versions

stream library is currently in active development (0.x versions). We provide security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |
| < 0.1.0 | :x:                |

Future stable releases (v1.0+) will follow semantic versioning with LTS support.

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability in stream, please report it responsibly.

### How to Report

**DO NOT** open a public GitHub issue for security vulnerabilities.

Instead, please report security issues by:

1. **Private Security Advisory** (preferred):
   https://github.com/coregx/stream/security/advisories/new

2. **Email** to maintainers:
   Create a private GitHub issue or contact via discussions

### What to Include

Please include the following information in your report:

- **Description** of the vulnerability
- **Steps to reproduce** the issue (include proof-of-concept if applicable)
- **Affected versions** (which versions are impacted)
- **Potential impact** (DoS, information disclosure, RCE, etc.)
- **Suggested fix** (if you have one)
- **Your contact information** (for follow-up questions)

### Response Timeline

- **Initial Response**: Within 48-72 hours
- **Triage & Assessment**: Within 1 week
- **Fix & Disclosure**: Coordinated with reporter

We aim to:
1. Acknowledge receipt within 72 hours
2. Provide an initial assessment within 1 week
3. Work with you on a coordinated disclosure timeline
4. Credit you in the security advisory (unless you prefer to remain anonymous)

## Security Considerations for Real-Time Communications

Real-time communication libraries (SSE and WebSocket) handle long-lived connections and untrusted data streams, introducing unique security risks.

### 1. WebSocket Handshake Attacks

**Risk**: Malicious handshake requests can exploit upgrade vulnerabilities.

**Attack Vectors**:
- Invalid `Sec-WebSocket-Key` values
- Missing required headers
- Origin header spoofing
- Subprotocol injection

**Mitigation in Library**:
- ✅ RFC 6455 compliant handshake validation
- ✅ Strict header validation
- ✅ Proper `Sec-WebSocket-Accept` computation
- ✅ Version negotiation (only version 13 supported)
- ✅ Optional origin validation

**User Recommendations**:
```go
// ❌ BAD - Don't skip origin validation in production
conn, _ := websocket.Upgrade(w, r, nil)

// ✅ GOOD - Validate origins
opts := &websocket.UpgradeOptions{
    CheckOrigin: func(r *http.Request) bool {
        origin := r.Header.Get("Origin")
        return origin == "https://yourdomain.com"
    },
}
conn, err := websocket.Upgrade(w, r, opts)
if err != nil {
    http.Error(w, "Forbidden", http.StatusForbidden)
    return
}
```

### 2. Denial of Service (DoS) Attacks

**Risk**: Long-lived connections can exhaust server resources.

**Attack Vectors**:
- Connection exhaustion (opening thousands of connections)
- Message flooding (sending excessive messages)
- Large frame/event payloads (memory exhaustion)
- Slow-read attacks (not consuming data)

**Mitigation in Library**:
- ✅ Configurable frame size limits
- ✅ Read/write timeouts
- ✅ Proper connection cleanup
- ✅ Hub-based broadcasting (efficient multi-client handling)

**User Recommendations**:
```go
// ✅ Set connection limits
opts := &websocket.UpgradeOptions{
    ReadBufferSize:  4096,
    WriteBufferSize: 4096,
}

// ✅ Implement connection counting
var connCount atomic.Int32
http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    if connCount.Load() >= 1000 {
        http.Error(w, "Too many connections", http.StatusServiceUnavailable)
        return
    }

    conn, _ := websocket.Upgrade(w, r, opts)
    connCount.Add(1)
    defer connCount.Add(-1)
    defer conn.Close()
    // Handle connection...
})

// ✅ Implement rate limiting per connection
limiter := rate.NewLimiter(100, 10)  // 100 msg/s, burst 10
for {
    if !limiter.Allow() {
        conn.Close()
        break
    }
    msgType, data, err := conn.Read()
    if err != nil {
        break
    }
    // Process message...
}
```

### 3. Message Injection and XSS

**Risk**: Malicious messages can exploit client-side vulnerabilities.

**Attack Vectors**:
- XSS via SSE event data
- XSS via WebSocket messages
- Malicious JSON payloads
- Script injection in event streams

**Mitigation**:
- ✅ JSON parsing uses `encoding/json/v2` (safe unmarshaling)
- ✅ Binary-safe message handling
- 🔄 **User Responsibility**: Sanitize data before sending to clients

**User Best Practices**:
```go
// ❌ BAD - Don't send unsanitized user input
hub.BroadcastText(userInput)

// ✅ GOOD - Sanitize HTML content
import "html"

safeData := html.EscapeString(userInput)
hub.BroadcastText(safeData)

// ✅ BETTER - Use JSON (auto-escaped)
type Message struct {
    User string `json:"user"`
    Text string `json:"text"`
}

hub.BroadcastJSON(Message{
    User: username,
    Text: userMessage,  // Auto-escaped in JSON
})

// ✅ WebSocket - validate message types
msgType, data, err := conn.Read()
if err != nil {
    return
}
if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
    conn.Close()  // Reject invalid types
    return
}
```

### 4. Authentication & Authorization

**Risk**: Unauthenticated access to real-time streams.

**Attack Vectors**:
- Missing authentication on upgrade
- Token theft via insecure connections
- Session hijacking
- Unauthorized subscription to private channels

**Mitigation**:
- ✅ Upgrade happens at HTTP layer (use standard HTTP authentication)
- ✅ Origin validation support
- 🔄 **User Responsibility**: Implement authentication before upgrade

**User Best Practices**:
```go
// ✅ Authenticate BEFORE upgrading to WebSocket
http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    // Validate JWT token from query param or header
    token := r.URL.Query().Get("token")
    user, err := validateToken(token)
    if err != nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    // Now upgrade
    conn, err := websocket.Upgrade(w, r, &websocket.UpgradeOptions{
        CheckOrigin: func(r *http.Request) bool {
            return r.Header.Get("Origin") == "https://yourdomain.com"
        },
    })
    if err != nil {
        return
    }

    // Handle authenticated connection
    handleAuthenticatedConn(conn, user)
})

// ✅ SSE - use standard HTTP auth
http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
    // Check Authorization header
    if !isAuthorized(r) {
        http.Error(w, "Forbidden", http.StatusForbidden)
        return
    }

    conn, _ := sse.Upgrade(w, r)
    defer conn.Close()
    // Send authorized events only
})
```

### 5. Frame Injection and Malformed Data

**Risk**: Malicious WebSocket frames can crash or compromise clients.

**Attack Vectors**:
- Malformed WebSocket frames
- Invalid UTF-8 in text frames
- Incorrect masking
- Reserved opcodes

**Mitigation in Library**:
- ✅ RFC 6455 compliant frame parsing
- ✅ UTF-8 validation for text frames
- ✅ Proper masking/unmasking
- ✅ Opcode validation
- ✅ Max frame size limits

**User Best Practices**:
```go
// ✅ Validate received data
msgType, data, err := conn.Read()
if err != nil {
    log.Printf("Read error: %v", err)
    conn.Close()
    return
}

// Validate message type
if msgType != websocket.TextMessage {
    log.Printf("Unexpected message type: %d", msgType)
    conn.Close()
    return
}

// Parse and validate JSON
var msg Message
if err := json.Unmarshal(data, &msg); err != nil {
    log.Printf("Invalid JSON: %v", err)
    return  // Ignore bad message, don't close
}
```

### 6. Resource Exhaustion via Hubs

**Risk**: Broadcasting to many clients can exhaust resources.

**Attack Vectors**:
- Memory exhaustion from buffered channels
- Goroutine leaks from unregistered clients
- CPU exhaustion from excessive broadcasts

**Mitigation in Library**:
- ✅ Hub uses buffered channels
- ✅ Proper client cleanup on unregister
- ✅ Non-blocking broadcast operations
- ✅ Graceful hub shutdown

**User Best Practices**:
```go
// ✅ Always unregister clients
http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Upgrade(w, r, nil)
    hub.Register(conn)
    defer hub.Unregister(conn)  // Critical!

    for {
        _, data, err := conn.Read()
        if err != nil {
            break
        }
        hub.Broadcast(data)
    }
})

// ✅ Monitor hub health
go func() {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        count := hub.ClientCount()
        if count > 10000 {
            log.Printf("WARNING: High client count: %d", count)
        }
    }
}()
```

## Security Best Practices for Users

### Input Validation

Always validate messages from clients:

```go
// Define message schema
type ChatMessage struct {
    Type    string `json:"type"`
    Content string `json:"content"`
    UserID  string `json:"user_id"`
}

// Validate all incoming messages
for {
    msgType, data, err := conn.Read()
    if err != nil {
        break
    }

    var msg ChatMessage
    if err := json.Unmarshal(data, &msg); err != nil {
        log.Printf("Invalid message format: %v", err)
        continue  // Ignore invalid messages
    }

    // Validate fields
    if msg.Type == "" || msg.Content == "" {
        log.Printf("Missing required fields")
        continue
    }

    // Sanitize content before broadcasting
    msg.Content = html.EscapeString(msg.Content)

    // Process validated message
    handleMessage(msg)
}
```

### Connection Rate Limiting

Protect against connection spam:

```go
// Per-IP connection limits
var connLimiter = make(map[string]*rate.Limiter)
var limitMu sync.Mutex

func getConnLimiter(ip string) *rate.Limiter {
    limitMu.Lock()
    defer limitMu.Unlock()

    limiter, exists := connLimiter[ip]
    if !exists {
        limiter = rate.NewLimiter(1, 5)  // 1 conn/sec, burst 5
        connLimiter[ip] = limiter
    }
    return limiter
}

http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    ip := r.RemoteAddr
    if !getConnLimiter(ip).Allow() {
        http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
        return
    }

    conn, _ := websocket.Upgrade(w, r, nil)
    // Handle connection...
})
```

### Error Handling

Never leak sensitive information:

```go
// ❌ BAD - Leaks internal details
conn, err := websocket.Upgrade(w, r, nil)
if err != nil {
    http.Error(w, err.Error(), 500)  // Leaks implementation details!
}

// ✅ GOOD - Generic error messages
conn, err := websocket.Upgrade(w, r, nil)
if err != nil {
    log.Printf("WebSocket upgrade failed: %v", err)  // Log internally
    http.Error(w, "Upgrade failed", http.StatusInternalServerError)
    return
}
```

### HTTPS/WSS Enforcement

Always use secure connections in production:

```go
// ✅ Require TLS for WebSocket connections
http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    if r.TLS == nil {
        http.Error(w, "HTTPS required", http.StatusUpgradeRequired)
        return
    }

    conn, _ := websocket.Upgrade(w, r, &websocket.UpgradeOptions{
        CheckOrigin: func(r *http.Request) bool {
            origin := r.Header.Get("Origin")
            return strings.HasPrefix(origin, "https://")
        },
    })
    // Handle connection...
})

// ✅ Use WSS (WebSocket Secure) URLs on client
// wss://yourdomain.com/ws (NOT ws://)
```

## Known Security Considerations

### 1. Connection Exhaustion

**Status**: Mitigated via configuration and best practices.

**Risk Level**: Medium

**Description**: Attackers can open thousands of connections to exhaust resources.

**Mitigation**:
- Configurable timeouts
- Frame size limits
- Hub channel buffer limits
- User responsibility to implement connection limits

### 2. Slow-Read/Write Attacks

**Status**: Mitigated via timeouts.

**Risk Level**: Medium

**Description**: Clients can deliberately read/write slowly to tie up resources.

**Mitigation**:
- Buffered channels prevent blocking
- 🔄 **User Responsibility**: Set appropriate timeouts

### 3. Fragment Reassembly DoS

**Status**: Mitigated in library.

**Risk Level**: Low

**Description**: Excessive fragmented frames can exhaust memory.

**Mitigation**:
- Max frame size enforced
- RFC 6455 compliant implementation

### 4. Dependency Security

stream library has **ZERO** external dependencies:

- ✅ Pure stdlib implementation (SSE and WebSocket)
- ✅ No supply chain attacks
- ✅ No dependency vulnerabilities
- ✅ No transitive dependencies
- ✅ Smaller attack surface

**Monitoring**:
- ✅ Go stdlib security updates tracked
- ✅ No Dependabot needed (zero deps!)

## Security Testing

### Current Testing

- ✅ Unit tests with malicious frame data (314 tests)
- ✅ Malformed handshake tests
- ✅ Invalid UTF-8 validation tests
- ✅ Frame injection tests
- ✅ Race detector (`go test -race`)
- ✅ Static analysis (`go vet`)
- ✅ 34+ golangci-lint linters (security-focused)

### Test Coverage

- **Overall**: 84.3% (314 tests)
- **SSE**: 92.3% coverage
- **WebSocket**: 84.3% coverage

### Planned for v1.0

- 🔄 Fuzzing with go-fuzz (frame parsing, handshakes)
- 🔄 Penetration testing of WebSocket endpoints
- 🔄 Load testing for DoS resilience
- 🔄 Memory profiling under attack scenarios

## Security Disclosure History

No security vulnerabilities have been reported or fixed yet (project is in 0.x development).

When vulnerabilities are addressed, they will be listed here with:
- **CVE ID** (if assigned)
- **Affected versions**
- **Fixed in version**
- **Severity** (Critical/High/Medium/Low)
- **Credit** to reporter

## Security Contact

- **GitHub Security Advisory**: https://github.com/coregx/stream/security/advisories/new
- **Public Issues** (for non-sensitive bugs): https://github.com/coregx/stream/issues
- **Discussions**: https://github.com/coregx/stream/discussions

## Bug Bounty Program

stream library does not currently have a bug bounty program. We rely on responsible disclosure from the security community.

If you report a valid security vulnerability:
- ✅ Public credit in security advisory (if desired)
- ✅ Acknowledgment in CHANGELOG
- ✅ Our gratitude and recognition in README
- ✅ Priority review and quick fix

---

**Thank you for helping keep stream secure!** 🔒

*Security is a journey, not a destination. We continuously improve our security posture with each release.*
