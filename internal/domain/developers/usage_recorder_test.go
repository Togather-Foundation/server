package developers

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUsageRepo is a mock implementation of UsageRepository for testing
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
	clientErrors int64
	serverErrors int64
}

func (m *mockUsageRepo) UpsertAPIKeyUsage(ctx context.Context, apiKeyID pgtype.UUID, date time.Time, requestCount, errorCount, clientErrorCount, serverErrorCount int64) error {
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
		clientErrors: clientErrorCount,
		serverErrors: serverErrorCount,
	})
	return nil
}

func (m *mockUsageRepo) UpsertAPIKeyUsageIP(ctx context.Context, apiKeyID pgtype.UUID, date time.Time, ip netip.Addr, requestCount, errorCount, clientErrorCount, serverErrorCount int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failErr != nil {
		return m.failErr
	}
	return nil
}

func (m *mockUsageRepo) getCalls() []usageCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]usageCall(nil), m.calls...)
}

func TestUsageRecorder_RecordRequest(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	apiKeyID := uuid.New()

	// Record several requests
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 200) // success
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 200) // success
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 500) // error

	// Check in-memory buffer
	size, requests, errors := recorder.Stats()
	assert.Equal(t, 1, size, "should have one API key in buffer")
	assert.Equal(t, int64(3), requests, "should have 3 total requests")
	assert.Equal(t, int64(1), errors, "should have 1 error")
}

func TestUsageRecorder_ManualFlush(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	apiKeyID := uuid.New()

	// Record some usage
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 200)
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 500)

	// Flush manually
	recorder.flush()

	// Wait a bit for async flush to complete
	time.Sleep(50 * time.Millisecond)

	// Check that data was flushed to repo
	calls := repo.getCalls()
	require.Len(t, calls, 1, "should have flushed one API key")
	assert.Equal(t, int64(2), calls[0].requestCount)
	assert.Equal(t, int64(1), calls[0].errorCount)

	// Check that buffer is empty
	size, _, _ := recorder.Stats()
	assert.Equal(t, 0, size, "buffer should be empty after flush")
}

func TestUsageRecorder_PeriodicFlush(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping periodic flush test in short mode")
	}

	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	apiKeyID := uuid.New()

	// Start the recorder
	recorder.Start()
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Fatalf("failed to close recorder: %v", err)
		}
	}()

	// Record some usage
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 200)
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 500)

	// Wait for periodic flush (30 seconds is too long for tests, so we manually flush)
	recorder.flush()
	time.Sleep(100 * time.Millisecond)

	// Check that data was flushed
	calls := repo.getCalls()
	require.GreaterOrEqual(t, len(calls), 1, "should have flushed at least once")
}

func TestUsageRecorder_SizeBasedFlush(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})
	recorder.maxSize = 3 // Set low threshold for testing

	// Record usage for multiple keys to exceed buffer size
	for i := 0; i < 5; i++ {
		apiKeyID := uuid.New()
		recorder.RecordRequest(apiKeyID, "1.2.3.4", 200)
	}

	// Give the async flush a moment to complete
	time.Sleep(100 * time.Millisecond)

	// Check that flush was triggered
	calls := repo.getCalls()
	assert.Greater(t, len(calls), 0, "should have triggered size-based flush")
}

func TestUsageRecorder_ConcurrentAccess(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	apiKeyID1 := uuid.New()
	apiKeyID2 := uuid.New()

	// Simulate concurrent requests from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			keyID := apiKeyID1
			if n%2 == 0 {
				keyID = apiKeyID2
			}
			statusCode := 200
			if n%10 == 0 {
				statusCode = 500
			}
			recorder.RecordRequest(keyID, "1.2.3.4", statusCode)
		}(i)
	}

	wg.Wait()

	// Check stats (should be safe with race detector)
	size, requests, errors := recorder.Stats()
	assert.Equal(t, 2, size, "should have two API keys")
	assert.Equal(t, int64(100), requests, "should have 100 requests")
	assert.Equal(t, int64(10), errors, "should have 10 errors")
}

