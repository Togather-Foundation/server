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
