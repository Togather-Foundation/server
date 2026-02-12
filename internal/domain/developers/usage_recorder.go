package developers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
)

const (
	// FlushInterval is how often the usage buffer is flushed to the database
	FlushInterval = 30 * time.Second

	// MaxBufferSize is the maximum number of API keys in the buffer before forcing a flush
	MaxBufferSize = 100
)

// UsageRepository defines the interface for persisting usage data
type UsageRepository interface {
	UpsertAPIKeyUsage(ctx context.Context, apiKeyID pgtype.UUID, date time.Time, requestCount, errorCount int64) error
}

// usageDelta tracks accumulated request and error counts for a single API key
type usageDelta struct {
	requests int64
	errors   int64
}

// UsageRecorder buffers API key usage metrics in memory and periodically flushes them to the database.
// It's safe for concurrent use.
type UsageRecorder struct {
	mu       sync.Mutex
	counts   map[uuid.UUID]*usageDelta
	repo     UsageRepository
	ticker   *time.Ticker
	done     chan struct{}
	wg       sync.WaitGroup
	maxSize  int
	logger   zerolog.Logger
	shutdown sync.Once
	started  bool
}

// NewUsageRecorder creates a new UsageRecorder instance.
// Call Start() to begin the background flush goroutine.
func NewUsageRecorder(repo UsageRepository, logger zerolog.Logger) *UsageRecorder {
	return &UsageRecorder{
		counts:  make(map[uuid.UUID]*usageDelta),
		repo:    repo,
		maxSize: MaxBufferSize,
		done:    make(chan struct{}),
		logger:  logger.With().Str("component", "usage_recorder").Logger(),
	}
}

// Start begins the background goroutine that periodically flushes buffered usage to the database.
// It's safe to call Start multiple times (subsequent calls are no-ops).
func (r *UsageRecorder) Start() {
	r.mu.Lock()
	if r.ticker != nil {
		r.mu.Unlock()
		return // already started
	}
	r.ticker = time.NewTicker(FlushInterval)
	r.started = true
	r.wg.Add(1)
	r.mu.Unlock()

	go r.flushLoop()
	r.logger.Info().Dur("interval", FlushInterval).Msg("usage recorder started")
}

// flushLoop runs in a background goroutine and flushes the buffer on a timer or when done is closed
func (r *UsageRecorder) flushLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ticker.C:
			r.flush()
		case <-r.done:
			r.flush() // final flush on shutdown
			return
		}
	}
}

// RecordRequest increments the usage counters for the given API key.
// isError should be true for HTTP 4xx/5xx responses.
// This method is safe for concurrent use.
func (r *UsageRecorder) RecordRequest(apiKeyID uuid.UUID, isError bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delta, ok := r.counts[apiKeyID]
	if !ok {
		delta = &usageDelta{}
		r.counts[apiKeyID] = delta
	}

	delta.requests++
	if isError {
		delta.errors++
	}

	// Check if buffer size exceeded and trigger flush if needed
	if len(r.counts) >= r.maxSize {
		r.logger.Debug().Int("size", len(r.counts)).Msg("buffer size limit reached, triggering flush")
		// Swap the buffer under the lock to avoid race condition
		snapshot := r.counts
		r.counts = make(map[uuid.UUID]*usageDelta)
		// Spawn goroutine with the swapped buffer (no lock needed)
		go r.flushSnapshot(snapshot)
	}
}

// flush writes all buffered usage data to the database and clears the buffer.
// This method acquires the lock, swaps the buffer, and delegates to flushSnapshot.
func (r *UsageRecorder) flush() {
	r.mu.Lock()
	if len(r.counts) == 0 {
		r.mu.Unlock()
		return
	}

	// Swap the buffer to minimize lock hold time
	snapshot := r.counts
	r.counts = make(map[uuid.UUID]*usageDelta)
	r.mu.Unlock()

	r.flushSnapshot(snapshot)
}

// flushSnapshot writes the given usage snapshot to the database without acquiring the lock.
// This allows callers to swap the buffer before calling, avoiding lock contention.
func (r *UsageRecorder) flushSnapshot(snapshot map[uuid.UUID]*usageDelta) {
	if len(snapshot) == 0 {
		return
	}

	// Use background context since the original request context is gone
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	flushed := 0
	failed := 0

	for keyID, delta := range snapshot {
		// Convert uuid.UUID to pgtype.UUID
		var pgUUID pgtype.UUID
		if err := pgUUID.Scan(keyID.String()); err != nil {
			r.logger.Error().
				Err(err).
				Str("api_key_id", keyID.String()).
				Msg("failed to convert UUID")
			failed++
			continue
		}

		if err := r.repo.UpsertAPIKeyUsage(ctx, pgUUID, now, delta.requests, delta.errors); err != nil {
			r.logger.Error().
				Err(err).
				Str("api_key_id", keyID.String()).
				Int64("requests", delta.requests).
				Int64("errors", delta.errors).
				Msg("failed to upsert usage")
			failed++
			continue
		}
		flushed++
	}

	if flushed > 0 || failed > 0 {
		r.logger.Info().
			Int("flushed", flushed).
			Int("failed", failed).
			Msg("usage buffer flushed")
	}
}

// Close gracefully shuts down the usage recorder, flushing any remaining buffered data.
// If Start() was called, it blocks until the flush loop exits. It's safe to call Close multiple times.
func (r *UsageRecorder) Close() error {
	r.shutdown.Do(func() {
		r.mu.Lock()
		wasStarted := r.started
		if r.ticker != nil {
			r.ticker.Stop()
		}
		r.mu.Unlock()

		if wasStarted {
			close(r.done)
			r.wg.Wait() // Wait for flush loop to exit after final flush
		} else {
			// If not started, just do a synchronous flush
			r.flush()
		}

		r.logger.Info().Msg("usage recorder shutdown")
	})
	return nil
}

// Stats returns the current buffer statistics (for testing/debugging)
func (r *UsageRecorder) Stats() (bufferSize int, totalRequests int64, totalErrors int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	bufferSize = len(r.counts)
	for _, delta := range r.counts {
		totalRequests += delta.requests
		totalErrors += delta.errors
	}
	return
}

// parseUUID converts a string UUID to uuid.UUID, returning an error if invalid
func parseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("invalid uuid: %w", err)
	}
	return id, nil
}
