package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestInit(t *testing.T) {
	// Create a new registry for testing (avoid conflicts with global registry)
	testRegistry := prometheus.NewRegistry()

	// Test that Init doesn't panic
	Init("v1.0.0", "abc123", "2026-01-30")

	// Verify app_info metric exists
	if testutil.CollectAndCount(AppInfo) == 0 {
		t.Error("AppInfo metric should be registered")
	}

	_ = testRegistry // unused but shows pattern for isolated tests
}

func TestHTTPMiddleware(t *testing.T) {
	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Wrap with metrics middleware
	wrapped := HTTPMiddleware(handler)

	// Create test request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Execute request
	wrapped.ServeHTTP(rec, req)

	// Verify response
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Verify metrics were recorded
	if testutil.CollectAndCount(HTTPRequestsTotal) == 0 {
		t.Error("HTTPRequestsTotal should have recorded at least one request")
	}

	if testutil.CollectAndCount(HTTPRequestDuration) == 0 {
		t.Error("HTTPRequestDuration should have recorded at least one request")
	}
}

func TestHTTPMiddlewareStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"OK", http.StatusOK},
		{"Not Found", http.StatusNotFound},
		{"Internal Server Error", http.StatusInternalServerError},
		{"Unauthorized", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			wrapped := HTTPMiddleware(handler)
			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			if rec.Code != tt.statusCode {
				t.Errorf("Expected status %d, got %d", tt.statusCode, rec.Code)
			}
		})
	}
}

func TestDBCollector(t *testing.T) {
	// Note: This test doesn't use a real pgxpool to avoid test dependencies
	// In production, the collector will work with a real pool

	// Create collector with nil pool (should not panic)
	collector := NewDBCollector(nil)

	// Collect should not panic with nil pool
	collector.collect()

	// Stop should not panic
	collector.Stop()
}

func TestRecordQuery(t *testing.T) {
	// Test successful query
	start := time.Now()
	RecordQuery("test_select", start, nil)

	// Verify metric was recorded
	if testutil.CollectAndCount(DBQueryDuration) == 0 {
		t.Error("DBQueryDuration should have recorded at least one query")
	}

	// Test failed query
	start = time.Now()
	RecordQuery("test_failed", start, context.Canceled)

	// Verify error was recorded
	if testutil.CollectAndCount(DBErrors) == 0 {
		t.Error("DBErrors should have recorded at least one error")
	}
}

func TestResponseWriterStatusCode(t *testing.T) {
	// Test that default status code is 200 when WriteHeader is not called
	rec := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: rec,
		statusCode:     0,
		bytesWritten:   0,
	}

	_, _ = rw.Write([]byte("test"))

	if rw.statusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", rw.statusCode)
	}
}

func TestResponseWriterBytesWritten(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: rec,
		statusCode:     0,
		bytesWritten:   0,
	}

	content := []byte("Hello, World!")
	_, _ = rw.Write(content)

	if rw.bytesWritten != len(content) {
		t.Errorf("Expected %d bytes written, got %d", len(content), rw.bytesWritten)
	}
}
