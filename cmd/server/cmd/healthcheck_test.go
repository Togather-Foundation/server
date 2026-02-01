package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// TestPerformHealthCheck tests the basic health check functionality
func TestPerformHealthCheck(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   interface{}
		expectHealthy  bool
		expectError    bool
		expectedStatus string
	}{
		{
			name:       "healthy server",
			statusCode: http.StatusOK,
			responseBody: HealthResponse{
				Status: "healthy",
				Checks: map[string]CheckResult{
					"database": {Status: "pass"},
				},
			},
			expectHealthy:  true,
			expectError:    false,
			expectedStatus: "healthy",
		},
		{
			name:       "degraded server",
			statusCode: http.StatusOK,
			responseBody: HealthResponse{
				Status: "degraded",
				Checks: map[string]CheckResult{
					"database":  {Status: "pass"},
					"job_queue": {Status: "warn"},
				},
			},
			expectHealthy:  false,
			expectError:    false,
			expectedStatus: "degraded",
		},
		{
			name:           "unhealthy server (503)",
			statusCode:     http.StatusServiceUnavailable,
			responseBody:   HealthResponse{Status: "unhealthy"},
			expectHealthy:  false,
			expectError:    false,
			expectedStatus: "unhealthy",
		},
		{
			name:          "invalid response",
			statusCode:    http.StatusOK,
			responseBody:  "not json",
			expectHealthy: false,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if str, ok := tt.responseBody.(string); ok {
					fmt.Fprint(w, str)
				} else {
					_ = json.NewEncoder(w).Encode(tt.responseBody)
				}
			}))
			defer server.Close()

			// Perform health check
			result := performHealthCheck(server.URL)

			// Validate result
			if result.IsHealthy != tt.expectHealthy {
				t.Errorf("expected IsHealthy=%v, got %v", tt.expectHealthy, result.IsHealthy)
			}

			if tt.expectError {
				if result.Error == "" {
					t.Error("expected error, got none")
				}
			}

			if !tt.expectError && tt.expectedStatus != "" {
				if result.Status != tt.expectedStatus {
					t.Errorf("expected status=%s, got %s", tt.expectedStatus, result.Status)
				}
			}

			// Check latency is recorded (allow 0ms for very fast responses)
			if result.LatencyMs < 0 {
				t.Error("expected non-negative latency")
			}
		})
	}
}

// TestPerformHealthCheckTimeout tests timeout handling
func TestPerformHealthCheckTimeout(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // Longer than default timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Set short timeout
	healthcheckTimeout = 1

	result := performHealthCheck(server.URL)

	if result.Error == "" {
		t.Error("expected timeout error, got none")
	}

	if result.IsHealthy {
		t.Error("expected unhealthy result on timeout")
	}
}

// TestGetSlotPort tests slot port mapping
func TestGetSlotPort(t *testing.T) {
	tests := []struct {
		slot         string
		expectedPort int
	}{
		{"blue", 8081},
		{"green", 8082},
		{"", 8080},
		{"invalid", 8080},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("slot_%s", tt.slot), func(t *testing.T) {
			port := getSlotPort(tt.slot)
			if port != tt.expectedPort {
				t.Errorf("expected port %d for slot %s, got %d", tt.expectedPort, tt.slot, port)
			}
		})
	}
}

// TestDetermineHealthCheckURLs tests URL determination logic
func TestDetermineHealthCheckURLs(t *testing.T) {
	// Save original env
	originalPort := os.Getenv("SERVER_PORT")
	defer os.Setenv("SERVER_PORT", originalPort)

	tests := []struct {
		name         string
		urlFlag      string
		slotFlag     string
		serverPort   string
		expectedURLs []string
	}{
		{
			name:         "explicit URL",
			urlFlag:      "http://example.com/health",
			expectedURLs: []string{"http://example.com/health"},
		},
		{
			name:         "blue slot",
			slotFlag:     "blue",
			expectedURLs: []string{"http://localhost:8081/health"},
		},
		{
			name:         "green slot",
			slotFlag:     "green",
			expectedURLs: []string{"http://localhost:8082/health"},
		},
		{
			name:         "default with SERVER_PORT",
			serverPort:   "9000",
			expectedURLs: []string{"http://localhost:9000/health"},
		},
		{
			name:         "default without SERVER_PORT",
			expectedURLs: []string{"http://localhost:8080/health"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			healthcheckURL = tt.urlFlag
			healthcheckSlot = tt.slotFlag
			healthcheckDeployment = ""

			// Set environment
			if tt.serverPort != "" {
				os.Setenv("SERVER_PORT", tt.serverPort)
			} else {
				os.Unsetenv("SERVER_PORT")
			}

			urls, err := determineHealthCheckURLs()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(urls) != len(tt.expectedURLs) {
				t.Fatalf("expected %d URLs, got %d", len(tt.expectedURLs), len(urls))
			}

			for i, url := range urls {
				if url != tt.expectedURLs[i] {
					t.Errorf("expected URL[%d]=%s, got %s", i, tt.expectedURLs[i], url)
				}
			}
		})
	}
}

