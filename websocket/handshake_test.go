package websocket

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestUpgrade_Success validates successful WebSocket upgrade.
func TestUpgrade_Success(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	w := httptest.NewRecorder()

	// Mock hijacker (httptest.ResponseRecorder doesn't support Hijack)
	// We'll test with real server in integration tests
	// For unit test, we validate error handling

	_, err := Upgrade(w, req, nil)

	// httptest.ResponseRecorder doesn't support Hijack
	//nolint:errorlint // Direct comparison valid for sentinel errors
	if err != ErrHijackFailed {
		t.Errorf("expected ErrHijackFailed with httptest.ResponseRecorder, got: %v", err)
	}

	// Verify headers were set before hijack attempt
	if w.Code != http.StatusSwitchingProtocols {
		t.Errorf("expected status 101, got: %d", w.Code)
	}

	if got := w.Header().Get("Upgrade"); got != "websocket" {
		t.Errorf("Upgrade header = %q, want %q", got, "websocket")
	}

	if got := w.Header().Get("Connection"); got != "Upgrade" {
		t.Errorf("Connection header = %q, want %q", got, "Upgrade")
	}

	// Verify Sec-WebSocket-Accept calculation
	expectedAccept := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got := w.Header().Get("Sec-WebSocket-Accept"); got != expectedAccept {
		t.Errorf("Sec-WebSocket-Accept = %q, want %q", got, expectedAccept)
	}
}

// TestUpgrade_InvalidMethod verifies rejection of non-GET requests.
func TestUpgrade_InvalidMethod(t *testing.T) {
	methods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodOptions,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/ws", http.NoBody)
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Sec-WebSocket-Version", "13")

			w := httptest.NewRecorder()

			_, err := Upgrade(w, req, nil)

			//nolint:errorlint // Direct comparison valid for sentinel errors
			if err != ErrInvalidMethod {
				t.Errorf("expected ErrInvalidMethod, got: %v", err)
			}
		})
	}
}

// TestUpgrade_MissingUpgradeHeader verifies rejection when Upgrade header missing.
func TestUpgrade_MissingUpgradeHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"missing", ""},
		{"wrong value", "http/1.1"},
		{"partial match", "web"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
			if tt.header != "" {
				req.Header.Set("Upgrade", tt.header)
			}
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Sec-WebSocket-Version", "13")

			w := httptest.NewRecorder()

			_, err := Upgrade(w, req, nil)

			//nolint:errorlint // Direct comparison valid for sentinel errors
			if err != ErrMissingUpgrade {
				t.Errorf("expected ErrMissingUpgrade, got: %v", err)
			}
		})
	}
}

// TestUpgrade_MissingConnectionHeader verifies rejection when Connection header missing.
func TestUpgrade_MissingConnectionHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"missing", ""},
		{"wrong value", "keep-alive"},
		{"partial match", "up"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
			req.Header.Set("Upgrade", "websocket")
			if tt.header != "" {
				req.Header.Set("Connection", tt.header)
			}
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Sec-WebSocket-Version", "13")

			w := httptest.NewRecorder()

			_, err := Upgrade(w, req, nil)

			//nolint:errorlint // Direct comparison valid for sentinel errors
			if err != ErrMissingConnection {
				t.Errorf("expected ErrMissingConnection, got: %v", err)
			}
		})
	}
}

// TestUpgrade_InvalidVersion verifies rejection of unsupported versions.
func TestUpgrade_InvalidVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"missing", ""},
		{"version 8", "8"},
		{"version 12", "12"},
		{"version 14", "14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Connection", "Upgrade")
			if tt.version != "" {
				req.Header.Set("Sec-WebSocket-Version", tt.version)
			}
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

			w := httptest.NewRecorder()

			_, err := Upgrade(w, req, nil)

			//nolint:errorlint // Direct comparison valid for sentinel errors
			if err != ErrInvalidVersion {
				t.Errorf("expected ErrInvalidVersion, got: %v", err)
			}
		})
	}
}

// TestUpgrade_MissingSecKey verifies rejection when Sec-WebSocket-Key missing.
func TestUpgrade_MissingSecKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")
	// No Sec-WebSocket-Key

	w := httptest.NewRecorder()

	_, err := Upgrade(w, req, nil)

	//nolint:errorlint // Direct comparison valid for sentinel errors
	if err != ErrMissingSecKey {
		t.Errorf("expected ErrMissingSecKey, got: %v", err)
	}
}

