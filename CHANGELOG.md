# Changelog

All notable changes to the stream library will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Integration tests for cross-feature scenarios
- fursy HTTP router integration examples

## [0.1.0] - 2025-01-18

### Added - WebSocket Support (Week 3-6)

**Core Implementation:**
- RFC 6455 compliant WebSocket protocol implementation
- `websocket.Conn` - WebSocket connection with Read/Write API
- `websocket.Upgrade()` - HTTP to WebSocket handshake (Section 4)
- Frame parsing and validation (Section 5)
- Fragment reassembly for multi-frame messages
- UTF-8 validation for text frames (Section 8.1)
- Automatic Ping/Pong handling (Section 5.5)
- Close handshake with 15 standard close codes (Section 7.4)

**Connection API:**
- `conn.Read()` - Read text/binary messages with auto-fragmentation
- `conn.Write()` - Write text/binary messages (thread-safe)
- `conn.ReadText()` / `conn.WriteText()` - Convenience text helpers
- `conn.ReadJSON()` / `conn.WriteJSON()` - Type-safe JSON serialization
- `conn.Ping()` / `conn.Pong()` - Keep-alive control frames
- `conn.Close()` / `conn.CloseWithCode()` - Graceful connection termination

**Broadcasting:**
- `websocket.Hub` - Multi-client broadcast manager
- `hub.Register()` / `hub.Unregister()` - Client lifecycle management
- `hub.Broadcast()` - Binary message broadcasting
- `hub.BroadcastText()` - Text message broadcasting
- `hub.BroadcastJSON()` - JSON message broadcasting
- `hub.ClientCount()` - Active connection count
- Thread-safe concurrent operations
- Automatic cleanup on client write failures

**Configuration:**
- `UpgradeOptions` - Customize handshake behavior
- Subprotocol negotiation (Section 1.9)
- Origin validation via `CheckOrigin` callback
- Configurable read/write buffer sizes (default 4096 bytes)

**Message Types:**
- `TextMessage` (opcode 0x1) - UTF-8 encoded text
- `BinaryMessage` (opcode 0x2) - Arbitrary binary data
- Automatic message type preservation in echo patterns

**Close Codes (RFC 6455 Section 7.4):**
- `CloseNormalClosure` (1000) - Normal connection closure
- `CloseGoingAway` (1001) - Server/client shutdown
- `CloseProtocolError` (1002) - Protocol violation
- `CloseUnsupportedData` (1003) - Unsupported data type
- `CloseInvalidFramePayloadData` (1007) - Invalid UTF-8 or data
- `ClosePolicyViolation` (1008) - Generic policy error
- `CloseMessageTooBig` (1009) - Message size limit exceeded
- `CloseInternalServerErr` (1011) - Internal server error
- `CloseServiceRestart` (1012) - Service restart
- `CloseTryAgainLater` (1013) - Temporary unavailability
- Plus reserved codes: 1005, 1006, 1015

**Error Handling:**
- `IsCloseError()` - Detect clean connection closure
- `IsTemporaryError()` - Identify retry-able network errors
- `ErrClosed` - Connection already closed
- `ErrInvalidUTF8` - Invalid UTF-8 in text message
- `ErrInvalidMessageType` - Unknown message type
- `ErrControlTooLarge` - Control frame > 125 bytes
- Handshake errors: `ErrInvalidMethod`, `ErrMissingUpgrade`, etc.

**Testing:**
- 99 WebSocket tests - 84.3% coverage
  - 11 Core tests (conn, hub, frame, handshake)
  - 4 Integration tests (cross-feature SSE ↔ WebSocket, Hub echo, subprotocols)
  - 5 Load tests (100 concurrent clients, Hub broadcasting, rapid messages)
  - 4 Stress tests (10MB messages, rapid connect/disconnect, concurrent broadcast, memory pressure)
  - 9 Benchmarks (Hub, E2E roundtrip, large messages, parallel clients)
- Test files (5,248 lines):
  - `websocket/conn_test.go` - Connection API tests
  - `websocket/hub_test.go` - Hub broadcasting tests (100% coverage)
  - `websocket/frame_test.go` - Frame parsing tests
  - `websocket/handshake_test.go` - Upgrade handshake tests
  - `websocket/integration_test.go` - RFC validation integration tests
  - `websocket/cross_feature_test.go` - SSE+WebSocket integration tests (4 tests)
  - `websocket/load_test.go` - Load testing (5 tests, 100+ concurrent clients)
  - `websocket/stress_test.go` - Stress testing (4 tests, 10MB messages)
  - `websocket/hub_bench_test.go` - Performance benchmarks (9 benchmarks)
