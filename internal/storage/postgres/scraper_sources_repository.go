package postgres

import (
	"context"
	"fmt"

	"github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time interface assertion.
var _ scraper.Repository = (*ScraperSourceRepository)(nil)

// ScraperSourceRepository implements scraper.Repository using PostgreSQL.
type ScraperSourceRepository struct {
	pool *pgxpool.Pool
}

// NewScraperSourceRepository creates a new ScraperSourceRepository.
func NewScraperSourceRepository(pool *pgxpool.Pool) *ScraperSourceRepository {
	return &ScraperSourceRepository{pool: pool}
}

func (r *ScraperSourceRepository) queries() *Queries {
	return &Queries{db: r.pool}
}

// Upsert inserts or updates a scraper source by name.
func (r *ScraperSourceRepository) Upsert(ctx context.Context, params scraper.UpsertParams) (*scraper.Source, error) {
	var lastScraped pgtype.Timestamptz
	if params.LastScrapedAt != nil {
		lastScraped = pgtype.Timestamptz{Time: *params.LastScrapedAt, Valid: true}
	}

	row, err := r.queries().UpsertScraperSource(ctx, UpsertScraperSourceParams{
		Name:          params.Name,
		Url:           params.URL,
		Tier:          int32(params.Tier),
		Schedule:      params.Schedule,
		TrustLevel:    int32(params.TrustLevel),
		License:       params.License,
		Enabled:       params.Enabled,
		MaxPages:      int32(params.MaxPages),
		Selectors:     params.Selectors,
		Notes:         pgtype.Text{String: params.Notes, Valid: params.Notes != ""},
		LastScrapedAt: lastScraped,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert scraper source %q: %w", params.Name, err)
	}

	return rowToSource(row), nil
}

// GetByName returns a scraper source by unique name.
func (r *ScraperSourceRepository) GetByName(ctx context.Context, name string) (*scraper.Source, error) {
	row, err := r.queries().GetScraperSourceByName(ctx, name)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, scraper.ErrNotFound
		}
		return nil, fmt.Errorf("get scraper source %q: %w", name, err)
	}
	return rowToSource(row), nil
}

// List returns all scraper sources, optionally filtered by enabled status.
func (r *ScraperSourceRepository) List(ctx context.Context, enabled *bool) ([]scraper.Source, error) {
	var enabledParam pgtype.Bool
	if enabled != nil {
		enabledParam = pgtype.Bool{Bool: *enabled, Valid: true}
	}

	rows, err := r.queries().ListScraperSources(ctx, enabledParam)
	if err != nil {
		return nil, fmt.Errorf("list scraper sources: %w", err)
	}

	sources := make([]scraper.Source, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, *rowToSource(row))
	}
	return sources, nil
}

// UpdateLastScraped sets last_scraped_at = NOW() for the named source.
func (r *ScraperSourceRepository) UpdateLastScraped(ctx context.Context, name string) error {
	if err := r.queries().UpdateScraperSourceLastScraped(ctx, name); err != nil {
		return fmt.Errorf("update last scraped for %q: %w", name, err)
	}
	return nil
}

// Delete removes a scraper source by name.
func (r *ScraperSourceRepository) Delete(ctx context.Context, name string) error {
	if err := r.queries().DeleteScraperSource(ctx, name); err != nil {
		return fmt.Errorf("delete scraper source %q: %w", name, err)
	}
	return nil
}

// LinkToOrg associates a scraper source with an organization.
func (r *ScraperSourceRepository) LinkToOrg(ctx context.Context, orgID string, sourceID int64) error {
	var uid pgtype.UUID
	if err := uid.Scan(orgID); err != nil {
		return fmt.Errorf("invalid organization ID %q: %w", orgID, err)
	}
	if err := r.queries().LinkOrgScraperSource(ctx, LinkOrgScraperSourceParams{
		OrganizationID:  uid,
		ScraperSourceID: sourceID,
	}); err != nil {
		return fmt.Errorf("link source %d to org %q: %w", sourceID, orgID, err)
	}
	return nil
}