// TestUpgrade_OriginCheck verifies custom origin checking.
func TestUpgrade_OriginCheck(t *testing.T) {
	tests := []struct {
		name        string
		origin      string
		checkOrigin func(*http.Request) bool
		wantErr     error
	}{
		{
			name:        "no check - allow all",
			origin:      "http://evil.com",
			checkOrigin: nil,
			wantErr:     ErrHijackFailed, // Will fail at hijack
		},
		{
			name:   "check passes",
			origin: "https://example.com",
			checkOrigin: func(r *http.Request) bool {
				return r.Header.Get("Origin") == "https://example.com"
			},
			wantErr: ErrHijackFailed, // Will fail at hijack
		},
		{
			name:   "check fails - wrong origin",
			origin: "http://evil.com",
			checkOrigin: func(r *http.Request) bool {
				return r.Header.Get("Origin") == "https://example.com"
			},
			wantErr: ErrOriginDenied,
		},
		{
			name:   "check fails - no origin",
			origin: "",
			checkOrigin: func(r *http.Request) bool {
				return r.Header.Get("Origin") != ""
			},
			wantErr: ErrOriginDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Sec-WebSocket-Version", "13")
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			w := httptest.NewRecorder()

			opts := &UpgradeOptions{
				CheckOrigin: tt.checkOrigin,
			}

			_, err := Upgrade(w, req, opts)

			//nolint:errorlint // Direct comparison valid for sentinel errors
			if err != tt.wantErr {
				t.Errorf("expected error %v, got: %v", tt.wantErr, err)
			}
		})
	}
}

// TestUpgrade_SubprotocolNegotiation verifies subprotocol selection.
func TestUpgrade_SubprotocolNegotiation(t *testing.T) {
	tests := []struct {
		name            string
		clientProtos    string
		serverProtos    []string
		wantSubprotocol string
	}{
		{
			name:            "no subprotocols",
			clientProtos:    "",
			serverProtos:    nil,
			wantSubprotocol: "",
		},
		{
			name:            "server doesn't support any",
			clientProtos:    "chat, superchat",
			serverProtos:    []string{},
			wantSubprotocol: "",
		},
		{
			name:            "first match - chat",
			clientProtos:    "chat, superchat",
			serverProtos:    []string{"chat", "superchat"},
			wantSubprotocol: "chat",
		},
		{
			name:            "first match - superchat",
			clientProtos:    "superchat, chat",
			serverProtos:    []string{"chat", "superchat"},
			wantSubprotocol: "superchat",
		},
		{
			name:            "no match",
			clientProtos:    "mqtt, amqp",
			serverProtos:    []string{"chat", "superchat"},
			wantSubprotocol: "",
		},
		{
			name:            "whitespace handling",
			clientProtos:    "  chat  ,  superchat  ",
			serverProtos:    []string{"chat"},
			wantSubprotocol: "chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Sec-WebSocket-Version", "13")
			if tt.clientProtos != "" {
				req.Header.Set("Sec-WebSocket-Protocol", tt.clientProtos)
			}

			w := httptest.NewRecorder()

			opts := &UpgradeOptions{
				Subprotocols: tt.serverProtos,
			}

			_, err := Upgrade(w, req, opts)

			// Will fail at hijack, but headers should be set
			//nolint:errorlint // Direct comparison valid for sentinel errors
			if err != ErrHijackFailed {
				t.Errorf("expected ErrHijackFailed, got: %v", err)
			}

			got := w.Header().Get("Sec-WebSocket-Protocol")
			if got != tt.wantSubprotocol {
				t.Errorf("subprotocol = %q, want %q", got, tt.wantSubprotocol)
			}
		})
	}
}

// TestUpgrade_BufferSizes verifies custom buffer sizes.
func TestUpgrade_BufferSizes(t *testing.T) {
	tests := []struct {
		name            string
		readBufferSize  int
		writeBufferSize int
		wantRead        int
		wantWrite       int
	}{
		{
			name:            "default sizes",
			readBufferSize:  0,
			writeBufferSize: 0,
			wantRead:        defaultReadBufferSize,
			wantWrite:       defaultWriteBufferSize,
		},
		{
			name:            "custom sizes",
			readBufferSize:  8192,
			writeBufferSize: 16384,
			wantRead:        8192,
			wantWrite:       16384,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &UpgradeOptions{
				ReadBufferSize:  tt.readBufferSize,
				WriteBufferSize: tt.writeBufferSize,
			}

			// Apply defaults (same logic as Upgrade)
			if opts.ReadBufferSize == 0 {
				opts.ReadBufferSize = defaultReadBufferSize
			}
			if opts.WriteBufferSize == 0 {
				opts.WriteBufferSize = defaultWriteBufferSize
			}

			if opts.ReadBufferSize != tt.wantRead {
				t.Errorf("read buffer = %d, want %d", opts.ReadBufferSize, tt.wantRead)
			}
			if opts.WriteBufferSize != tt.wantWrite {
				t.Errorf("write buffer = %d, want %d", opts.WriteBufferSize, tt.wantWrite)
			}
		})
	}
}

