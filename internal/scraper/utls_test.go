package scraper

import (
	"net"
	"regexp"
	"testing"

	utls "github.com/refraction-networking/utls"
)

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
