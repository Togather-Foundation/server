package scraper

import "testing"

func TestNewChromeFingerprintTransport(t *testing.T) {
	t.Parallel()

	tr := NewChromeFingerprintTransport(nil)

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

	if ct.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns = %d, want 10", ct.MaxIdleConns)
	}

	if ct.IdleConnTimeout == 0 {
		t.Error("IdleConnTimeout should be non-zero")
	}
}

func TestNewChromeFingerprintTransport_WithCookieJar(t *testing.T) {
	t.Parallel()

	jar := &fakeJar{}
	tr := NewChromeFingerprintTransport(jar)

	if tr == nil {
		t.Fatal("NewChromeFingerprintTransport returned nil")
	}

	ct, ok := tr.(*chromeFingerprintTransport)
	if !ok {
		t.Fatalf("expected *chromeFingerprintTransport, got %T", tr)
	}

	if ct.cookieJar != jar {
		t.Error("cookie jar not wired through")
	}
}

func TestNewChromeFingerprintTransport_NilJarAllowed(t *testing.T) {
	t.Parallel()

	tr := NewChromeFingerprintTransport(nil)

	if tr == nil {
		t.Fatal("NewChromeFingerprintTransport returned nil for nil jar")
	}

	ct := tr.(*chromeFingerprintTransport)
	if ct.cookieJar != nil {
		t.Error("expected nil cookie jar")
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
}
