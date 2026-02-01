package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders_AllHeaders(t *testing.T) {
	handler := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	tests := []struct {
		header   string
		expected string
	}{
		{"X-Frame-Options", "DENY"},
		{"X-Content-Type-Options", "nosniff"},
		{"X-XSS-Protection", "1; mode=block"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'"},
	}

	for _, tt := range tests {
		if got := rec.Header().Get(tt.header); got != tt.expected {
			t.Errorf("expected %s: %s, got %s", tt.header, tt.expected, got)
		}
	}
}

func TestSecurityHeaders_HSTS_NotSetInDevelopment(t *testing.T) {
	handler := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS should not be set in development, got %s", got)
	}
}

func TestSecurityHeaders_HSTS_SetInProduction(t *testing.T) {
	handler := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test with TLS connection (r.TLS != nil)
	req := httptest.NewRequest(http.MethodGet, "https://sel.events/api/v1/events", nil)
	req.TLS = &tls.ConnectionState{} // Non-nil TLS to simulate HTTPS
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	expected := "max-age=31536000; includeSubDomains; preload"
	if got := rec.Header().Get("Strict-Transport-Security"); got != expected {
		t.Errorf("expected HSTS: %s, got %s", expected, got)
	}
}

func TestSecurityHeaders_HSTS_NotSetOnHTTP(t *testing.T) {
	handler := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test with HTTP connection (r.TLS == nil)
	req := httptest.NewRequest(http.MethodGet, "http://sel.events/api/v1/events", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// HSTS should not be set on HTTP connections even in production
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS should not be set on HTTP connections, got %s", got)
	}
}

func TestSecurityHeaders_AllEndpoints(t *testing.T) {
	handler := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	endpoints := []string{
		"/api/v1/events",
		"/api/v1/places",
		"/api/v1/admin/login",
		"/healthz",
		"/readyz",
	}

	for _, endpoint := range endpoints {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("X-Frame-Options"); got == "" {
			t.Errorf("security headers should be set on %s", endpoint)
		}
	}
}
