package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/rs/zerolog"
)

// TestCORS_ProductionMode_BlockedOrigin_LogsWarning verifies that rejected CORS requests are logged
func TestCORS_ProductionMode_BlockedOrigin_LogsWarning(t *testing.T) {
	cfg := config.CORSConfig{
		AllowAllOrigins: false,
		AllowedOrigins:  []string{"https://sel.events"},
	}

	// Create a buffer to capture log output
	var logBuf bytes.Buffer
	logger := zerolog.New(&logBuf).With().Timestamp().Logger()

	handler := CORS(cfg, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Request should succeed but without CORS headers
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header, got %s", got)
	}

	// Verify that a warning was logged
	logOutput := logBuf.String()
	if logOutput == "" {
		t.Fatal("expected log output, got empty string")
	}

	// Parse the JSON log entry
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(logOutput), &logEntry); err != nil {
		t.Fatalf("failed to parse log output as JSON: %v\nOutput: %s", err, logOutput)
	}

	// Verify log level is warning
	if level, ok := logEntry["level"].(string); !ok || level != "warn" {
		t.Errorf("expected log level 'warn', got %v", logEntry["level"])
	}

	// Verify origin is logged
	if origin, ok := logEntry["origin"].(string); !ok || origin != "https://evil.com" {
		t.Errorf("expected origin 'https://evil.com' in log, got %v", logEntry["origin"])
	}

	// Verify path is logged
	if path, ok := logEntry["path"].(string); !ok || path != "/api/v1/events" {
		t.Errorf("expected path '/api/v1/events' in log, got %v", logEntry["path"])
	}

	// Verify method is logged
	if method, ok := logEntry["method"].(string); !ok || method != "GET" {
		t.Errorf("expected method 'GET' in log, got %v", logEntry["method"])
	}

	// Verify message contains relevant info
	if msg, ok := logEntry["message"].(string); !ok || msg != "CORS request rejected: origin not in whitelist" {
		t.Errorf("expected message about CORS rejection, got %v", logEntry["message"])
	}
}

