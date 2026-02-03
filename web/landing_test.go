package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndexHandler(t *testing.T) {
	handler := IndexHandler()

	tests := []struct {
		name            string
		method          string
		wantStatus      int
		wantContentType string
	}{
		{
			name:            "GET returns 200",
			method:          http.MethodGet,
			wantStatus:      http.StatusOK,
			wantContentType: "text/html",
		},
		{
			name:            "HEAD returns 200",
			method:          http.MethodHead,
			wantStatus:      http.StatusOK,
			wantContentType: "text/html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			contentType := rec.Header().Get("Content-Type")
			if !strings.Contains(contentType, tt.wantContentType) {
				t.Errorf("Content-Type = %q, want to contain %q", contentType, tt.wantContentType)
			}

			// Check cache headers
			cacheControl := rec.Header().Get("Cache-Control")
			if !strings.Contains(cacheControl, "max-age=3600") {
				t.Errorf("Cache-Control = %q, want to contain max-age=3600", cacheControl)
			}
		})
	}
}

func TestIndexHandlerMethods(t *testing.T) {
	handler := IndexHandler()

	tests := []struct {
		name       string
		method     string
		wantStatus int
	}{
		{
			name:       "POST returns 405",
			method:     http.MethodPost,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "PUT returns 405",
			method:     http.MethodPut,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "DELETE returns 405",
			method:     http.MethodDelete,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "PATCH returns 405",
			method:     http.MethodPatch,
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			// Check Allow header is set
			allow := rec.Header().Get("Allow")
			if allow == "" {
				t.Error("Allow header not set for 405 response")
			}
			if !strings.Contains(allow, "GET") {
				t.Errorf("Allow header = %q, want to contain GET", allow)
			}
		})
	}
}

func TestIndexHTMLStructure(t *testing.T) {
	handler := IndexHandler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	bodyStr := string(body)

	// Check for essential content
	requiredStrings := []string{
		"Shared Events Library",
		"For Coding Agents",
		"For Humans",
		"For Bots",
		"GitHub:",
		"https://github.com/Togather-Foundation/server",
		"/api/docs",
		"/api/v1/openapi.json",
		"/api/v1/openapi.yaml",
		"Togather Foundation",
		"https://togather.foundation",
		"info@togather.foundation",
		"/sitemap.xml",
		"/robots.txt",
		"schema.org",
		"WebAPI",
	}

	for _, s := range requiredStrings {
		if !strings.Contains(bodyStr, s) {
			t.Errorf("HTML body missing expected string: %q", s)
		}
	}

	// Check for live stats JavaScript
	if !strings.Contains(bodyStr, "/health") {
		t.Error("HTML body missing /health endpoint reference")
	}
	if !strings.Contains(bodyStr, "/version") {
		t.Error("HTML body missing /version endpoint reference")
	}
	if !strings.Contains(bodyStr, "fetch") {
		t.Error("HTML body missing fetch API call for live stats")
	}
}
