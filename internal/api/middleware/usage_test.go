package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

// mockUsageRepo is a mock implementation of developers.UsageRepository for testing
type mockUsageRepo struct {
	mu      sync.Mutex
	calls   []usageCall
	failErr error
}

type usageCall struct {
	apiKeyID     pgtype.UUID
	date         time.Time
	requestCount int64
	errorCount   int64
}

func (m *mockUsageRepo) UpsertAPIKeyUsage(ctx context.Context, apiKeyID pgtype.UUID, date time.Time, requestCount, errorCount int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failErr != nil {
		return m.failErr
	}

	m.calls = append(m.calls, usageCall{
		apiKeyID:     apiKeyID,
		date:         date,
		requestCount: requestCount,
		errorCount:   errorCount,
	})
	return nil
}

// contextWithAgentKey adds an API key to the context (for testing)
func contextWithAgentKey(ctx context.Context, key *auth.APIKey) context.Context {
	return context.WithValue(ctx, agentKey, key)
}

func TestUsageTracking_Success(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := developers.NewUsageRecorder(repo, logger)

	// Create test API key
	apiKeyID := uuid.New()
	apiKey := &auth.APIKey{
		ID:   apiKeyID.String(),
		Name: "test-key",
	}

	// Create handler chain
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	middleware := UsageTracking(recorder, logger)
	wrapped := middleware(handler)

	// Create request with API key in context
	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(contextWithAgentKey(req.Context(), apiKey))
	rec := httptest.NewRecorder()

	// Execute
	wrapped.ServeHTTP(rec, req)

	// Check response
	assert.Equal(t, http.StatusOK, rec.Code)

	// Check usage was recorded
	size, requests, errors := recorder.Stats()
	assert.Equal(t, 1, size)
	assert.Equal(t, int64(1), requests)
	assert.Equal(t, int64(0), errors, "2xx should not be counted as error")
}

func TestUsageTracking_Error(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := developers.NewUsageRecorder(repo, logger)

	apiKeyID := uuid.New()
	apiKey := &auth.APIKey{
		ID:   apiKeyID.String(),
		Name: "test-key",
	}

	// Handler that returns an error
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("error"))
	})

	middleware := UsageTracking(recorder, logger)
	wrapped := middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(contextWithAgentKey(req.Context(), apiKey))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Check usage was recorded as error
	size, requests, errors := recorder.Stats()
	assert.Equal(t, 1, size)
	assert.Equal(t, int64(1), requests)
	assert.Equal(t, int64(1), errors, "4xx should be counted as error")
}

func TestUsageTracking_NoAPIKey(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := developers.NewUsageRecorder(repo, logger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := UsageTracking(recorder, logger)
	wrapped := middleware(handler)

	// Request without API key in context
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// No usage should be recorded
	size, _, _ := recorder.Stats()
	assert.Equal(t, 0, size, "should not record usage without API key")
}

func TestUsageTracking_InvalidUUID(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := developers.NewUsageRecorder(repo, logger)

	// API key with invalid UUID
	apiKey := &auth.APIKey{
		ID:   "not-a-uuid",
		Name: "test-key",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := UsageTracking(recorder, logger)
	wrapped := middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(contextWithAgentKey(req.Context(), apiKey))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// No usage should be recorded due to invalid UUID
	size, _, _ := recorder.Stats()
	assert.Equal(t, 0, size, "should not record usage with invalid UUID")
}

func TestUsageTracking_MultipleStatusCodes(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantError bool
	}{
		{"200 OK", http.StatusOK, false},
		{"201 Created", http.StatusCreated, false},
		{"204 No Content", http.StatusNoContent, false},
		{"301 Moved", http.StatusMovedPermanently, false},
		{"400 Bad Request", http.StatusBadRequest, true},
		{"401 Unauthorized", http.StatusUnauthorized, true},
		{"403 Forbidden", http.StatusForbidden, true},
		{"404 Not Found", http.StatusNotFound, true},
		{"500 Internal Error", http.StatusInternalServerError, true},
		{"503 Unavailable", http.StatusServiceUnavailable, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zerolog.Nop()
			repo := &mockUsageRepo{}
			recorder := developers.NewUsageRecorder(repo, logger)

			apiKeyID := uuid.New()
			apiKey := &auth.APIKey{
				ID:   apiKeyID.String(),
				Name: "test-key",
			}

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			})

			middleware := UsageTracking(recorder, logger)
			wrapped := middleware(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			req = req.WithContext(contextWithAgentKey(req.Context(), apiKey))
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			_, requests, errors := recorder.Stats()
			assert.Equal(t, int64(1), requests)
			if tt.wantError {
				assert.Equal(t, int64(1), errors, "status %d should be counted as error", tt.status)
			} else {
				assert.Equal(t, int64(0), errors, "status %d should not be counted as error", tt.status)
			}
		})
	}
}

func TestUsageTracking_ImplicitStatusOK(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := developers.NewUsageRecorder(repo, logger)

	apiKeyID := uuid.New()
	apiKey := &auth.APIKey{
		ID:   apiKeyID.String(),
		Name: "test-key",
	}

	// Handler that doesn't explicitly set status (should default to 200)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("success"))
	})

	middleware := UsageTracking(recorder, logger)
	wrapped := middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(contextWithAgentKey(req.Context(), apiKey))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	_, requests, errors := recorder.Stats()
	assert.Equal(t, int64(1), requests)
	assert.Equal(t, int64(0), errors, "implicit 200 should not be error")
}
