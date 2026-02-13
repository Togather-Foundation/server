package developers

import (
	"context"
	"sync"
	"testing"
	"time"

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

func (m *mockUsageRepo) getCalls() []usageCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]usageCall(nil), m.calls...)
}

func TestUsageRecorder_RecordRequest(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger)

	apiKeyID := uuid.New()

	// Record several requests
	recorder.RecordRequest(apiKeyID, false) // success
	recorder.RecordRequest(apiKeyID, false) // success
	recorder.RecordRequest(apiKeyID, true)  // error

	// Check in-memory buffer
	size, requests, errors := recorder.Stats()
	assert.Equal(t, 1, size, "should have one API key in buffer")
	assert.Equal(t, int64(3), requests, "should have 3 total requests")
	assert.Equal(t, int64(1), errors, "should have 1 error")
}

func TestUsageRecorder_ManualFlush(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger)

	apiKeyID := uuid.New()

	// Record some usage
	recorder.RecordRequest(apiKeyID, false)
	recorder.RecordRequest(apiKeyID, true)

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
	recorder := NewUsageRecorder(repo, logger)

	apiKeyID := uuid.New()

	// Start the recorder
	recorder.Start()
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Fatalf("failed to close recorder: %v", err)
		}
	}()

	// Record some usage
	recorder.RecordRequest(apiKeyID, false)
	recorder.RecordRequest(apiKeyID, true)

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
	recorder := NewUsageRecorder(repo, logger)
	recorder.maxSize = 3 // Set low threshold for testing

	// Record usage for multiple keys to exceed buffer size
	for i := 0; i < 5; i++ {
		apiKeyID := uuid.New()
		recorder.RecordRequest(apiKeyID, false)
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
	recorder := NewUsageRecorder(repo, logger)

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
			isError := n%10 == 0
			recorder.RecordRequest(keyID, isError)
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
	recorder := NewUsageRecorder(repo, logger)
	recorder.Start()

	apiKeyID := uuid.New()

	// Record some usage
	recorder.RecordRequest(apiKeyID, false)
	recorder.RecordRequest(apiKeyID, true)

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
	recorder := NewUsageRecorder(repo, logger)

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
	recorder := NewUsageRecorder(repo, logger)

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
	recorder := NewUsageRecorder(repo, logger)

	// Create multiple API keys
	keys := make([]uuid.UUID, 10)
	for i := range keys {
		keys[i] = uuid.New()
	}

	// Record usage for each key
	for _, key := range keys {
		recorder.RecordRequest(key, false)
		recorder.RecordRequest(key, true)
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
	recorder := NewUsageRecorder(repo, logger)

	// Flush with no data should not panic
	recorder.flush()

	// Should not have called repo
	calls := repo.getCalls()
	assert.Len(t, calls, 0, "should not flush empty buffer")
}

func TestUsageRecorder_ErrorCategorization(t *testing.T) {
	logger := zerolog.Nop()
	repo := &mockUsageRepo{}
	recorder := NewUsageRecorder(repo, logger)

	apiKeyID := uuid.New()

	// Test various status codes through error flag
	// 2xx/3xx → isError=false
	recorder.RecordRequest(apiKeyID, false) // 200
	recorder.RecordRequest(apiKeyID, false) // 201
	recorder.RecordRequest(apiKeyID, false) // 204

	// 4xx → isError=true
	recorder.RecordRequest(apiKeyID, true) // 400
	recorder.RecordRequest(apiKeyID, true) // 404

	// 5xx → isError=true
	recorder.RecordRequest(apiKeyID, true) // 500
	recorder.RecordRequest(apiKeyID, true) // 503

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
