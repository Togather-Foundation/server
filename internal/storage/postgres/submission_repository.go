package postgres

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time interface assertion.
var _ scraper.SubmissionRepository = (*ScraperSubmissionRepository)(nil)

// ScraperSubmissionRepository implements scraper.SubmissionRepository using PostgreSQL.
type ScraperSubmissionRepository struct {
	pool *pgxpool.Pool
}

// NewScraperSubmissionRepository creates a new ScraperSubmissionRepository.
func NewScraperSubmissionRepository(pool *pgxpool.Pool) *ScraperSubmissionRepository {
	return &ScraperSubmissionRepository{pool: pool}
}

func (r *ScraperSubmissionRepository) queries() *Queries {
	return &Queries{db: r.pool}
}

// GetRecentByURLNorm returns a submission for the given url_norm within the 30-day dedup window.
// Returns nil (no error) when no recent submission is found.
func (r *ScraperSubmissionRepository) GetRecentByURLNorm(ctx context.Context, urlNorm string) (*scraper.Submission, error) {
	row, err := r.queries().GetRecentSubmissionByURLNorm(ctx, GetRecentSubmissionByURLNormParams{
		UrlNorm:  urlNorm,
		Interval: pgtype.Interval{Days: 30, Valid: true},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get recent submission by url_norm %q: %w", urlNorm, err)
	}
	return rowToSubmission(row), nil
}

// CountRecentByIP returns the number of URLs submitted from the given IP in the last 24 hours.
func (r *ScraperSubmissionRepository) CountRecentByIP(ctx context.Context, ip string) (int64, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return 0, fmt.Errorf("parse submitter IP %q: %w", ip, err)
	}
	// 24 hours expressed as microseconds (pgtype.Interval has no Hours field).
	const microsPerHour = int64(3600 * 1_000_000)
	count, err := r.queries().CountRecentSubmissionsByIP(ctx, CountRecentSubmissionsByIPParams{
		SubmitterIp: addr,
		Interval:    pgtype.Interval{Microseconds: 24 * microsPerHour, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("count recent submissions by IP: %w", err)
	}
	return count, nil
}

// Insert stores a new submission and returns the stored row.
func (r *ScraperSubmissionRepository) Insert(ctx context.Context, sub *scraper.Submission) (*scraper.Submission, error) {
	addr, err := netip.ParseAddr(sub.SubmitterIP)
	if err != nil {
		return nil, fmt.Errorf("parse submitter IP %q: %w", sub.SubmitterIP, err)
	}
	row, err := r.queries().InsertScraperSubmission(ctx, InsertScraperSubmissionParams{
		Url:         sub.URL,
		UrlNorm:     sub.URLNorm,
		SubmitterIp: addr,
	})
	if err != nil {
		return nil, fmt.Errorf("insert scraper submission: %w", err)
	}
	return rowToSubmission(row), nil
}

// ListPendingValidation returns up to limit rows with status pending_validation, oldest first.
func (r *ScraperSubmissionRepository) ListPendingValidation(ctx context.Context, limit int) ([]*scraper.Submission, error) {
	rows, err := r.queries().ListPendingValidation(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("list pending validation: %w", err)
	}
	subs := make([]*scraper.Submission, 0, len(rows))
	for _, row := range rows {
		subs = append(subs, rowToSubmission(row))
	}
	return subs, nil
}

// CountPendingValidation returns the count of pending_validation rows.
func (r *ScraperSubmissionRepository) CountPendingValidation(ctx context.Context) (int64, error) {
	count, err := r.queries().CountPendingValidation(ctx)
	if err != nil {
		return 0, fmt.Errorf("count pending validation: %w", err)
	}
	return count, nil
}

// UpdateStatus updates status, rejection_reason, and validated_at for a submission.
func (r *ScraperSubmissionRepository) UpdateStatus(ctx context.Context, id int64, status string, rejectionReason *string, validatedAt *time.Time) error {
	var reason pgtype.Text
	if rejectionReason != nil {
		reason = pgtype.Text{String: *rejectionReason, Valid: true}
	}
	var vAt pgtype.Timestamptz
	if validatedAt != nil {
		vAt = pgtype.Timestamptz{Time: *validatedAt, Valid: true}
	}
	if err := r.queries().UpdateSubmissionStatus(ctx, UpdateSubmissionStatusParams{
		ID:              id,
		Status:          status,
		RejectionReason: reason,
		ValidatedAt:     vAt,
	}); err != nil {
		return fmt.Errorf("update submission status id=%d: %w", id, err)
	}
	return nil
}

// UpdateAdminReview updates status and notes for a submission, returning the updated row.
func (r *ScraperSubmissionRepository) UpdateAdminReview(ctx context.Context, id int64, status string, notes *string) (*scraper.Submission, error) {
	var n pgtype.Text
	if notes != nil {
		n = pgtype.Text{String: *notes, Valid: true}
	}
	row, err := r.queries().UpdateSubmissionAdminReview(ctx, UpdateSubmissionAdminReviewParams{
		ID:     id,
		Status: status,
		Notes:  n,
	})
	if err != nil {
		return nil, fmt.Errorf("update admin review id=%d: %w", id, err)
	}
	return rowToSubmission(row), nil
}

// List returns a paginated list of submissions with optional status filter.
func (r *ScraperSubmissionRepository) List(ctx context.Context, status *string, limit, offset int) ([]*scraper.Submission, error) {
	var statusParam pgtype.Text
	if status != nil {
		statusParam = pgtype.Text{String: *status, Valid: true}
	}
	rows, err := r.queries().ListScraperSubmissions(ctx, ListScraperSubmissionsParams{
		Status: statusParam,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list scraper submissions: %w", err)
	}
	subs := make([]*scraper.Submission, 0, len(rows))
	for _, row := range rows {
		subs = append(subs, rowToSubmission(row))
	}
	return subs, nil
}

// Count returns the total count of submissions with optional status filter.
func (r *ScraperSubmissionRepository) Count(ctx context.Context, status *string) (int64, error) {
	var statusParam pgtype.Text
	if status != nil {
		statusParam = pgtype.Text{String: *status, Valid: true}
	}
	count, err := r.queries().CountScraperSubmissions(ctx, statusParam)
	if err != nil {
		return 0, fmt.Errorf("count scraper submissions: %w", err)
	}
	return count, nil
}

// rowToSubmission converts a generated ScraperSubmission row to the domain type.
func rowToSubmission(row ScraperSubmission) *scraper.Submission {
	sub := &scraper.Submission{
		ID:          row.ID,
		URL:         row.Url,
		URLNorm:     row.UrlNorm,
		SubmitterIP: row.SubmitterIp.String(),
		Status:      row.Status,
	}
	if row.SubmittedAt.Valid {
		sub.SubmittedAt = row.SubmittedAt.Time
	}
	if row.RejectionReason.Valid {
		s := row.RejectionReason.String
		sub.RejectionReason = &s
	}
	if row.Notes.Valid {
		s := row.Notes.String
		sub.Notes = &s
	}
	if row.ValidatedAt.Valid {
		t := row.ValidatedAt.Time
		sub.ValidatedAt = &t
	}
	return sub
}