// TestPerformHealthCheckWithRetries tests retry logic
func TestPerformHealthCheckWithRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			// Fail first two attempts
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(HealthResponse{Status: "unhealthy"})
		} else {
			// Succeed on third attempt
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(HealthResponse{Status: "healthy"})
		}
	}))
	defer server.Close()

	// Configure retries
	healthcheckRetries = 3
	healthcheckRetryDelay = 10 * time.Millisecond

	result := performHealthCheckWithRetries(server.URL)

	if !result.IsHealthy {
		t.Error("expected healthy result after retries")
	}

	if result.RetryCount != 2 {
		t.Errorf("expected 2 retries, got %d", result.RetryCount)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

// TestPerformHealthCheckWithRetriesAllFail tests retry exhaustion
func TestPerformHealthCheckWithRetriesAllFail(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(HealthResponse{Status: "unhealthy"})
	}))
	defer server.Close()

	// Configure retries
	healthcheckRetries = 2
	healthcheckRetryDelay = 10 * time.Millisecond

	result := performHealthCheckWithRetries(server.URL)

	if result.IsHealthy {
		t.Error("expected unhealthy result after all retries exhausted")
	}

	if result.RetryCount != 2 {
		t.Errorf("expected 2 retries, got %d", result.RetryCount)
	}

	if attempts != 3 { // Initial attempt + 2 retries
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

// TestOutputFormats tests different output formats
func TestOutputFormats(t *testing.T) {
	results := []HealthCheckResult{
		{
			URL:        "http://localhost:8081/health",
			Status:     "healthy",
			StatusCode: 200,
			IsHealthy:  true,
			LatencyMs:  42,
			Slot:       "blue",
			Response: &HealthResponse{
				Status: "healthy",
				Checks: map[string]CheckResult{
					"database":  {Status: "pass"},
					"job_queue": {Status: "pass"},
				},
			},
		},
	}

	tests := []struct {
		name   string
		format string
	}{
		{"json", "json"},
		{"table", "table"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthcheckFormat = tt.format

			// Capture output (this will write to stdout, we just verify no panic)
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("outputResults panicked: %v", r)
				}
			}()

			// Note: In real scenario, we'd redirect stdout to verify output
			// For now, just verify it doesn't panic
			outputResults(results)
		})
	}
}

// TestGetDeploymentURLs tests deployment state integration
func TestGetDeploymentURLs(t *testing.T) {
	t.Skip("Skipping deployment state integration test - requires state file setup")
	// Note: This test would need proper state file setup in a real scenario
	// We've tested the underlying deployment.State logic in internal/deployment/deployment_test.go
}

// TestGetDeploymentURLsNoDeployment tests error handling for missing deployment
func TestGetDeploymentURLsNoDeployment(t *testing.T) {
	t.Skip("Skipping deployment state integration test - requires state file setup")
	// Note: This test would need proper state file setup in a real scenario
}

// TestSlotDetectionFromURL tests slot detection based on URL port
func TestSlotDetectionFromURL(t *testing.T) {
	tests := []struct {
		url          string
		expectedSlot string
	}{
		{"http://localhost:8081/health", "blue"},
		{"http://localhost:8082/health", "green"},
		{"http://localhost:8080/health", ""},
		{"http://example.com/health", ""},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(HealthResponse{Status: "healthy"})
			}))
			defer server.Close()

			// Replace port in URL
			testURL := tt.url
			if tt.url != "http://example.com/health" {
				testURL = server.URL
				if tt.expectedSlot == "blue" {
					testURL = "http://localhost:8081/health"
				} else if tt.expectedSlot == "green" {
					testURL = "http://localhost:8082/health"
				}
			}

			result := performHealthCheck(testURL)

			if result.Slot != tt.expectedSlot {
				t.Errorf("expected slot=%s, got %s", tt.expectedSlot, result.Slot)
			}
		})
	}
}
