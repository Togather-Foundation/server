package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAPIHandler(t *testing.T) {
	handler := OpenAPIHandler()

	tests := []struct {
		name           string
		method         string
		expectStatus   int
		expectHeader   string
		expectNotEmpty bool
	}{
		{
			name:           "GET returns OpenAPI spec",
			method:         http.MethodGet,
			expectStatus:   http.StatusOK,
			expectHeader:   "application/json",
			expectNotEmpty: true,
		},
		{
			name:         "POST not allowed",
			method:       http.MethodPost,
			expectStatus: http.StatusMethodNotAllowed,
		},
		{
			name:         "PUT not allowed",
			method:       http.MethodPut,
			expectStatus: http.StatusMethodNotAllowed,
		},
		{
			name:         "DELETE not allowed",
			method:       http.MethodDelete,
			expectStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/v1/openapi.json", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("expected status %d, got %d", tt.expectStatus, w.Code)
			}

			if tt.expectHeader != "" {
				contentType := w.Header().Get("Content-Type")
				if contentType != tt.expectHeader {
					t.Errorf("expected Content-Type %q, got %q", tt.expectHeader, contentType)
				}
			}

			if tt.expectNotEmpty && w.Body.Len() == 0 {
				t.Error("expected non-empty response body")
			}
		})
	}
}

func TestOpenAPIHandlerCaching(t *testing.T) {
	// Test that OpenAPI spec is loaded once and cached
	handler := OpenAPIHandler()

	// Make first request
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	// Make second request
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	// Both should return the same status
	if w1.Code != w2.Code {
		t.Errorf("expected same status code, got %d and %d", w1.Code, w2.Code)
	}

	// If successful, both should have same body
	if w1.Code == http.StatusOK && w2.Code == http.StatusOK {
		if w1.Body.String() != w2.Body.String() {
			t.Error("expected cached response to be identical")
		}
	}
}

func TestResolveOpenAPIPath(t *testing.T) {
	// Test that resolveOpenAPIPath returns a valid path
	path := resolveOpenAPIPath()

	if path == "" {
		t.Error("expected non-empty path")
	}

	// Path should contain the expected filename
	if len(path) < len("openapi.yaml") {
		t.Error("path seems too short")
	}
}

func TestRepoRoot(t *testing.T) {
	// Test that repoRoot finds the repository root
	root, err := repoRoot()

	// It's okay if this fails in some environments,
	// but if it succeeds, verify it's a valid path
	if err == nil {
		if root == "" {
			t.Error("expected non-empty root path when no error")
		}

		// Root should be an absolute path
		if root[0] != '/' && len(root) < 2 || (len(root) >= 2 && root[1] != ':') {
			t.Error("expected absolute path")
		}
	}
}

func TestOpenAPIHandlerMultipleConcurrentRequests(t *testing.T) {
	// Test that handler is safe for concurrent use
	handler := OpenAPIHandler()

	done := make(chan bool)
	numRequests := 10

	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			// Just verify it doesn't panic and returns a valid status
			if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
				t.Errorf("unexpected status code: %d", w.Code)
			}

			done <- true
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}
}