- Key benchmarks:
  - `BenchmarkHub_Broadcast_10clients` - 11 μs/op (9x better than 100μs target)
  - `BenchmarkHub_Broadcast_100clients` - 75 μs/op (~13K broadcasts/s)
  - `BenchmarkE2E_WebSocket_Roundtrip` - 58 μs/op (full roundtrip latency)
  - `BenchmarkE2E_LargeMessage` - 4.7 ms for 1MB message (223 MB/s throughput)
  - `BenchmarkE2E_ParallelClients` - 22 μs/op (parallel client stress test)

**Examples:**
- `examples/websocket/echo-server/` - Simple echo server (Read/Write loop)
- `examples/websocket/chat-server/` - Multi-client chat with Hub and JSON
  - Beautiful HTML/CSS/JS web client
  - Join/leave notifications
  - Real-time message delivery
  - Username support via query params
- `examples/websocket/ping-pong/` - Keep-alive demonstration
  - Periodic Ping every 30 seconds
  - Automatic Pong responses
  - Connection health monitoring

**Documentation:**
- `docs/WEBSOCKET_GUIDE.md` - Comprehensive 600-line guide
  - Introduction and RFC 6455 compliance
  - Quick Start examples
  - Complete Connection API reference
  - Message Types and UTF-8 validation
  - Broadcasting with Hub architecture
  - Control Frames (Ping/Pong/Close)
  - Best Practices (security, error handling, goroutines)
  - Performance benchmarks and optimization tips
  - Production deployment (TLS, auth, load balancing, monitoring)
  - Troubleshooting guide
- README.md updated with WebSocket Quick Start
- All public APIs have comprehensive godoc comments

### Added - SSE Support (Week 1-2)

**Core Implementation:**
- RFC text/event-stream compliant SSE implementation
- `sse.Event` - Event builder with fluent API
- `sse.Conn` - SSE connection management
- `sse.Upgrade()` - HTTP to SSE upgrade
- Generic `sse.Hub[T]` for type-safe broadcasting
- Automatic heartbeat (keep-alive) support
- Connection lifecycle management

**Testing:**
- 215 SSE tests - 92.3% coverage
- Comprehensive event parsing tests
- Hub broadcasting tests
- Integration tests
- Performance benchmarks

**Examples:**
- `examples/sse-basic/` - Simple time update stream
- `examples/sse-chat/` - Multi-client chat with Hub

**Documentation:**
- `docs/SSE_GUIDE.md` - Complete SSE documentation
- README.md with Quick Start examples

### Performance

**WebSocket:**
- Hub broadcast (10 clients): 11 μs/op
- Hub broadcast (100 clients): 75 μs/op
- Throughput: ~13K broadcasts/s (100 clients)
- Near-zero allocation routing (1 alloc/op)

**SSE:**
- Hub broadcast (10 clients): 8 μs/op
- Hub broadcast (100 clients): 52 μs/op

### Technical Details

**Dependencies:**
- Zero external dependencies (production)
- Pure stdlib: `net/http`, `bufio`, `encoding/json/v2`, `crypto/sha1`
- Go 1.25+ required

**Architecture:**
- Clean separation: `sse/` and `websocket/` packages
- Thread-safe Hub pattern for both SSE and WebSocket
- Reusable buffer management
- Context-aware connection handling

**Code Quality:**
- **314 total tests** (215 SSE + 99 WebSocket) - **9,245 lines of test code**
- **84.3% overall coverage** (SSE: 92.3%, WebSocket: 84.3%)
- **23 benchmarks** (4 SSE + 19 WebSocket) with comprehensive performance metrics
- Minimal linter warnings (cognitive complexity in complex integration tests only)
- Production-ready error handling
- **Load tested**: 100 concurrent WebSocket connections, 1,000 broadcasts
- **Stress tested**: 10MB messages, rapid connect/disconnect cycles, memory pressure

---

## Version History

- **v0.1.0** (2025-01-18) - Initial release with SSE and WebSocket support
- **v0.0.1** (2025-01-11) - Repository setup and project structure

---

## Upcoming

### v0.2.0 (Q1 2025)
- fursy HTTP router integration
- Middleware examples (auth, rate limiting)
- Production hardening (TLS helpers, metrics)
- Client reconnection strategies
- Message compression (permessage-deflate)

### v1.0.0 (Q2 2025)
- Stable API guarantee
- Full production deployment guide
- Load testing benchmarks
- Kubernetes deployment examples

---

**Links:**
- [GitHub Repository](https://github.com/coregx/stream)
- [API Documentation](https://pkg.go.dev/github.com/coregx/stream)
- [SSE Guide](docs/SSE_GUIDE.md)
- [WebSocket Guide](docs/WEBSOCKET_GUIDE.md)
