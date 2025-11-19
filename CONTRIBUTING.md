# Contributing to Stream Library

Thank you for considering contributing to Stream! This document outlines the development workflow and guidelines.

## Git Workflow (Git-Flow)

This project uses Git-Flow branching model for development.

### Branch Structure

```
main                 # Production-ready code (tagged releases)
  └─ develop         # Integration branch for next release
       ├─ feature/*  # New features
       ├─ bugfix/*   # Bug fixes
       └─ hotfix/*   # Critical fixes from main
```

### Branch Purposes

- **main**: Production-ready code. Only releases are merged here.
- **develop**: Active development branch. All features merge here first.
- **feature/\***: New features. Branch from `develop`, merge back to `develop`.
- **bugfix/\***: Bug fixes. Branch from `develop`, merge back to `develop`.
- **hotfix/\***: Critical production fixes. Branch from `main`, merge to both `main` and `develop`.

### Workflow Commands

#### Starting a New Feature

```bash
# Create feature branch from develop
git checkout develop
git pull origin develop
git checkout -b feature/my-new-feature

# Work on your feature...
git add .
git commit -m "feat: add my new feature"

# When done, merge back to develop
git checkout develop
git merge --no-ff feature/my-new-feature
git branch -d feature/my-new-feature
git push origin develop
```

#### Fixing a Bug

```bash
# Create bugfix branch from develop
git checkout develop
git pull origin develop
git checkout -b bugfix/fix-issue-123

# Fix the bug...
git add .
git commit -m "fix: resolve issue #123"

# Merge back to develop
git checkout develop
git merge --no-ff bugfix/fix-issue-123
git branch -d bugfix/fix-issue-123
git push origin develop
```

#### Creating a Release

```bash
# Create release branch from develop
git checkout develop
git pull origin develop
git checkout -b release/v0.4.0

# Update version numbers, CHANGELOG, etc.
git add .
git commit -m "chore: prepare release v0.4.0"

# Merge to main and tag
git checkout main
git merge --no-ff release/v0.4.0
git tag -a v0.4.0 -m "Release v0.4.0"

# Merge back to develop
git checkout develop
git merge --no-ff release/v0.4.0

# Delete release branch
git branch -d release/v0.4.0

# Push everything
git push origin main develop --tags
```

#### Hotfix (Critical Production Bug)

```bash
# Create hotfix branch from main
git checkout main
git pull origin main
git checkout -b hotfix/critical-bug

# Fix the bug...
git add .
git commit -m "fix: critical production bug"

# Merge to main and tag
git checkout main
git merge --no-ff hotfix/critical-bug
git tag -a v0.3.1 -m "Hotfix v0.3.1"

# Merge to develop
git checkout develop
git merge --no-ff hotfix/critical-bug

# Delete hotfix branch
git branch -d hotfix/critical-bug

# Push everything
git push origin main develop --tags
```

## Semantic Versioning

