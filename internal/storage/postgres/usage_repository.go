package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UsageRepository handles API key usage tracking operations
type UsageRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

// NewUsageRepository creates a new UsageRepository
func NewUsageRepository(pool *pgxpool.Pool) *UsageRepository {
	return &UsageRepository{pool: pool}
}

// queryer returns the appropriate database interface (transaction or pool)
func (r *UsageRepository) queryer() Querier {
	if r.tx != nil {
		return &Queries{db: r.tx}
	}
	return &Queries{db: r.pool}
}

// UpsertAPIKeyUsage upserts usage stats for an API key on a given date
// This increments counters if a row already exists for the date
func (r *UsageRepository) UpsertAPIKeyUsage(ctx context.Context, apiKeyID pgtype.UUID, date time.Time, requestCount, errorCount int64) error {
	q := r.queryer()

	params := UpsertAPIKeyUsageParams{
		ApiKeyID:     apiKeyID,
		Date:         pgtype.Date{Time: date, Valid: true},
		RequestCount: requestCount,
		ErrorCount:   errorCount,
	}

	if err := q.UpsertAPIKeyUsage(ctx, params); err != nil {
		return fmt.Errorf("upsert api key usage: %w", err)
	}
	return nil
}

// GetAPIKeyUsage retrieves usage records for an API key within a date range
func (r *UsageRepository) GetAPIKeyUsage(ctx context.Context, apiKeyID pgtype.UUID, startDate, endDate time.Time) ([]ApiKeyUsage, error) {
	q := r.queryer()

	params := GetAPIKeyUsageParams{
		ApiKeyID: apiKeyID,
		Date:     pgtype.Date{Time: startDate, Valid: true},
		Date_2:   pgtype.Date{Time: endDate, Valid: true},
	}

	records, err := q.GetAPIKeyUsage(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("get api key usage: %w", err)
	}
	return records, nil
}

// GetAPIKeyUsageTotal retrieves aggregated usage totals for an API key within a date range
func (r *UsageRepository) GetAPIKeyUsageTotal(ctx context.Context, apiKeyID pgtype.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error) {
	q := r.queryer()

	params := GetAPIKeyUsageTotalParams{
		ApiKeyID: apiKeyID,
		Date:     pgtype.Date{Time: startDate, Valid: true},
		Date_2:   pgtype.Date{Time: endDate, Valid: true},
	}

	result, err := q.GetAPIKeyUsageTotal(ctx, params)
	if err != nil {
		return 0, 0, fmt.Errorf("get api key usage total: %w", err)
	}
	return result.TotalRequests, result.TotalErrors, nil
}

// GetDeveloperUsageTotal retrieves aggregated usage totals across all API keys for a developer within a date range
func (r *UsageRepository) GetDeveloperUsageTotal(ctx context.Context, developerID pgtype.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error) {
	q := r.queryer()

	params := GetDeveloperUsageTotalParams{
		DeveloperID: developerID,
		Date:        pgtype.Date{Time: startDate, Valid: true},
		Date_2:      pgtype.Date{Time: endDate, Valid: true},
	}

	result, err := q.GetDeveloperUsageTotal(ctx, params)
	if err != nil {
		return 0, 0, fmt.Errorf("get developer usage total: %w", err)
	}
	return result.TotalRequests, result.TotalErrors, nil
}

// WithTx returns a new repository instance that will use the provided transaction
func (r *UsageRepository) WithTx(tx pgx.Tx) *UsageRepository {
	return &UsageRepository{
		pool: r.pool,
		tx:   tx,
	}
}