func TestUsageRecorder_GracefulShutdown(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})
	recorder.Start()

	apiKeyID := uuid.New()

	// Record some usage
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 200)
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 500)

	// Close should flush remaining buffer and block until complete
	err := recorder.Close()
	require.NoError(t, err)

	// Check that data was flushed on close
	calls := repo.getCalls()
	require.Len(t, calls, 1, "should have flushed on close")
	assert.Equal(t, int64(2), calls[0].requestCount)
	assert.Equal(t, int64(1), calls[0].errorCount)
}

func TestUsageRecorder_MultipleClose(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	// Multiple closes should not panic
	err1 := recorder.Close()
	err2 := recorder.Close()
	err3 := recorder.Close()

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NoError(t, err3)
}

func TestUsageRecorder_MultipleStart(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	// Multiple starts should not panic or create multiple goroutines
	recorder.Start()
	recorder.Start()
	recorder.Start()

	if err := recorder.Close(); err != nil {
		t.Fatalf("failed to close recorder: %v", err)
	}
}

func TestUsageRecorder_MultipleAPIKeys(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	// Create multiple API keys
	keys := make([]uuid.UUID, 10)
	for i := range keys {
		keys[i] = uuid.New()
	}

	// Record usage for each key
	for _, key := range keys {
		recorder.RecordRequest(key, "1.2.3.4", 200)
		recorder.RecordRequest(key, "1.2.3.4", 500)
	}

	// Flush
	recorder.flush()
	time.Sleep(100 * time.Millisecond)

	// Check that all keys were flushed
	calls := repo.getCalls()
	assert.Len(t, calls, 10, "should have flushed all 10 keys")

	// Verify counts
	for _, call := range calls {
		assert.Equal(t, int64(2), call.requestCount)
		assert.Equal(t, int64(1), call.errorCount)
	}
}

func TestUsageRecorder_EmptyFlush(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	// Flush with no data should not panic
	recorder.flush()

	// Should not have called repo
	calls := repo.getCalls()
	assert.Len(t, calls, 0, "should not flush empty buffer")
}

func TestUsageRecorder_ErrorCategorization(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger, config.DeveloperConfig{UsageFlushTimeoutSeconds: 10})

	apiKeyID := uuid.New()

	// Test successful status codes.
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 200) // 200
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 201) // 201
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 204) // 204

	// 4xx are client errors.
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 400) // 400
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 404) // 404

	// 5xx are server errors.
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 500) // 500
	recorder.RecordRequest(apiKeyID, "1.2.3.4", 503) // 503

	// Check stats
	size, requests, errors := recorder.Stats()
	assert.Equal(t, 1, size, "should have one API key")
	assert.Equal(t, int64(7), requests, "should have 7 total requests")
	assert.Equal(t, int64(4), errors, "should have 4 errors (4xx and 5xx)")

	// Flush and verify
	recorder.flush()
	time.Sleep(50 * time.Millisecond)

	calls := repo.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, int64(7), calls[0].requestCount)
	assert.Equal(t, int64(4), calls[0].errorCount)
}

func TestUsageRecorder_StatusClassBoundaries(t *testing.T) {
	for _, tt := range []struct {
		code           int
		client, server int64
	}{{200, 0, 0}, {302, 0, 0}, {400, 1, 0}, {429, 1, 0}, {499, 1, 0}, {500, 0, 1}, {599, 0, 1}} {
		t.Run(fmt.Sprint(tt.code), func(t *testing.T) {
			repo := &mockUsageRepo{}
			r := NewUsageRecorder(repo, zerolog.Nop(), config.DeveloperConfig{})
			key := uuid.New()
			r.RecordRequest(key, "1.2.3.4", tt.code)
			r.flush()
			calls := repo.getCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, tt.client, calls[0].clientErrors)
			assert.Equal(t, tt.server, calls[0].serverErrors)
			assert.Equal(t, tt.client+tt.server, calls[0].errorCount)
		})
	}
}
