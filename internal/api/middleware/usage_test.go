package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/config"
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
	ipCalls []usageIPCall
	failErr error
}

type usageIPCall struct {
	apiKeyID     pgtype.UUID
	ip           netip.Addr
	requestCount int64
	errorCount   int64
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

func (m *mockUsageRepo) UpsertAPIKeyUsageIP(ctx context.Context, apiKeyID pgtype.UUID, date time.Time, ip netip.Addr, requestCount, errorCount int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failErr != nil {
		return m.failErr
	}

	m.ipCalls = append(m.ipCalls, usageIPCall{
		apiKeyID:     apiKeyID,
		ip:           ip,
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
	recorder := developers.NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	// Create test API key
	apiKeyID := uuid.New()
	apiKey := &auth.APIKey{
		ID:   apiKeyID.String(),
		Name: "test-key",
	}

	// Create handler chain
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	middleware := UsageTracking(recorder, logger, nil)
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
	recorder := developers.NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	apiKeyID := uuid.New()
	apiKey := &auth.APIKey{
		ID:   apiKeyID.String(),
		Name: "test-key",
	}

	// Handler that returns an error
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("error"))
	})

	middleware := UsageTracking(recorder, logger, nil)
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
	recorder := developers.NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := UsageTracking(recorder, logger, nil)
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
	recorder := developers.NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	// API key with invalid UUID
	apiKey := &auth.APIKey{
		ID:   "not-a-uuid",
		Name: "test-key",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := UsageTracking(recorder, logger, nil)
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
			recorder := developers.NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

			apiKeyID := uuid.New()
			apiKey := &auth.APIKey{
				ID:   apiKeyID.String(),
				Name: "test-key",
			}

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			})

			middleware := UsageTracking(recorder, logger, nil)
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
	recorder := developers.NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	apiKeyID := uuid.New()
	apiKey := &auth.APIKey{
		ID:   apiKeyID.String(),
		Name: "test-key",
	}

	// Handler that doesn't explicitly set status (should default to 200)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("success"))
	})

	middleware := UsageTracking(recorder, logger, nil)
	wrapped := middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(contextWithAgentKey(req.Context(), apiKey))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	_, requests, errors := recorder.Stats()
	assert.Equal(t, int64(1), requests)
	assert.Equal(t, int64(0), errors, "implicit 200 should not be error")
}

func TestUsageTracking_ClientIP_TrustedProxy(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := developers.NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	apiKeyID := uuid.New()
	apiKey := &auth.APIKey{ID: apiKeyID.String(), Name: "test-key"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := UsageTracking(recorder, logger, []string{"10.0.0.0/8"})
	wrapped := middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.45")
	req = req.WithContext(contextWithAgentKey(req.Context(), apiKey))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)
	_ = recorder.Close()
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Len(t, repo.ipCalls, 1)
	assert.Equal(t, netip.MustParseAddr("203.0.113.45"), repo.ipCalls[0].ip)
}

func TestUsageTracking_ClientIP_Direct(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := developers.NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	apiKeyID := uuid.New()
	apiKey := &auth.APIKey{ID: apiKeyID.String(), Name: "test-key"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := UsageTracking(recorder, logger, nil)
	wrapped := middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req = req.WithContext(contextWithAgentKey(req.Context(), apiKey))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)
	_ = recorder.Close()
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Len(t, repo.ipCalls, 1)
	assert.Equal(t, netip.MustParseAddr("192.168.1.100"), repo.ipCalls[0].ip)
}

func TestUsageTracking_ClientIP_UntrustedHeaderIgnored(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := developers.NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	apiKeyID := uuid.New()
	apiKey := &auth.APIKey{ID: apiKeyID.String(), Name: "test-key"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := UsageTracking(recorder, logger, nil)
	wrapped := middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req = req.WithContext(contextWithAgentKey(req.Context(), apiKey))
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)
	_ = recorder.Close()
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Len(t, repo.ipCalls, 1)
	assert.Equal(t, netip.MustParseAddr("192.168.1.100"), repo.ipCalls[0].ip,
		"should ignore X-Forwarded-For from untrusted source")
}

func TestUsageResponseWriter_Flush(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &usageResponseWriter{ResponseWriter: rr}
	assert.NotPanics(t, func() { w.Flush() }, "Flush should not panic on non-flusher")
}

type flusherRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flusherRecorder) Flush() {
	f.flushed = true
}

func TestUsageResponseWriter_Flush_WithFlusher(t *testing.T) {
	fr := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	w := &usageResponseWriter{ResponseWriter: fr}
	w.Flush()
	assert.True(t, fr.flushed, "Flush should delegate to underlying http.Flusher")
}
