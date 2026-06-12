package scraper

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	utls "github.com/refraction-networking/utls"
)

type chromeFingerprintTransport struct {
	*http.Transport
}

func NewChromeFingerprintTransport() http.RoundTripper {
	return &chromeFingerprintTransport{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialTLS: func(network, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}

				tcpConn, err := net.Dial(network, addr)
				if err != nil {
					return nil, err
				}

				uconn, err := setupChromeUConn(tcpConn, host)
				if err != nil {
					_ = tcpConn.Close()
					return nil, fmt.Errorf("uTLS setup for %s: %w", host, err)
				}

				if err := uconn.Handshake(); err != nil {
					_ = uconn.Close()
					return nil, err
				}

				return uconn, nil
			},
			ForceAttemptHTTP2: false,
			MaxIdleConns:      10,
			IdleConnTimeout:   90 * time.Second,
		},
	}
}

// setupChromeUConn creates a uTLS connection with Chrome's TLS fingerprint
// but strips h2 from the ALPN extension. Go's http.Transport uses a concrete
// type assertion (*crypto/tls.Conn) to detect TLS connections; utls.UConn
// doesn't satisfy it, so Go treats the connection as non-TLS and speaks
// HTTP/1.1. If ALPN negotiates h2, Cloudflare sends HTTP/2 frames but Go
// expects HTTP/1.1, producing "malformed HTTP response" errors. Stripping
// h2 forces HTTP/1.1-only while preserving the Chrome TLS fingerprint
// for WAF bypass.
func setupChromeUConn(tcpConn net.Conn, host string) (*utls.UConn, error) {
	uconn := utls.UClient(tcpConn, &utls.Config{ServerName: host}, utls.HelloChrome_Auto)
	if err := uconn.BuildHandshakeState(); err != nil {
		return nil, err
	}
	for _, ext := range uconn.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			filtered := make([]string, 0, len(alpn.AlpnProtocols))
			for _, p := range alpn.AlpnProtocols {
				if p != "h2" {
					filtered = append(filtered, p)
				}
			}
			alpn.AlpnProtocols = filtered
			break
		}
	}
	return uconn, nil
}

// resolveTransport returns the appropriate transport for the given TLS fingerprint.
// When fingerprint is empty, existing is returned unchanged.
// When fingerprint is set and existing is a *CachingTransport, the uTLS transport is
// set as the CachingTransport's Wrapped (mutating it).
// When fingerprint is set and existing is nil, a new Chrome fingerprint transport is returned.
// When fingerprint is set and existing is non-nil but not a *CachingTransport, the existing
// transport is preserved (caller's explicit choice takes precedence) — this case logs
// a warning via the provided logger that uTLS is being skipped.
func resolveTransport(fingerprint string, existing http.RoundTripper, logger *slog.Logger) http.RoundTripper {
	if fingerprint == "" {
		return existing
	}
	if ct, ok := existing.(*CachingTransport); ok {
		ct.Wrapped = NewChromeFingerprintTransport()
		return existing
	}
	if existing == nil {
		return NewChromeFingerprintTransport()
	}
	if logger != nil {
		logger.Warn("uTLS fingerprint set but existing transport is not a CachingTransport; skipping uTLS", "fingerprint", fingerprint, "transport_type", fmt.Sprintf("%T", existing))
	}
	return existing
}

// ChromeHeaders returns a set of HTTP headers that mimic a modern Chrome
// browser. The User-Agent version is intentionally generic (999) — the actual
// browser version is tracked by the TLS fingerprint (HelloChrome_Auto).
func ChromeHeaders() map[string]string {
	return map[string]string{
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/999.0.0.0 Safari/537.36",
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"Accept-Language": "en-US,en;q=0.9",
		"Sec-Fetch-Dest":  "document",
		"Sec-Fetch-Mode":  "navigate",
		"Sec-Fetch-Site":  "none",
		"Sec-Fetch-User":  "?1",
		"Cache-Control":   "no-cache",
	}
}