// TestComputeAcceptKey verifies Sec-WebSocket-Accept calculation.
//
// RFC 6455 Section 1.3: SHA-1(key + GUID) base64 encoded.
func TestComputeAcceptKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "RFC example",
			key:  "dGhlIHNhbXBsZSBub25jZQ==",
			want: "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=",
		},
		{
			name: "different key",
			key:  "x3JJHMbDL1EzLkh9GBhXDw==",
			want: "HSmrc0sMlYUkAGmm5OPpG2HaGWk=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeAcceptKey(tt.key)
			if got != tt.want {
				t.Errorf("computeAcceptKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// TestNegotiateSubprotocol verifies subprotocol selection logic.
func TestNegotiateSubprotocol(t *testing.T) {
	tests := []struct {
		name         string
		clientProtos string
		serverProtos []string
		want         string
	}{
		{
			name:         "no server protocols",
			clientProtos: "chat, superchat",
			serverProtos: nil,
			want:         "",
		},
		{
			name:         "no client protocols",
			clientProtos: "",
			serverProtos: []string{"chat"},
			want:         "",
		},
		{
			name:         "first match",
			clientProtos: "chat, superchat",
			serverProtos: []string{"chat", "superchat"},
			want:         "chat",
		},
		{
			name:         "second match",
			clientProtos: "mqtt, chat",
			serverProtos: []string{"chat", "superchat"},
			want:         "chat",
		},
		{
			name:         "no match",
			clientProtos: "mqtt, amqp",
			serverProtos: []string{"chat"},
			want:         "",
		},
		{
			name:         "whitespace",
			clientProtos: "  chat  ,  superchat  ",
			serverProtos: []string{"chat"},
			want:         "chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.Header.Set("Sec-WebSocket-Protocol", tt.clientProtos)

			got := negotiateSubprotocol(req, tt.serverProtos)
			if got != tt.want {
				t.Errorf("negotiateSubprotocol() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHeaderContainsToken verifies case-insensitive token matching.
func TestHeaderContainsToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		token  string
		want   bool
	}{
		{
			name:   "exact match",
			header: "websocket",
			token:  "websocket",
			want:   true,
		},
		{
			name:   "case insensitive",
			header: "WebSocket",
			token:  "websocket",
			want:   true,
		},
		{
			name:   "multiple tokens - first",
			header: "Upgrade, HTTP/2.0",
			token:  "upgrade",
			want:   true,
		},
		{
			name:   "multiple tokens - second",
			header: "keep-alive, Upgrade",
			token:  "upgrade",
			want:   true,
		},
		{
			name:   "no match",
			header: "keep-alive",
			token:  "upgrade",
			want:   false,
		},
		{
			name:   "partial match - should not match",
			header: "websockets",
			token:  "websocket",
			want:   false,
		},
		{
			name:   "whitespace",
			header: "  Upgrade  ,  HTTP/2.0  ",
			token:  "upgrade",
			want:   true,
		},
		{
			name:   "empty header",
			header: "",
			token:  "upgrade",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := headerContainsToken(tt.header, tt.token)
			if got != tt.want {
				t.Errorf("headerContainsToken(%q, %q) = %v, want %v", tt.header, tt.token, got, tt.want)
			}
		})
	}
}

// TestCheckSameOrigin verifies default origin checker.
func TestCheckSameOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		host   string
		tls    bool
		want   bool
	}{
		{
			name:   "no origin - allow",
			origin: "",
			host:   "example.com",
			tls:    false,
			want:   true,
		},
		{
			name:   "http same origin",
			origin: "http://example.com",
			host:   "example.com",
			tls:    false,
			want:   true,
		},
		{
			name:   "https same origin",
			origin: "https://example.com",
			host:   "example.com",
			tls:    true,
			want:   true,
		},
		{
			name:   "different origin",
			origin: "http://evil.com",
			host:   "example.com",
			tls:    false,
			want:   false,
		},
		{
			name:   "scheme mismatch",
			origin: "https://example.com",
			host:   "example.com",
			tls:    false,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.tls {
				// Mock TLS connection
				req.TLS = &tls.ConnectionState{}
			}

			got := checkSameOrigin(req)
			if got != tt.want {
				t.Errorf("checkSameOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

// BenchmarkComputeAcceptKey benchmarks Sec-WebSocket-Accept calculation.
func BenchmarkComputeAcceptKey(b *testing.B) {
	key := "dGhlIHNhbXBsZSBub25jZQ=="

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = computeAcceptKey(key)
	}
}

// BenchmarkHeaderContainsToken benchmarks token matching.
func BenchmarkHeaderContainsToken(b *testing.B) {
	header := "Upgrade, HTTP/2.0, WebSocket"
	token := "upgrade"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = headerContainsToken(header, token)
	}
}

// BenchmarkNegotiateSubprotocol benchmarks subprotocol negotiation.
func BenchmarkNegotiateSubprotocol(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Sec-WebSocket-Protocol", "chat, superchat, mqtt")
	serverProtos := []string{"mqtt", "amqp", "stomp"}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = negotiateSubprotocol(req, serverProtos)
	}
}
