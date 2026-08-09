package developers

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
)

const (
	FlushInterval = 30 * time.Second

	MaxBufferSize = 100
)

type UsageRepository interface {
	UpsertAPIKeyUsage(ctx context.Context, apiKeyID pgtype.UUID, date time.Time, requestCount, errorCount int64) error
	UpsertAPIKeyUsageIP(ctx context.Context, apiKeyID pgtype.UUID, date time.Time, ip netip.Addr, requestCount, errorCount int64) error
}

type usageKey struct {
	apiKeyID uuid.UUID
	ip       string
}

type usageDelta struct {
	requests int64
	errors   int64
}

type UsageRecorder struct {
	mu           sync.Mutex
	counts       map[usageKey]*usageDelta
	repo         UsageRepository
	ticker       *time.Ticker
	done         chan struct{}
	wg           sync.WaitGroup
	maxSize      int
	flushTimeout time.Duration
	logger       zerolog.Logger
	shutdown     sync.Once
	started      bool
}

func NewUsageRecorder(repo UsageRepository, logger zerolog.Logger, cfg config.DeveloperConfig) *UsageRecorder {
	cfg = cfg.WithDefaults()
	flushTimeout := time.Duration(cfg.UsageFlushTimeoutSeconds) * time.Second
	return &UsageRecorder{
		counts:       make(map[usageKey]*usageDelta),
		repo:         repo,
		maxSize:      MaxBufferSize,
		flushTimeout: flushTimeout,
		done:         make(chan struct{}),
		logger:       logger.With().Str("component", "usage_recorder").Logger(),
	}
}

func (r *UsageRecorder) Start() {
	r.mu.Lock()
	if r.ticker != nil {
		r.mu.Unlock()
		return
	}
	r.ticker = time.NewTicker(FlushInterval)
	r.started = true
	r.wg.Add(1)
	r.mu.Unlock()

	go r.flushLoop()
	r.logger.Info().Dur("interval", FlushInterval).Msg("usage recorder started")
}

func (r *UsageRecorder) flushLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ticker.C:
			r.flush()
		case <-r.done:
			r.flush()
			return
		}
	}
}

func (r *UsageRecorder) RecordRequest(apiKeyID uuid.UUID, clientIP string, isError bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	k := usageKey{apiKeyID: apiKeyID, ip: clientIP}
	delta, ok := r.counts[k]
	if !ok {
		delta = &usageDelta{}
		r.counts[k] = delta
	}

	delta.requests++
	if isError {
		delta.errors++
	}

	if len(r.counts) >= r.maxSize {
		r.logger.Debug().Int("size", len(r.counts)).Msg("buffer size limit reached, triggering flush")
		snapshot := r.counts
		r.counts = make(map[usageKey]*usageDelta)
		go r.flushSnapshot(snapshot)
	}
}

func (r *UsageRecorder) flush() {
	r.mu.Lock()
	if len(r.counts) == 0 {
		r.mu.Unlock()
		return
	}

	snapshot := r.counts
	r.counts = make(map[usageKey]*usageDelta)
	r.mu.Unlock()

	r.flushSnapshot(snapshot)
}

func (r *UsageRecorder) flushSnapshot(snapshot map[usageKey]*usageDelta) {
	if len(snapshot) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.flushTimeout)
	defer cancel()

	now := time.Now()
	flushed := 0
	failed := 0

	keyAgg := make(map[uuid.UUID]*usageDelta)

	for k, delta := range snapshot {
		var pgUUID pgtype.UUID
		if err := pgUUID.Scan(k.apiKeyID.String()); err != nil {
			r.logger.Error().
				Err(err).
				Str("api_key_id", k.apiKeyID.String()).
				Msg("failed to convert UUID")
			failed++
			continue
		}

		if k.ip != "" {
			ip, err := netip.ParseAddr(k.ip)
			if err != nil {
				r.logger.Debug().
					Str("ip", k.ip).
					Msg("skipping invalid IP for per-IP usage upsert")
			} else {
				if err := r.repo.UpsertAPIKeyUsageIP(ctx, pgUUID, now, ip, delta.requests, delta.errors); err != nil {
					r.logger.Error().
						Err(err).
						Str("api_key_id", k.apiKeyID.String()).
						Str("ip", k.ip).
						Int64("requests", delta.requests).
						Int64("errors", delta.errors).
						Msg("failed to upsert per-IP usage")
					failed++
					continue
				}
			}
		}

		agg, ok := keyAgg[k.apiKeyID]
		if !ok {
			agg = &usageDelta{}
			keyAgg[k.apiKeyID] = agg
		}
		agg.requests += delta.requests
		agg.errors += delta.errors

		flushed++
	}

	for keyID, delta := range keyAgg {
		var pgUUID pgtype.UUID
		if err := pgUUID.Scan(keyID.String()); err != nil {
			r.logger.Error().Err(err).Str("api_key_id", keyID.String()).Msg("failed to convert UUID for aggregate")
			continue
		}

		if err := r.repo.UpsertAPIKeyUsage(ctx, pgUUID, now, delta.requests, delta.errors); err != nil {
			r.logger.Error().
				Err(err).
				Str("api_key_id", keyID.String()).
				Int64("requests", delta.requests).
				Int64("errors", delta.errors).
				Msg("failed to upsert aggregate usage")
		}
	}

	if flushed > 0 || failed > 0 {
		r.logger.Info().
			Int("flushed", flushed).
			Int("failed", failed).
			Msg("usage buffer flushed")
	}
}

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
			r.wg.Wait()
		} else {
			r.flush()
		}

		r.logger.Info().Msg("usage recorder shutdown")
	})
	return nil
}

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
