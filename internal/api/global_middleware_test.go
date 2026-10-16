package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/rs/zerolog"
)

// A rate-limited (429) public read must still carry CORS headers, otherwise a
// cross-origin page (e.g. file:// with Origin: null) cannot read the status or
// Retry-After and just sees an opaque network error.
func TestGlobalMiddleware_RateLimited429CarriesCORSHeaders(t *testing.T) {
	cfg := config.Config{
		CORS:      config.CORSConfig{AllowedOrigins: []string{"https://sel.events"}, PublicReadsOpen: true},
		RateLimit: config.RateLimitConfig{PublicPerMinute: 2},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := applyGlobalMiddleware(mux, cfg, zerolog.Nop(), false)

	var last *httptest.ResponseRecorder
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
		req.RemoteAddr = "203.0.113.7:4444"
		req.Header.Set("Origin", "null")
		last = httptest.NewRecorder()
		handler.ServeHTTP(last, req)
	}

	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("third request status = %d, want 429", last.Code)
	}
	if got := last.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("429 Allow-Origin = %q, want *", got)
	}
	if got := last.Header().Get("Access-Control-Expose-Headers"); got == "" {
		t.Error("429 must expose Retry-After to cross-origin pages")
	}
	if got := last.Header().Get("Retry-After"); got == "" {
		t.Error("429 missing Retry-After")
	}
}

func TestGlobalMiddleware_PreflightNotRateLimited(t *testing.T) {
	cfg := config.Config{
		CORS:      config.CORSConfig{PublicReadsOpen: true},
		RateLimit: config.RateLimitConfig{PublicPerMinute: 1},
	}
	handler := applyGlobalMiddleware(http.NewServeMux(), cfg, zerolog.Nop(), false)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/events", nil)
		req.RemoteAddr = "203.0.113.7:4444"
		req.Header.Set("Origin", "null")
		req.Header.Set("Access-Control-Request-Method", "GET")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("preflight %d status = %d, want 204 (preflights must not burn the rate limit)", i, rec.Code)
		}
	}
}
