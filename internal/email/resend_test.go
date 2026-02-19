package email

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/resend/resend-go/v2"
	"github.com/rs/zerolog"
)

// TestSendViaResend_Success verifies successful email sending via Resend API
func TestSendViaResend_Success(t *testing.T) {
	// Mock HTTP server to simulate Resend API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "POST" || r.URL.Path != "/emails" {
			t.Errorf("Expected POST /emails, got %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Verify Authorization header
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			t.Errorf("Expected Bearer token in Authorization header, got %q", authHeader)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Parse request body
		var req resend.SendEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Verify request parameters
		if req.From != "test@example.com" {
			t.Errorf("Expected From=test@example.com, got %q", req.From)
		}
		if len(req.To) != 1 || req.To[0] != "recipient@example.com" {
			t.Errorf("Expected To=[recipient@example.com], got %v", req.To)
		}
		if req.Subject != "Test Subject" {
			t.Errorf("Expected Subject='Test Subject', got %q", req.Subject)
		}
		if !strings.Contains(req.Html, "Test Body") {
			t.Errorf("Expected HTML body to contain 'Test Body', got %q", req.Html)
		}

		// Return successful response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id": "mock-email-id-123",
		})
	}))
	defer mockServer.Close()

	// Create service with mock Resend client
	cfg := config.EmailConfig{
		Enabled:      true,
		Provider:     "resend",
		From:         "test@example.com",
		ResendAPIKey: "test-api-key",
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	client := resend.NewClient("test-api-key")
	baseURL, _ := url.Parse(mockServer.URL)
	client.BaseURL = baseURL

	svc := &Service{
		config:       cfg,
		provider:     "resend",
		resendClient: client,
		logger:       logger,
	}

	// Test sending
	ctx := context.Background()
	err := svc.sendViaResend(ctx, "recipient@example.com", "Test Subject", "<html><body>Test Body</body></html>")
	if err != nil {
		t.Errorf("Expected successful send, got error: %v", err)
	}
}

// TestSendViaResend_RateLimitError verifies rate limit error handling
func TestSendViaResend_RateLimitError(t *testing.T) {
	// Mock HTTP server that returns rate limit error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Rate limit exceeded",
		})
	}))
	defer mockServer.Close()

	cfg := config.EmailConfig{
		Enabled:      true,
		Provider:     "resend",
		From:         "test@example.com",
		ResendAPIKey: "test-api-key",
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	client := resend.NewClient("test-api-key")
	baseURL, _ := url.Parse(mockServer.URL)
	client.BaseURL = baseURL

	svc := &Service{
		config:       cfg,
		provider:     "resend",
		resendClient: client,
		logger:       logger,
	}

	ctx := context.Background()
	err := svc.sendViaResend(ctx, "recipient@example.com", "Test Subject", "<html><body>Test Body</body></html>")

	// Should return an error
	if err == nil {
		t.Fatal("Expected rate limit error, got nil")
	}

	// Check error message contains rate limit info
	errMsg := err.Error()
	if !strings.Contains(errMsg, "rate limit") {
		t.Errorf("Expected error message to contain 'rate limit', got: %v", errMsg)
	}

	// Verify it's a rate limit error type
	var rateLimitErr *resend.RateLimitError
	if !errors.As(err, &rateLimitErr) {
		// The wrapped error should contain the rate limit info in the message
		if !strings.Contains(errMsg, "limit") || !strings.Contains(errMsg, "reset") {
			t.Errorf("Expected error to contain rate limit details, got: %v", errMsg)
		}
	}
}

// TestSendViaResend_ContextCancellation verifies context cancellation handling
func TestSendViaResend_ContextCancellation(t *testing.T) {
	// Mock HTTP server with slow response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This handler should not be reached due to context cancellation
		t.Error("Handler should not be called with cancelled context")
	}))
	defer mockServer.Close()

	cfg := config.EmailConfig{
		Enabled:      true,
		Provider:     "resend",
		From:         "test@example.com",
		ResendAPIKey: "test-api-key",
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	client := resend.NewClient("test-api-key")
	baseURL, _ := url.Parse(mockServer.URL)
	client.BaseURL = baseURL

	svc := &Service{
		config:       cfg,
		provider:     "resend",
		resendClient: client,
		logger:       logger,
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := svc.sendViaResend(ctx, "recipient@example.com", "Test Subject", "<html><body>Test Body</body></html>")

	// Should return context cancelled error
	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
	}

	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
}

// TestSendViaResend_NilClient verifies error when Resend client is not initialized
func TestSendViaResend_NilClient(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:      true,
		Provider:     "resend",
		From:         "test@example.com",
		ResendAPIKey: "test-api-key",
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	svc := &Service{
		config:       cfg,
		provider:     "resend",
		resendClient: nil, // Nil client
		logger:       logger,
	}

	ctx := context.Background()
	err := svc.sendViaResend(ctx, "recipient@example.com", "Test Subject", "<html><body>Test Body</body></html>")

	if err == nil {
		t.Fatal("Expected error for nil client, got nil")
	}

	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Expected 'not initialized' error, got: %v", err)
	}
}

// TestSendViaResend_GenericAPIError verifies handling of generic API errors
func TestSendViaResend_GenericAPIError(t *testing.T) {
	// Mock HTTP server that returns a generic error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Invalid request",
			"name":    "validation_error",
		})
	}))
	defer mockServer.Close()

	cfg := config.EmailConfig{
		Enabled:      true,
		Provider:     "resend",
		From:         "test@example.com",
		ResendAPIKey: "test-api-key",
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	client := resend.NewClient("test-api-key")
	baseURL, _ := url.Parse(mockServer.URL)
	client.BaseURL = baseURL

	svc := &Service{
		config:       cfg,
		provider:     "resend",
		resendClient: client,
		logger:       logger,
	}

	ctx := context.Background()
	err := svc.sendViaResend(ctx, "recipient@example.com", "Test Subject", "<html><body>Test Body</body></html>")

	if err == nil {
		t.Fatal("Expected API error, got nil")
	}

	if !strings.Contains(err.Error(), "resend API error") {
		t.Errorf("Expected 'resend API error' in message, got: %v", err)
	}
}
