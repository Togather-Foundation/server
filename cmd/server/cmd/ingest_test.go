package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestIngestCommandHelp(t *testing.T) {
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"ingest", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("ingest command --help failed: %v", err)
	}

	output := buf.String()

	// Verify help text contains expected content
	expectedStrings := []string{
		"Ingest events from a JSON file",
		"--key",
		"--server",
		"--timeout",
		"--watch",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help text to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestIngestCommandFlags(t *testing.T) {
	cmd := ingestCmd

	// Verify that ingest-specific flags are registered
	flags := []string{"key", "server", "timeout", "watch"}
	for _, flag := range flags {
		if f := cmd.Flags().Lookup(flag); f == nil {
			t.Errorf("expected flag %q to be defined on ingest command", flag)
		}
	}
}

func TestIngestCommandRequiresFile(t *testing.T) {
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"ingest"})

	err := root.Execute()
	if err == nil {
		t.Error("expected error when no file specified")
	}
}

func TestIngestEventsValidation(t *testing.T) {
	// Create temporary test files
	tests := []struct {
		name        string
		content     string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid JSON",
			content: `{
				"events": [
					{
						"@type": "Event",
						"name": "Test Event"
					}
				]
			}`,
			expectError: false,
		},
		{
			name:        "invalid JSON",
			content:     `{invalid json`,
			expectError: true,
			errorMsg:    "invalid JSON",
		},
		{
			name: "missing events key",
			content: `{
				"data": []
			}`,
			expectError: true,
			errorMsg:    "invalid JSON structure",
		},
		{
			name: "events not an array",
			content: `{
				"events": "not an array"
			}`,
			expectError: true,
			errorMsg:    "invalid JSON structure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpFile, err := os.CreateTemp("", "test-events-*.json")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(tt.content); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}
			_ = tmpFile.Close()

			// Set up test API key
			origAPIKey := os.Getenv("API_KEY")
			os.Setenv("API_KEY", "test-key")
			defer func() {
				if origAPIKey != "" {
					os.Setenv("API_KEY", origAPIKey)
				} else {
					os.Unsetenv("API_KEY")
				}
			}()

			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"batch_id": "test-batch-123",
					"status":   "accepted",
				})
			}))
			defer server.Close()

			// Save original flags
			origServerURL := ingestServerURL
			defer func() {
				ingestServerURL = origServerURL
			}()
			ingestServerURL = server.URL

			err = ingestEvents(tmpFile.Name())

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.expectError && err != nil && !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("expected error containing %q, got: %v", tt.errorMsg, err)
			}
		})
	}
}

func TestIngestEventsMissingAPIKey(t *testing.T) {
	// Create valid test file
	tmpFile, err := os.CreateTemp("", "test-events-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	content := `{"events": [{"@type": "Event", "name": "Test"}]}`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	_ = tmpFile.Close()

	// Clear API key
	origAPIKey := os.Getenv("API_KEY")
	_ = os.Unsetenv("API_KEY")
	defer func() {
		if origAPIKey != "" {
			_ = os.Setenv("API_KEY", origAPIKey)
		}
	}()

	// Save original flags
	origIngestAPIKey := ingestAPIKey
	defer func() {
		ingestAPIKey = origIngestAPIKey
	}()
	ingestAPIKey = ""

	err = ingestEvents(tmpFile.Name())
	if err == nil {
		t.Error("expected error when API key is missing")
	}
	if !strings.Contains(err.Error(), "API key required") {
		t.Errorf("expected 'API key required' error, got: %v", err)
	}
}

func TestIngestEventsServerResponse(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  map[string]interface{}
		expectError   bool
		errorContains string
	}{
		{
			name:       "accepted",
			statusCode: http.StatusAccepted,
			responseBody: map[string]interface{}{
				"batch_id":  "batch-123",
				"job_id":    float64(456),
				"status":    "pending",
				"submitted": float64(3),
			},
			expectError: false,
		},
		{
			name:       "ok",
			statusCode: http.StatusOK,
			responseBody: map[string]interface{}{
				"batch_id": "batch-456",
				"status":   "completed",
			},
			expectError: false,
		},
		{
			name:          "unauthorized",
			statusCode:    http.StatusUnauthorized,
			responseBody:  map[string]interface{}{"error": "invalid API key"},
			expectError:   true,
			errorContains: "401",
		},
		{
			name:          "bad request",
			statusCode:    http.StatusBadRequest,
			responseBody:  map[string]interface{}{"error": "invalid payload"},
			expectError:   true,
			errorContains: "400",
		},
		{
			name:          "server error",
			statusCode:    http.StatusInternalServerError,
			responseBody:  map[string]interface{}{"error": "internal error"},
			expectError:   true,
			errorContains: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file with valid JSON
			tmpFile, err := os.CreateTemp("", "test-events-*.json")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			content := `{"events": [{"@type": "Event", "name": "Test Event"}]}`
			if _, err := tmpFile.WriteString(content); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}
			tmpFile.Close()

			// Set up test API key
			origAPIKey := os.Getenv("API_KEY")
			os.Setenv("API_KEY", "test-key")
			defer func() {
				if origAPIKey != "" {
					os.Setenv("API_KEY", origAPIKey)
				} else {
					os.Unsetenv("API_KEY")
				}
			}()

			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if !strings.HasSuffix(r.URL.Path, "/api/v1/events:batch") {
					t.Errorf("expected path ending with /api/v1/events:batch, got %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("expected Authorization header with Bearer token")
				}

				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			// Save original flags
			origServerURL := ingestServerURL
			defer func() {
				ingestServerURL = origServerURL
			}()
			ingestServerURL = server.URL

			err = ingestEvents(tmpFile.Name())

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.expectError && err != nil && tt.errorContains != "" {
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errorContains, err)
				}
			}
		})
	}
}

func TestWatchBatchStatus(t *testing.T) {
	// Create mock server that simulates batch processing
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++

		if attemptCount < 2 {
			// First attempt: still processing
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Second attempt: completed
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"batch_id":  "batch-123",
			"status":    "completed",
			"processed": float64(3),
		})
	}))
	defer server.Close()

	// Create HTTP client
	client := &http.Client{}

	// Save original server URL
	origServerURL := ingestServerURL
	defer func() {
		ingestServerURL = origServerURL
	}()
	ingestServerURL = server.URL

	// Watch batch status
	err := watchBatchStatus(client, "batch-123")
	if err != nil {
		t.Errorf("watchBatchStatus failed: %v", err)
	}

	if attemptCount < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attemptCount)
	}
}

func TestIngestCommandFileNotFound(t *testing.T) {
	// Set up test API key
	origAPIKey := os.Getenv("API_KEY")
	_ = os.Setenv("API_KEY", "test-key")
	defer func() {
		if origAPIKey != "" {
			_ = os.Setenv("API_KEY", origAPIKey)
		} else {
			_ = os.Unsetenv("API_KEY")
		}
	}()

	err := ingestEvents("/nonexistent/file/12345.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "read file") {
		t.Errorf("expected 'read file' error, got: %v", err)
	}
}
