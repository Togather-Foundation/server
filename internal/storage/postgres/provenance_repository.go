package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/provenance"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ provenance.Repository = (*ProvenanceRepository)(nil)

type ProvenanceRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type sourceRow struct {
	ID          string
	Name        string
	SourceType  string
	BaseURL     *string
	LicenseURL  string
	LicenseType string
	TrustLevel  int
	IsActive    bool
	CreatedAt   pgtype.Timestamptz
	UpdatedAt   pgtype.Timestamptz
}

func (r *ProvenanceRepository) GetByBaseURL(ctx context.Context, baseURL string) (*provenance.Source, error) {
	queryer := r.queryer()

	row := queryer.QueryRow(ctx, `
SELECT id, name, source_type, base_url, license_url, license_type, trust_level, is_active, created_at, updated_at
  FROM sources
 WHERE base_url = $1
 LIMIT 1
`, strings.TrimSpace(baseURL))

	var data sourceRow
	if err := row.Scan(
		&data.ID,
		&data.Name,
		&data.SourceType,
		&data.BaseURL,
		&data.LicenseURL,
		&data.LicenseType,
		&data.TrustLevel,
		&data.IsActive,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, provenance.ErrNotFound
		}
		return nil, fmt.Errorf("get source by base url: %w", err)
	}

	return mapSourceRow(data), nil
}

func (r *ProvenanceRepository) Create(ctx context.Context, params provenance.CreateSourceParams) (*provenance.Source, error) {
	queryer := r.queryer()

	row := queryer.QueryRow(ctx, `
INSERT INTO sources (
	name,
	source_type,
	base_url,
	license_url,
	license_type,
	trust_level
) VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6)
RETURNING id, name, source_type, base_url, license_url, license_type, trust_level, is_active, created_at, updated_at
`,
		params.Name,
		params.SourceType,
		params.BaseURL,
		params.LicenseURL,
		params.LicenseType,
		params.TrustLevel,
	)

	var data sourceRow
	if err := row.Scan(
		&data.ID,
		&data.Name,
		&data.SourceType,
		&data.BaseURL,
		&data.LicenseURL,
		&data.LicenseType,
		&data.TrustLevel,
		&data.IsActive,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create source: %w", err)
	}

	return mapSourceRow(data), nil
}

type provenanceQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (r *ProvenanceRepository) queryer() provenanceQueryer {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}

func mapSourceRow(data sourceRow) *provenance.Source {
	source := &provenance.Source{
		ID:          data.ID,
		Name:        data.Name,
		SourceType:  data.SourceType,
		LicenseURL:  data.LicenseURL,
		LicenseType: data.LicenseType,
		TrustLevel:  data.TrustLevel,
		IsActive:    data.IsActive,
		CreatedAt:   time.Time{},
		UpdatedAt:   time.Time{},
	}
	if data.BaseURL != nil {
		source.BaseURL = *data.BaseURL
	}
	if data.CreatedAt.Valid {
		source.CreatedAt = data.CreatedAt.Time
	}
	if data.UpdatedAt.Valid {
		source.UpdatedAt = data.UpdatedAt.Time
	}
	return source
}
