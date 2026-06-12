package scraper

import (
	"bytes"
	"net"
	"net/http"
	"regexp"
	"testing"

	utls "github.com/refraction-networking/utls"
	"github.com/rs/zerolog"
)

func TestResolveTransport(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	defaultTransport := http.DefaultTransport

	tests := []struct {
		name        string
		fingerprint string
		existing    http.RoundTripper
		wantNil     bool
		wantSame    bool
		wantWrapped bool
		wantLogMsg  string
	}{
		{
			name:        "empty fingerprint, nil transport",
			fingerprint: "",
			existing:    nil,
			wantNil:     true,
		},
		{
			name:        "empty fingerprint, CachingTransport",
			fingerprint: "",
			existing:    NewCachingTransport(nil, tempDir, false, zerolog.Nop()),
			wantSame:    true,
		},
		{
			name:        "chrome_auto fingerprint, nil transport",
			fingerprint: "chrome_auto",
			existing:    nil,
			wantNil:     false,
		},
		{
			name:        "chrome_auto fingerprint, CachingTransport",
			fingerprint: "chrome_auto",
			existing:    NewCachingTransport(nil, tempDir, false, zerolog.Nop()),
			wantWrapped: true,
		},
		{
			name:        "chrome_auto fingerprint, non-CachingTransport",
			fingerprint: "chrome_auto",
			existing:    defaultTransport,
			wantSame:    true,
			wantLogMsg:  "uTLS fingerprint set but existing transport is not a CachingTransport",
		},
		{
			name:        "chrome_auto fingerprint, non-CachingTransport, nil logger",
			fingerprint: "chrome_auto",
			existing:    defaultTransport,
			wantSame:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBuf bytes.Buffer
			var logger zerolog.Logger
			if tt.wantLogMsg != "" {
				logger = zerolog.New(&logBuf)
			}

			got := resolveTransport(tt.fingerprint, tt.existing, logger)

			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %T", got)
				}
				return
			}

			if tt.wantSame {
				if got != tt.existing {
					t.Errorf("expected same transport %T, got %T", tt.existing, got)
				}
				if tt.wantLogMsg != "" {
					if !bytes.Contains(logBuf.Bytes(), []byte(tt.wantLogMsg)) {
						t.Errorf("expected log message %q, got %s", tt.wantLogMsg, logBuf.String())
					}
				}
				return
			}

			if tt.wantWrapped {
				ct, ok := got.(*CachingTransport)
				if !ok {
					t.Fatalf("expected *CachingTransport, got %T", got)
				}
				if ct == tt.existing {
					t.Error("expected cloned CachingTransport, got same pointer")
				}
				if ct.Wrapped == nil {
					t.Fatal("expected CachingTransport.Wrapped to be set to uTLS transport, got nil")
				}
				if _, ok := ct.Wrapped.(*chromeFingerprintTransport); !ok {
					t.Errorf("expected Wrapped to be *chromeFingerprintTransport, got %T", ct.Wrapped)
				}
				// Verify original CachingTransport was not mutated
				orig, ok := tt.existing.(*CachingTransport)
				if !ok {
					t.Fatalf("expected existing to be *CachingTransport, got %T", tt.existing)
				}
				if orig.Wrapped != nil {
					t.Errorf("original CachingTransport.Wrapped was mutated, got %T", orig.Wrapped)
				}
				return
			}

			// Non-nil result, verify it's a uTLS transport
			if _, ok := got.(*chromeFingerprintTransport); !ok {
				t.Errorf("expected *chromeFingerprintTransport, got %T", got)
			}
		})
	}
}

func TestNewChromeFingerprintTransport(t *testing.T) {
	t.Parallel()

	tr := NewChromeFingerprintTransport()

	if tr == nil {
		t.Fatal("NewChromeFingerprintTransport returned nil")
	}

	ct, ok := tr.(*chromeFingerprintTransport)
	if !ok {
		t.Fatalf("expected *chromeFingerprintTransport, got %T", tr)
	}

	if ct.Transport == nil {
		t.Fatal("embedded Transport is nil")
	}

	if ct.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be false to avoid uTLS HTTP/2 protocol mismatch on Cloudflare-protected sites")
	}

	if ct.MaxIdleConns <= 0 {
		t.Errorf("MaxIdleConns = %d, want > 0", ct.MaxIdleConns)
	}

	if ct.IdleConnTimeout == 0 {
		t.Error("IdleConnTimeout should be non-zero")
	}
}

func TestChromeFingerprintNoHTTP2ALPN(t *testing.T) {
	t.Parallel()

	_, clientConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()

	uconn, err := setupChromeUConn(clientConn, "example.com")
	if err != nil {
		t.Fatalf("setupChromeUConn failed: %v", err)
	}
	defer func() { _ = uconn.Close() }()

	var alpnExt *utls.ALPNExtension
	for _, ext := range uconn.Extensions {
		if a, ok := ext.(*utls.ALPNExtension); ok {
			alpnExt = a
			break
		}
	}
	if alpnExt == nil {
		t.Fatal("no ALPN extension found in Chrome fingerprint extensions")
	}

	for _, proto := range alpnExt.AlpnProtocols {
		if proto == "h2" {
			t.Error("ALPN protocols should not contain h2 (Cloudflare HTTP/2 protocol mismatch)")
		}
	}

	hasHTTP1 := false
	for _, proto := range alpnExt.AlpnProtocols {
		if proto == "http/1.1" {
			hasHTTP1 = true
			break
		}
	}
	if !hasHTTP1 {
		t.Error("ALPN protocols should contain http/1.1")
	}
}

func TestSetupChromeUConn_BuildHandshakeStateError(t *testing.T) {
	t.Parallel()

	_, clientConn := net.Pipe()

	uconn, err := setupChromeUConn(clientConn, "")
	if uconn != nil {
		_ = uconn.Close()
		t.Error("expected nil uconn when BuildHandshakeState fails")
	}
	if err == nil {
		t.Error("expected non-nil error from BuildHandshakeState")
	}
}

func TestChromeHeaders(t *testing.T) {
	t.Parallel()

	headers := ChromeHeaders()

	requiredKeys := []string{
		"User-Agent",
		"Accept",
		"Accept-Language",
		"Sec-Fetch-Dest",
		"Sec-Fetch-Mode",
		"Sec-Fetch-Site",
		"Sec-Fetch-User",
		"Cache-Control",
	}

	for _, key := range requiredKeys {
		t.Run(key, func(t *testing.T) {
			val, ok := headers[key]
			if !ok {
				t.Errorf("missing required header key: %s", key)
				return
			}
			if val == "" {
				t.Errorf("header %s has empty value", key)
			}
		})
	}

	ua := headers["User-Agent"]
	chromePattern := regexp.MustCompile(`Chrome/\d+`)
	if !chromePattern.MatchString(ua) {
		t.Errorf("User-Agent does not contain Chrome/<version>: %s", ua)
	}
}
