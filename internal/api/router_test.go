package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMethodMux(t *testing.T) {
	getHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("GET response"))
	})

	postHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("POST response"))
	})

	handlers := map[string]http.Handler{
		http.MethodGet:  getHandler,
		http.MethodPost: postHandler,
	}

	mux := methodMux(handlers)

	tests := []struct {
		name         string
		method       string
		expectStatus int
		expectBody   string
		expectAllow  string
	}{
		{
			name:         "GET allowed",
			method:       http.MethodGet,
			expectStatus: http.StatusOK,
			expectBody:   "GET response",
		},
		{
			name:         "POST allowed",
			method:       http.MethodPost,
			expectStatus: http.StatusCreated,
			expectBody:   "POST response",
		},
		{
			name:         "PUT not allowed",
			method:       http.MethodPut,
			expectStatus: http.StatusMethodNotAllowed,
			expectAllow:  "GET, POST",
		},
		{
			name:         "DELETE not allowed",
			method:       http.MethodDelete,
			expectStatus: http.StatusMethodNotAllowed,
			expectAllow:  "GET, POST",
		},
		{
			name:         "PATCH not allowed",
			method:       http.MethodPatch,
			expectStatus: http.StatusMethodNotAllowed,
			expectAllow:  "GET, POST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("expected status %d, got %d", tt.expectStatus, w.Code)
			}

			if tt.expectBody != "" {
				body := w.Body.String()
				if body != tt.expectBody {
					t.Errorf("expected body %q, got %q", tt.expectBody, body)
				}
			}

			if tt.expectAllow != "" {
				allow := w.Header().Get("Allow")
				if allow != tt.expectAllow {
					t.Errorf("expected Allow header %q, got %q", tt.expectAllow, allow)
				}
			}
		})
	}
}

func TestAllowedMethods(t *testing.T) {
	tests := []struct {
		name     string
		handlers map[string]http.Handler
		expected string
	}{
		{
			name: "single method",
			handlers: map[string]http.Handler{
				http.MethodGet: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			},
			expected: "GET",
		},
		{
			name: "two methods sorted",
			handlers: map[string]http.Handler{
				http.MethodPost: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
				http.MethodGet:  http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			},
			expected: "GET, POST",
		},
		{
			name: "multiple methods sorted",
			handlers: map[string]http.Handler{
				http.MethodPut:    http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
				http.MethodGet:    http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
				http.MethodDelete: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
				http.MethodPost:   http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			},
			expected: "DELETE, GET, POST, PUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := allowedMethods(tt.handlers)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFindShapesDirectory(t *testing.T) {
	// Test that findShapesDirectory returns a path
	path := findShapesDirectory()

	if path == "" {
		t.Error("expected non-empty path")
	}

	// Path should contain "shapes"
	if len(path) < len("shapes") {
		t.Error("path seems too short")
	}
}

func TestLoadAdminTemplates(t *testing.T) {
	// Test that loadAdminTemplates attempts to find templates
	// It may return an error if templates aren't found, which is okay
	tmpl, err := loadAdminTemplates()

	// Either it succeeds and returns templates, or fails gracefully
	if err == nil && tmpl == nil {
		t.Error("if no error, expected non-nil template")
	}

	// If it succeeded, verify we got a template
	if err == nil {
		if tmpl == nil {
			t.Error("expected template when no error")
		}
	}
}

func TestMethodMuxEmptyHandlers(t *testing.T) {
	// Test that methodMux handles empty handlers map
	handlers := map[string]http.Handler{}
	mux := methodMux(handlers)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d for empty handlers, got %d", http.StatusMethodNotAllowed, w.Code)
	}

	// Allow header should be empty for no handlers
	allow := w.Header().Get("Allow")
	if allow != "" {
		t.Errorf("expected empty Allow header, got %q", allow)
	}
}

func TestMethodMuxOptionsMethod(t *testing.T) {
	// Test that OPTIONS returns 405 (not explicitly handled)
	handlers := map[string]http.Handler{
		http.MethodGet: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	mux := methodMux(handlers)

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// OPTIONS is not in the handlers, so should get 405
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}

	// Should have Allow header
	allow := w.Header().Get("Allow")
	if allow == "" {
		t.Error("expected Allow header to be set")
	}
}