Stream follows [Semantic Versioning 2.0.0](https://semver.org/):

### For 0.x.y versions (pre-1.0):
- **0.y.0** - New features (minor bump)
- **0.y.z** - Bug fixes, hotfixes (patch bump)

### For 1.x.y+ versions (stable API):
- **Major (x.0.0)** - Breaking changes
- **Minor (x.y.0)** - New features (backwards-compatible)
- **Patch (x.y.z)** - Bug fixes only

**Note**: v1.0.0 will only be released after 6-12 months of production usage and full API stabilization. Breaking changes are allowed in 0.x versions.

## Commit Message Guidelines

Follow [Conventional Commits](https://www.conventionalcommits.org/) specification:

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

### Types

- **feat**: New feature
- **fix**: Bug fix
- **docs**: Documentation changes
- **style**: Code style changes (formatting, etc.)
- **refactor**: Code refactoring
- **test**: Adding or updating tests
- **chore**: Maintenance tasks (build, dependencies, etc.)
- **perf**: Performance improvements

### Examples

```bash
feat(sse): add event retry mechanism
fix(websocket): resolve frame fragmentation edge case
docs: update README with hub broadcasting examples
refactor(websocket): simplify handshake validation logic
test(sse): add benchmarks for concurrent broadcasts
perf(websocket): optimize frame masking performance
chore: update golangci-lint to v1.60
```

## Code Quality Standards

### Before Committing

Run the pre-commit checks:

```bash
bash scripts/pre-release-check.sh
```

This script runs:
1. `go fmt ./...` - Format code
2. `golangci-lint run` - Lint code
3. `go test -race -coverprofile=coverage.txt ./...` - Run tests with race detector
4. Coverage check (>80% target, current: 84.3%)

### Pull Request Requirements

- [ ] Code is formatted (`go fmt ./...`)
- [ ] Linter passes (`golangci-lint run`)
- [ ] All tests pass with race detector (`go test -race ./...`)
- [ ] New code has tests (minimum 80% coverage, target: 85%+)
- [ ] Benchmarks for performance-critical code
- [ ] Documentation updated (if applicable)
- [ ] Commit messages follow Conventional Commits
- [ ] No sensitive data (credentials, tokens, etc.)
- [ ] Uses `encoding/json/v2` (NOT `encoding/json`)
- [ ] Uses `log/slog` for logging
- [ ] No external dependencies (pure stdlib)

## Development Setup

### Prerequisites

- **Go 1.25 or later** (required for generics and modern stdlib)
- **golangci-lint** (for code quality checks)
- **git** (for version control)

### Install Dependencies

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Verify installation
golangci-lint --version
```

### Running Tests

```bash
# Run all tests
go test -v ./...

# Run with coverage
go test -v -coverprofile=coverage.txt ./...

# Run with race detector (always use before commit!)
go test -race ./...

# Run benchmarks
go test -bench=. -benchmem ./...

# Run specific benchmark
go test -bench=BenchmarkHub_Broadcast -benchmem ./...
```

### Running Linter

```bash
# Run linter
golangci-lint run

# Run with verbose output
golangci-lint run -v

# Run and save report
golangci-lint run --out-format=colored-line-number > lint-report.txt
```

## Project Structure

```
stream/
├── .golangci.yml         # Linter configuration
├── .github/
│   ├── workflows/        # CI/CD pipelines
│   └── CODEOWNERS        # Code ownership
├── docs/                 # Public documentation
│   ├── SSE_GUIDE.md
│   └── WEBSOCKET_GUIDE.md
├── examples/             # Usage examples
│   ├── sse/
│   │   ├── basic/
│   │   ├── hub/
│   │   └── events/
│   └── websocket/
│       ├── echo/
│       ├── chat/
│       └── broadcast/
├── sse/                  # Server-Sent Events implementation
│   ├── conn.go          # SSE connection
│   ├── event.go         # Event structures
│   ├── hub.go           # Broadcasting hub
│   └── *_test.go        # Tests and benchmarks
├── websocket/            # WebSocket RFC 6455 implementation
│   ├── conn.go          # WebSocket connection
│   ├── frame.go         # Frame parsing/encoding
│   ├── hub.go           # Broadcasting hub
│   ├── message.go       # Message handling
│   └── *_test.go        # Tests and benchmarks
├── scripts/              # Development scripts
│   └── pre-release-check.sh
├── CONTRIBUTING.md       # This file
├── SECURITY.md           # Security policy
├── README.md             # Main documentation
├── CHANGELOG.md          # Version history
└── go.mod                # Go module (zero dependencies!)
```

## Architecture Principles

### Clean Public API

Stream provides two independent packages with clean, focused APIs:

```
github.com/coregx/stream/sse/          ← SSE (Server-Sent Events)
├── conn.go                            ← Connection handling
├── event.go                           ← Event structures
└── hub.go                             ← Broadcasting

github.com/coregx/stream/websocket/    ← WebSocket RFC 6455
├── conn.go                            ← Connection handling
├── frame.go                           ← Frame parsing
├── hub.go                             ← Broadcasting
└── message.go                         ← Message types
```

**Design Goals:**
- Pure stdlib implementation (zero dependencies)
- RFC-compliant implementations (SSE: text/event-stream, WebSocket: RFC 6455)
- Simple, intuitive APIs
- High performance with minimal allocations

### Zero Dependencies

- **ALL packages** use ONLY stdlib
- No external dependencies whatsoever
- Pure Go implementation
- Never add external dependencies without discussion

### Type Safety

Use Go 1.25+ features for type-safe APIs:

```go
// SSE type-safe event building
event := sse.NewEvent("data").
    WithType("message").
    WithID("123").
    WithRetry(5000)

// WebSocket type-safe message handling
conn.WriteText([]byte("hello"))
conn.WriteJSON(myStruct)
```

## Adding New Features

1. Check if issue exists, if not create one
2. Discuss approach in the issue
3. Create feature branch from `develop`
4. Write tests FIRST (TDD approach)
5. Implement feature
6. Add benchmarks for performance-critical code
7. Update documentation
8. Run quality checks (`bash scripts/pre-release-check.sh`)
9. Create pull request to `develop`
10. Wait for code review
11. Address feedback
12. Merge when approved

## Code Style Guidelines

### General Principles

- Follow Go conventions and idioms
- Write self-documenting code
- Add comments for complex logic (especially in frame parsing and handshakes)
- Keep functions small and focused (<50 lines ideal)
- Use meaningful variable names
- **TDD approach** - write tests first!

### Naming Conventions

- **Public types/functions**: `PascalCase` (e.g., `Conn`, `Upgrade`, `Hub`)
- **Private types/functions**: `camelCase` (e.g., `parseFrame`, `computeAcceptKey`)
- **Constants**: `PascalCase` (e.g., `OpText`, `OpBinary`, `CloseNormalClosure`)
- **Test functions**: `Test*` (e.g., `TestConn_Send`, `TestWebSocket_Handshake`)
- **Benchmark functions**: `Benchmark*` (e.g., `BenchmarkHub_Broadcast`)

### Required Standards

#### 1. Use encoding/json/v2

```go
// ✅ CORRECT
import "encoding/json/v2"

// ❌ WRONG - Do NOT use old version
import "encoding/json"
```

#### 2. Use log/slog

```go
import "log/slog"

// Structured logging
slog.Info("request processed",
    "method", req.Method,
    "path", req.URL.Path,
    "duration", duration,
)
```

#### 3. Error Handling

```go
// Always return errors, don't panic
conn, err := sse.Upgrade(w, r)
if err != nil {
    http.Error(w, "Upgrade failed", http.StatusInternalServerError)
    return
}

// Use standard error types
if errors.Is(err, websocket.ErrHandshakeFailed) {
    // Handle handshake error
}
```

### Testing

- Use table-driven tests when appropriate
- Test both success and error cases
- Use `testing.T.Run()` for subtests
- **Minimum coverage**: 80% overall, 85%+ target (current: 84.3%)
- Always run with race detector: `go test -race`

### Benchmarking

Performance is a core goal. Always benchmark critical paths:

```go
func BenchmarkHub_Broadcast(b *testing.B) {
    hub := websocket.NewHub()
    go hub.Run()
    defer hub.Close()

    // Setup clients...
    for i := 0; i < 10; i++ {
        conn := setupTestConn()
        hub.Register(conn)
    }

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        hub.Broadcast(websocket.OpText, []byte("test"))
    }
}
```

**Performance targets**:
- SSE event send: <500 ns/op
- WebSocket frame read/write: <10 μs/op
- Hub broadcast (10 clients): <10 μs/op
- Minimal allocations (<5 allocs/op for typical operations)

## Getting Help

- Check [existing issues](https://github.com/coregx/stream/issues)
- Read documentation in `docs/` (SSE_GUIDE.md, WEBSOCKET_GUIDE.md)
- Review examples in `examples/`
- Ask questions in GitHub Issues
- Check CHANGELOG.md for version history

## Performance Benchmarking

Stream prioritizes performance and low overhead. Current benchmarks:

**SSE Performance**:
- Event send: ~509 ns/op, 5 allocs/op
- Event serialization: ~643 ns/op
- Hub broadcast (10 clients): ~5.4 μs/op
- Large event (1MB): ~100 ms/op @ 10.47 MB/s

**WebSocket Performance**:
- Frame read (small): ~7.3 μs/op
- Frame write (small): ~3.8 μs/op
- Hub broadcast (10 clients): ~7.5 μs/op
- E2E roundtrip: ~131 μs/op

**Test Coverage**: 84.3% (314 tests, 37 benchmarks)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

---

**Thank you for contributing to Stream!** 🚀