// UnlinkFromOrg removes a source↔org association.
func (r *ScraperSourceRepository) UnlinkFromOrg(ctx context.Context, orgID string, sourceID int64) error {
	var uid pgtype.UUID
	if err := uid.Scan(orgID); err != nil {
		return fmt.Errorf("invalid organization ID %q: %w", orgID, err)
	}
	if err := r.queries().UnlinkOrgScraperSource(ctx, UnlinkOrgScraperSourceParams{
		OrganizationID:  uid,
		ScraperSourceID: sourceID,
	}); err != nil {
		return fmt.Errorf("unlink source %d from org %q: %w", sourceID, orgID, err)
	}
	return nil
}

// ListByOrg returns all scraper sources linked to the given organization UUID.
func (r *ScraperSourceRepository) ListByOrg(ctx context.Context, orgID string) ([]scraper.Source, error) {
	var uid pgtype.UUID
	if err := uid.Scan(orgID); err != nil {
		return nil, fmt.Errorf("invalid organization ID %q: %w", orgID, err)
	}
	rows, err := r.queries().ListScraperSourcesByOrg(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("list scraper sources for org %q: %w", orgID, err)
	}
	sources := make([]scraper.Source, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, *rowToSource(row))
	}
	return sources, nil
}

// LinkToPlace associates a scraper source with a place.
func (r *ScraperSourceRepository) LinkToPlace(ctx context.Context, placeID string, sourceID int64) error {
	var uid pgtype.UUID
	if err := uid.Scan(placeID); err != nil {
		return fmt.Errorf("invalid place ID %q: %w", placeID, err)
	}
	if err := r.queries().LinkPlaceScraperSource(ctx, LinkPlaceScraperSourceParams{
		PlaceID:         uid,
		ScraperSourceID: sourceID,
	}); err != nil {
		return fmt.Errorf("link source %d to place %q: %w", sourceID, placeID, err)
	}
	return nil
}

// UnlinkFromPlace removes a source↔place association.
func (r *ScraperSourceRepository) UnlinkFromPlace(ctx context.Context, placeID string, sourceID int64) error {
	var uid pgtype.UUID
	if err := uid.Scan(placeID); err != nil {
		return fmt.Errorf("invalid place ID %q: %w", placeID, err)
	}
	if err := r.queries().UnlinkPlaceScraperSource(ctx, UnlinkPlaceScraperSourceParams{
		PlaceID:         uid,
		ScraperSourceID: sourceID,
	}); err != nil {
		return fmt.Errorf("unlink source %d from place %q: %w", sourceID, placeID, err)
	}
	return nil
}

// ListByPlace returns all scraper sources linked to the given place UUID.
func (r *ScraperSourceRepository) ListByPlace(ctx context.Context, placeID string) ([]scraper.Source, error) {
	var uid pgtype.UUID
	if err := uid.Scan(placeID); err != nil {
		return nil, fmt.Errorf("invalid place ID %q: %w", placeID, err)
	}
	rows, err := r.queries().ListScraperSourcesByPlace(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("list scraper sources for place %q: %w", placeID, err)
	}
	sources := make([]scraper.Source, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, *rowToSource(row))
	}
	return sources, nil
}

// rowToSource converts a SQLc ScraperSource model to the domain Source type.
func rowToSource(row ScraperSource) *scraper.Source {
	s := &scraper.Source{
		ID:         row.ID,
		Name:       row.Name,
		URL:        row.Url,
		Tier:       int(row.Tier),
		Schedule:   row.Schedule,
		TrustLevel: int(row.TrustLevel),
		License:    row.License,
		Enabled:    row.Enabled,
		MaxPages:   int(row.MaxPages),
		Selectors:  row.Selectors,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
	if row.Notes.Valid {
		s.Notes = row.Notes.String
	}
	if row.LastScrapedAt.Valid {
		t := row.LastScrapedAt.Time
		s.LastScrapedAt = &t
	}
	return s
}
