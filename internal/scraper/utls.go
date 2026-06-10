package scraper

import (
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
					return nil, err
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
		}
	}
	return uconn, nil
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
