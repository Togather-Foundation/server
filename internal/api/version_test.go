package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestVersionHandler(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		gitCommit   string
		buildDate   string
		wantVersion string
		wantCommit  string
		wantDate    string
	}{
		{
			name:        "with all values",
			version:     "0.1.0",
			gitCommit:   "abc123def456",
			buildDate:   "2026-01-28T12:00:00Z",
			wantVersion: "0.1.0",
			wantCommit:  "abc123def456",
			wantDate:    "2026-01-28T12:00:00Z",
		},
		{
			name:        "with defaults",
			version:     "",
			gitCommit:   "",
			buildDate:   "",
			wantVersion: "dev",
			wantCommit:  "unknown",
			wantDate:    "unknown",
		},
		{
			name:        "with partial values",
			version:     "1.0.0",
			gitCommit:   "",
			buildDate:   "2026-01-28T12:00:00Z",
			wantVersion: "1.0.0",
			wantCommit:  "unknown",
			wantDate:    "2026-01-28T12:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := VersionHandler(tt.version, tt.gitCommit, tt.buildDate)

			req := httptest.NewRequest(http.MethodGet, "/version", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// Check status code
			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}

			// Check content type
			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
			}

			// Parse response
			var resp versionResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			// Verify response fields
			if resp.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", resp.Version, tt.wantVersion)
			}
			if resp.GitCommit != tt.wantCommit {
				t.Errorf("GitCommit = %q, want %q", resp.GitCommit, tt.wantCommit)
			}
			if resp.BuildDate != tt.wantDate {
				t.Errorf("BuildDate = %q, want %q", resp.BuildDate, tt.wantDate)
			}
			if resp.GoVersion != runtime.Version() {
				t.Errorf("GoVersion = %q, want %q", resp.GoVersion, runtime.Version())
			}
		})
	}
}

func TestVersionHandler_MethodNotAllowed(t *testing.T) {
	handler := VersionHandler("0.1.0", "abc123", "2026-01-28T12:00:00Z")

	methods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/version", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}
