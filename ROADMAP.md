# stream - Development Roadmap

> **Strategic Position**: Production-ready real-time communications for Go 1.25+
> **Approach**: Zero dependencies + RFC compliance + Performance = Ecosystem foundation

**Last Updated**: 2025-01-19 | **Current Version**: v0.1.0 | **Strategy**: Foundation → Integration → Ecosystem → LTS | **Milestone**: v0.1.0 RELEASED! (2025-01-19) → v1.0.0 (Q2 2025)

---

## 🎯 Vision

Build a **production-ready, zero-dependency real-time communications library** that provides SSE and WebSocket implementations for the coregx ecosystem and broader Go community.

### Key Advantages

✅ **Zero Dependencies**
- Pure stdlib implementation (no external dependencies in production)
- No vendor lock-in or dependency hell
- Simpler deployment and smaller binaries
- Works with `CGO_ENABLED=0` builds

✅ **RFC Compliance**
- SSE: [text/event-stream](https://html.spec.whatwg.org/multipage/server-sent-events.html) standard
- WebSocket: [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455) compliant
- Full browser and client compatibility
- Standards-based implementations

✅ **Production Ready**
- 84.3% test coverage (SSE: 92.3%, WebSocket: 84.3%)
- 314 comprehensive tests (9,245 lines)
- 23 benchmarks (E2E latency, throughput, load tests)
- Battle-tested in coregx ecosystem

---

## 🚀 Version Strategy

### Philosophy: Foundation → Integration → Ecosystem → Stable

```
v0.1.0 (FOUNDATION) ✅ RELEASED 2025-01-19
         ↓ (Both SSE + WebSocket production-ready!)
v0.2.0 (FURSY INTEGRATION) → Tight integration with fursy router
         ↓ (2-3 weeks)
v0.3.0 (ECOSYSTEM FEATURES) → Additional protocols, enhanced features
         ↓ (1-2 months)
v1.0.0 LTS → Long-term support with proven stability (Q2 2025)
```

### Critical Milestones

**v0.1.0** = Foundation ✅ RELEASED
- SSE implementation (92.3% coverage, 215 tests)
- WebSocket implementation (84.3% coverage, 99 tests)
- Broadcasting Hub pattern
- Comprehensive guides and examples
- Zero external dependencies
- Production-ready quality

**v0.2.0** = fursy Integration (planned)
- Native fursy router integration
- Type-safe handlers with generics
- Middleware for SSE/WebSocket routes
- OpenAPI 3.1 support for WebSocket endpoints
- Enhanced examples with fursy

**v0.3.0** = Ecosystem Features (planned)
- Additional real-time protocols (gRPC streaming, etc.)
- Advanced connection management
- Metrics and observability
- Performance optimizations
- Community-requested features

**v1.0.0** = Long-Term Support
- Proven in production (3+ months)
- API stability guarantee
- Semantic versioning strictly followed
- Community adoption and feedback incorporated
- Long-term support (2+ years)

**Why v0.1.0 (not alpha)?**: Both SSE and WebSocket implementations are production-ready with high test coverage, comprehensive documentation, and proven functionality. This is a stable foundation, not an experimental release.

**See**: [CHANGELOG.md](CHANGELOG.md) for complete release history

---

## 📊 Current Status (v0.1.0)

**Phase**: ✅ Foundation Complete
**SSE**: Production-ready! 92.3% coverage, RFC compliant! 🎉
**WebSocket**: Production-ready! 84.3% coverage, RFC 6455 compliant! ✨

**What Works**:
- ✅ **SSE (Server-Sent Events)**
  - RFC text/event-stream compliant
  - Event types, IDs, retry control
  - Automatic flushing
  - 92.3% test coverage (215 tests)

- ✅ **WebSocket**
  - RFC 6455 compliant
  - Text & Binary messages
  - Control frames (Ping/Pong, Close)
  - Broadcasting Hub pattern
  - 84.3% test coverage (99 tests)

- ✅ **Common Features**
  - Zero external dependencies
  - High performance (<100 μs broadcasts)
  - Comprehensive documentation (SSE_GUIDE.md, WEBSOCKET_GUIDE.md)
  - Working examples (6 examples total)
  - Modern Go 1.25+ with generics

**Example Usage**:
```bash
# SSE
go run examples/sse-basic/main.go
curl http://localhost:8080/events

# WebSocket
go run examples/websocket/echo-server/main.go
# Connect via browser WebSocket API
```

**Validation**:
- ✅ RFC compliance verified (SSE, WebSocket)
- ✅ Cross-platform tested (Linux, macOS, Windows)
- ✅ No external dependencies
- ✅ 314 tests passing (100% pass rate)
- ✅ Works with `CGO_ENABLED=0`

**Performance**:
- SSE: 23.4 μs/op (Send), 47.2 μs/op (Broadcast)
- WebSocket: 15.3 μs/op (Echo), 32.1 μs/op (Broadcast)
- Hub: 156 μs/op (1000 clients)
- Throughput: >20,000 messages/sec per connection

**History**: See [CHANGELOG.md](CHANGELOG.md) for complete release history

---

## 📅 What's Next

### **v0.2.0 - fursy Integration** (February 2025) [PLANNED]

**Goal**: Tight integration with fursy HTTP router

**Duration**: 2-3 weeks

**Planned Features**:
1. **Native Router Integration** (P1 - Critical)
   - Type-safe SSE/WebSocket handlers with generics
   - Middleware support for real-time routes
   - Route groups for SSE/WebSocket endpoints
   - Seamless fursy Context integration

2. **OpenAPI Support** (P1 - Critical)
   - OpenAPI 3.1 schemas for WebSocket endpoints
   - SSE endpoint documentation
   - Interactive API documentation

3. **Enhanced Examples** (P2 - Important)
   - Chat application with fursy
   - Real-time dashboard with metrics
   - Notification system with SSE
   - Production-ready patterns

4. **Documentation** (P2 - Important)
   - fursy integration guide
   - Migration from standalone to integrated
   - Best practices and patterns

**Quality Targets**:
- ✅ Maintain >80% test coverage
- ✅ Zero breaking changes to v0.1.0 API
- ✅ fursy examples fully functional
- ✅ OpenAPI generation working

**Target**: February 28, 2025

---

### **v0.3.0 - Ecosystem Features** (March-April 2025) [PLANNED]

**Goal**: Expand ecosystem and enhance features

**Duration**: 4-6 weeks

**Planned Work**:
1. **Additional Protocols** (P2 - Optional)
   - gRPC streaming support
   - HTTP/2 Server Push
   - Long polling fallback

2. **Advanced Features** (P2 - Optional)
   - Connection pooling and management
   - Automatic reconnection logic
   - Message compression
   - Binary protocol optimizations

3. **Observability** (P1 - Important)
   - Metrics (Prometheus format)
   - Distributed tracing (OpenTelemetry)
   - Health checks and status endpoints
   - Performance profiling

4. **Community Features** (P2 - Optional)
   - Community-requested features
   - Compatibility improvements
   - Performance optimizations

**Target**: April 30, 2025

---

### **v1.0.0 - Long-Term Support Release** (Q2 2025)

**Goal**: LTS release with proven stability

**Requirements**:
- v0.3.0 stable for 2+ months
- Positive community feedback
- Production deployments validated
- No critical bugs or security issues
- API proven and stable

**LTS Guarantees**:
- ✅ API stability (no breaking changes in v1.x.x)
- ✅ Long-term support (2+ years minimum)
- ✅ Semantic versioning strictly followed
- ✅ Security updates and bug fixes
- ✅ Performance improvements (non-breaking)

**Community Goals**:
- 500+ GitHub stars
- 10+ production deployments
- Active community discussions
- Third-party integrations

**Target**: June 2025 (after validation period)

---

## 📚 Resources

**Standards & Specifications**:
- SSE Spec: [WHATWG HTML Living Standard](https://html.spec.whatwg.org/multipage/server-sent-events.html)
- WebSocket Spec: [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455)
- WebSocket Protocol Guide: https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API

**Documentation**:
- [SSE Guide](docs/SSE_GUIDE.md) - Complete SSE documentation
- [WebSocket Guide](docs/WEBSOCKET_GUIDE.md) - Complete WebSocket documentation
- [API Reference](https://pkg.go.dev/github.com/coregx/stream) - pkg.go.dev documentation
- [Examples](examples/) - Working code examples

**Development**:
- CONTRIBUTING.md - How to contribute (coming soon)
- examples/ - Example programs (6 examples)
- Sister projects: [fursy](https://github.com/coregx/fursy), [relica](https://github.com/coregx/relica)

---

## 📞 Support

**Documentation**:
- README.md - Project overview and quick start
- docs/SSE_GUIDE.md - Complete SSE guide
- docs/WEBSOCKET_GUIDE.md - Complete WebSocket guide
- CHANGELOG.md - Release history

**Feedback**:
- GitHub Issues - Bug reports and feature requests
- GitHub Discussions - Questions and community help
- Pull Requests - Contributions welcome

---

## 🔬 Development Approach

**Pure Go Implementation**:
- Zero external dependencies (stdlib only)
- RFC compliance as foundation
- Go idioms and best practices
- Comprehensive testing (unit + integration + E2E)

**Quality Standards**:
- ✅ 80%+ test coverage minimum
- ✅ 90%+ for core protocol logic
- ✅ 100% test pass rate
- ✅ Zero golangci-lint issues
- ✅ Professional error messages
- ✅ Comprehensive documentation

**Community First**:
- Open development process
- Regular updates and releases
- Responsive to feedback and bug reports
- Collaborative with coregx ecosystem

---

## 🤝 Sister Projects

Part of the **coregx** ecosystem:

- **[fursy](https://github.com/coregx/fursy)** - HTTP Router with generics, OpenAPI, RFC 9457 (v0.2.0 integration target)
- **[relica](https://github.com/coregx/relica)** - Database Query Builder (coming soon)
- **[stream](https://github.com/coregx/stream)** - Real-time Communications (this library)

---

*Version 1.0 (Created 2025-01-19)*
*Current: v0.1.0 (RELEASED) | Phase: Foundation Complete | Next: v0.2.0 (fursy Integration) | Target: v1.0.0 LTS (Q2 2025)*
