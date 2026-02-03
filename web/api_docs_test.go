package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIDocsHandler(t *testing.T) {
	handler := APIDocsHandler()

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		expectedType   string
	}{
		{
			name:           "GET root returns index.html",
			method:         http.MethodGet,
			path:           "/api/docs",
			expectedStatus: http.StatusOK,
			expectedType:   "text/html",
		},
		{
			name:           "GET with trailing slash returns index.html",
			method:         http.MethodGet,
			path:           "/api/docs/",
			expectedStatus: http.StatusOK,
			expectedType:   "text/html",
		},
		{
			name:           "GET JS file returns JavaScript",
			method:         http.MethodGet,
			path:           "/api/docs/scalar-standalone.js",
			expectedStatus: http.StatusOK,
			expectedType:   "text/javascript",
		},
		{
			name:           "POST not allowed",
			method:         http.MethodPost,
			path:           "/api/docs",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "PUT not allowed",
			method:         http.MethodPut,
			path:           "/api/docs",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "DELETE not allowed",
			method:         http.MethodDelete,
			path:           "/api/docs",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "HEAD allowed",
			method:         http.MethodHead,
			path:           "/api/docs",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectedType != "" {
				contentType := rec.Header().Get("Content-Type")
				if contentType != tt.expectedType && !containsContentType(contentType, tt.expectedType) {
					t.Errorf("expected Content-Type to contain %q, got %q", tt.expectedType, contentType)
				}
			}

			// Verify cache headers for successful responses
			if rec.Code == http.StatusOK {
				cacheControl := rec.Header().Get("Cache-Control")
				if tt.path == "/api/docs/scalar-standalone.js" {
					// JS files should have long-term caching
					if cacheControl != "public, max-age=31536000, immutable" {
						t.Errorf("expected long-term cache for JS, got %q", cacheControl)
					}
				} else if tt.expectedType == "text/html" {
					// HTML should not be cached
					if cacheControl != "no-cache, must-revalidate" {
						t.Errorf("expected no-cache for HTML, got %q", cacheControl)
					}
					// HTML should have relaxed CSP to allow Scalar inline scripts
					csp := rec.Header().Get("Content-Security-Policy")
					if !contains(csp, "'unsafe-inline'") {
						t.Errorf("expected CSP to allow unsafe-inline for HTML, got %q", csp)
					}
				}
			}
		})
	}
}

// TestAPIDocsHandlerContent verifies the embedded files are actually served
func TestAPIDocsHandlerContent(t *testing.T) {
	handler := APIDocsHandler()

	t.Run("index.html contains Scalar initialization", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		body := rec.Body.String()

		// Verify key elements of the Scalar UI HTML
		requiredStrings := []string{
			"<!doctype html>",
			"Togather API Documentation",
			"Scalar",
			"/api/v1/openapi.json",
			"info@togather.foundation",
		}

		for _, required := range requiredStrings {
			if !contains(body, required) {
				t.Errorf("expected HTML to contain %q, but it was not found", required)
			}
		}
	})

	t.Run("scalar-standalone.js is non-empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/docs/scalar-standalone.js", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		if rec.Body.Len() < 1000 {
			t.Errorf("expected JS file to be substantial (>1KB), got %d bytes", rec.Body.Len())
		}
	})
}

// Helper functions
func contains(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 &&
		(haystack == needle || len(haystack) >= len(needle) &&
			findSubstring(haystack, needle))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsContentType(contentType, expected string) bool {
	// Content-Type may include charset, e.g., "text/html; charset=utf-8"
	return contains(contentType, expected)
}
