package scraper

import (
	"regexp"
	"testing"
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

	if !ct.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be true")
	}

	if ct.MaxIdleConns <= 0 {
		t.Errorf("MaxIdleConns = %d, want > 0", ct.MaxIdleConns)
	}

	if ct.IdleConnTimeout == 0 {
		t.Error("IdleConnTimeout should be non-zero")
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