func TestCORS_DevelopmentMode(t *testing.T) {
	cfg := config.CORSConfig{
		AllowAllOrigins: true,
		AllowedOrigins:  nil,
	}
	logger := zerolog.Nop() // No-op logger for tests

	handler := CORS(cfg, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test with Origin header
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("expected Access-Control-Allow-Origin: http://localhost:3000, got %s", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials: true, got %s", got)
	}
}

func TestCORS_ProductionMode_AllowedOrigin(t *testing.T) {
	cfg := config.CORSConfig{
		AllowAllOrigins: false,
		AllowedOrigins:  []string{"https://sel.events", "https://admin.sel.events"},
	}
	logger := zerolog.Nop() // No-op logger for tests

	handler := CORS(cfg, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Origin", "https://sel.events")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://sel.events" {
		t.Errorf("expected Access-Control-Allow-Origin: https://sel.events, got %s", got)
	}
}

func TestCORS_ProductionMode_BlockedOrigin(t *testing.T) {
	cfg := config.CORSConfig{
		AllowAllOrigins: false,
		AllowedOrigins:  []string{"https://sel.events"},
	}
	logger := zerolog.Nop() // No-op logger for tests

	handler := CORS(cfg, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Request should succeed but without CORS headers
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header, got %s", got)
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	cfg := config.CORSConfig{
		AllowAllOrigins: true,
		AllowedOrigins:  nil,
	}
	logger := zerolog.Nop() // No-op logger for tests

	handler := CORS(cfg, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request without Origin header (same-origin request)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	// No CORS headers should be set for same-origin requests
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no CORS headers for same-origin request, got %s", got)
	}
}

func TestCORS_PreflightRequest(t *testing.T) {
	cfg := config.CORSConfig{
		AllowAllOrigins: true,
		AllowedOrigins:  nil,
	}
	logger := zerolog.Nop() // No-op logger for tests

	handler := CORS(cfg, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for OPTIONS preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/events", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status 204 for OPTIONS, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("expected Access-Control-Allow-Origin header, got %s", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Access-Control-Allow-Methods header")
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Errorf("expected Access-Control-Max-Age: 86400, got %s", got)
	}
}

// serveCORS runs one request through the CORS middleware and returns the recorder.
func serveCORS(t *testing.T, cfg config.CORSConfig, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	handler := CORS(cfg, zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// TestCORS_PublicReads_OpenToAnyOrigin covers the W04 job-search page use case: a
// page opened from file:// (Origin: null), a phone, or an arbitrary static host
// must be able to read public GET endpoints even when the production whitelist
// does not list it. The answer is a credential-less wildcard.
func TestCORS_PublicReads_OpenToAnyOrigin(t *testing.T) {
	cfgs := map[string]config.CORSConfig{
		"whitelist": {AllowedOrigins: []string{"https://sel.events"}, PublicReadsOpen: true},
		"allow-all": {AllowAllOrigins: true, PublicReadsOpen: true},
	}
	paths := []string{
		"/api/v1/events",
		"/api/v1/events.ics",
		"/api/v1/events/01ABC",
		"/api/v1/events/01ABC/ics",
		"/api/v1/places",
		"/api/v1/places/01ABC",
		"/api/v1/organizations",
		"/api/v1/organizations/01ABC",
		"/api/v1/openapi.json",
		"/api/v1/openapi.yaml",
		"/api/v1/stats",
	}
	origins := []string{"null", "https://evil.example", "http://localhost:3000"}

	for name, cfg := range cfgs {
		for _, path := range paths {
			for _, origin := range origins {
				for _, method := range []string{http.MethodGet, http.MethodHead} {
					rec := serveCORS(t, cfg, method, path, map[string]string{"Origin": origin})
					if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
						t.Errorf("%s %s %s Origin=%s: Allow-Origin = %q, want *", name, method, path, origin, got)
					}
					if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
						t.Errorf("%s %s %s Origin=%s: Allow-Credentials = %q, want unset", name, method, path, origin, got)
					}
					if got := rec.Header().Get("Access-Control-Expose-Headers"); got == "" {
						t.Errorf("%s %s %s: Expose-Headers must stay set so pages can read Retry-After", name, method, path)
					}
				}
			}
		}
	}
}

func TestCORS_PublicReads_Preflight(t *testing.T) {
	cfg := config.CORSConfig{AllowedOrigins: []string{"https://sel.events"}, PublicReadsOpen: true}

	rec := serveCORS(t, cfg, http.MethodOptions, "/api/v1/events", map[string]string{
		"Origin":                         "null",
		"Access-Control-Request-Method":  "GET",
		"Access-Control-Request-Headers": "accept",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("preflight Allow-Origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("preflight Allow-Credentials = %q, want unset", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, HEAD, OPTIONS" {
		t.Errorf("preflight Allow-Methods = %q, want read-only methods", got)
	}
}

// TestCORS_PublicReads_DoNotOpenWrites guards the dangerous direction: the same
// path (POST /api/v1/events creates events) and non-public routes must never get
// the open wildcard.
func TestCORS_PublicReads_DoNotOpenWrites(t *testing.T) {
	cfg := config.CORSConfig{AllowedOrigins: []string{"https://sel.events"}, PublicReadsOpen: true}
	evil := map[string]string{"Origin": "https://evil.example"}

	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/events"},
		{http.MethodPut, "/api/v1/events/01ABC"},
		{http.MethodDelete, "/api/v1/events/01ABC"},
		{http.MethodPost, "/api/v1/events:batch"},
		{http.MethodGet, "/api/v1/admin/stats"},
		{http.MethodGet, "/api/v1/admin/events"},
		{http.MethodPost, "/api/v1/auth/token"},
		{http.MethodPost, "/api/v1/admin/login"},
		{http.MethodGet, "/dev/dashboard"},
		{http.MethodGet, "/admin/dashboard"},
		{http.MethodGet, "/api/v1/events/01ABC/publish"},
		{http.MethodGet, "/api/v1/events/"},
		{http.MethodGet, "/api/v1/geocode"},
		{http.MethodGet, "/api/v1/eventsXYZ"},
	}
	for _, tc := range cases {
		rec := serveCORS(t, cfg, tc.method, tc.path, evil)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%s %s: Allow-Origin = %q, want none for non-whitelisted origin", tc.method, tc.path, got)
		}
	}

	// Preflight announcing a write on a public path is not a public read.
	rec := serveCORS(t, cfg, http.MethodOptions, "/api/v1/events", map[string]string{
		"Origin":                        "https://evil.example",
		"Access-Control-Request-Method": "POST",
	})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("POST preflight on /api/v1/events: Allow-Origin = %q, want none", got)
	}
}

func TestCORS_PublicReadsClosed_FallsBackToWhitelist(t *testing.T) {
	cfg := config.CORSConfig{AllowedOrigins: []string{"https://sel.events"}, PublicReadsOpen: false}

	rec := serveCORS(t, cfg, http.MethodGet, "/api/v1/events", map[string]string{"Origin": "null"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want none when public reads are not opened", got)
	}
}

// TestCORS_AllowAll_NoCredentialsForForeignOrigins: reflecting an arbitrary origin
// together with Allow-Credentials lets any website make credentialed requests to
// admin/auth/write routes. Credentials must be limited to explicitly whitelisted
// origins (and loopback origins when running in allow-all dev mode).
func TestCORS_AllowAll_NoCredentialsForForeignOrigins(t *testing.T) {
	cfg := config.CORSConfig{AllowAllOrigins: true, PublicReadsOpen: true}

	for _, path := range []string{"/api/v1/admin/stats", "/api/v1/auth/token", "/dev/dashboard", "/api/v1/events"} {
		rec := serveCORS(t, cfg, http.MethodPost, path, map[string]string{"Origin": "https://evil.example"})
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Errorf("POST %s from foreign origin: Allow-Credentials = %q, want unset", path, got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Errorf("POST %s: Vary = %q, want Origin (reflected origin)", path, got)
		}
	}

	rec := serveCORS(t, cfg, http.MethodGet, "/api/v1/admin/stats", map[string]string{"Origin": "http://127.0.0.1:5173"})
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("loopback dev origin: Allow-Credentials = %q, want true", got)
	}
}

func TestCORS_Whitelist_CredentialsOnlyForListedOrigin(t *testing.T) {
	cfg := config.CORSConfig{AllowedOrigins: []string{"https://admin.sel.events"}, PublicReadsOpen: true}

	rec := serveCORS(t, cfg, http.MethodGet, "/api/v1/admin/stats", map[string]string{"Origin": "https://admin.sel.events"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://admin.sel.events" {
		t.Errorf("Allow-Origin = %q, want the listed origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true for a listed origin", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
}

// A cached response to a request without an Origin header must not poison later
// cross-origin fetches of the same public URL, so public reads always carry ACAO: *.
func TestCORS_PublicReads_WildcardEvenWithoutOrigin(t *testing.T) {
	cfg := config.CORSConfig{AllowedOrigins: []string{"https://sel.events"}, PublicReadsOpen: true}

	rec := serveCORS(t, cfg, http.MethodGet, "/api/v1/stats", nil)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("public read without Origin: ACAO = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("public read without Origin: unexpected Allow-Credentials %q", got)
	}

	rec = serveCORS(t, cfg, http.MethodGet, "/api/v1/admin/stats", nil)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("non-public path without Origin: ACAO = %q, want none", got)
	}

	rec = serveCORS(t, cfg, http.MethodPost, "/api/v1/events", nil)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("write without Origin: ACAO = %q, want none", got)
	}
}
