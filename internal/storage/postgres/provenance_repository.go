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

func (r *ProvenanceRepository) GetEventSources(ctx context.Context, eventID string) ([]provenance.EventSourceAttribution, error) {
	q := New(r.pool)

	// Parse eventID to UUID
	eventUUID, err := parseUUID(eventID)
	if err != nil {
		return nil, fmt.Errorf("invalid event ID: %w", err)
	}

	rows, err := q.GetEventSources(ctx, eventUUID)
	if err != nil {
		return nil, fmt.Errorf("get event sources: %w", err)
	}

	result := make([]provenance.EventSourceAttribution, 0, len(rows))
	for _, row := range rows {
		attr := provenance.EventSourceAttribution{
			SourceID:    uuidToString(row.SourceID),
			SourceName:  row.SourceName,
			SourceType:  row.SourceType,
			SourceURL:   row.SourceUrl,
			TrustLevel:  int(row.TrustLevel),
			LicenseURL:  row.LicenseUrl,
			LicenseType: row.LicenseType,
			RetrievedAt: row.RetrievedAt.Time,
		}

		if row.Confidence.Valid {
			val := float64(row.Confidence.Int.Int64()) / 100.0
			attr.Confidence = &val
		}

		if row.SourceEventID.Valid {
			attr.SourceEventID = &row.SourceEventID.String
		}

		result = append(result, attr)
	}

	return result, nil
}

func (r *ProvenanceRepository) GetFieldProvenance(ctx context.Context, eventID string) ([]provenance.FieldProvenanceInfo, error) {
	q := New(r.pool)

	eventUUID, err := parseUUID(eventID)
	if err != nil {
		return nil, fmt.Errorf("invalid event ID: %w", err)
	}

	rows, err := q.GetFieldProvenance(ctx, eventUUID)
	if err != nil {
		return nil, fmt.Errorf("get field provenance: %w", err)
	}

	return mapFieldProvenanceRows(rows), nil
}

func (r *ProvenanceRepository) GetFieldProvenanceForPaths(ctx context.Context, eventID string, fieldPaths []string) ([]provenance.FieldProvenanceInfo, error) {
	q := New(r.pool)

	eventUUID, err := parseUUID(eventID)
	if err != nil {
		return nil, fmt.Errorf("invalid event ID: %w", err)
	}

	rows, err := q.GetFieldProvenanceForPaths(ctx, GetFieldProvenanceForPathsParams{
		EventID: eventUUID,
		Column2: fieldPaths,
	})
	if err != nil {
		return nil, fmt.Errorf("get field provenance for paths: %w", err)
	}

	return mapFieldProvenanceRowsFromPaths(rows), nil
}

func (r *ProvenanceRepository) GetCanonicalFieldValue(ctx context.Context, eventID string, fieldPath string) (*provenance.FieldProvenanceInfo, error) {
	q := New(r.pool)

	eventUUID, err := parseUUID(eventID)
	if err != nil {
		return nil, fmt.Errorf("invalid event ID: %w", err)
	}

	row, err := q.GetCanonicalFieldValue(ctx, GetCanonicalFieldValueParams{
		EventID:   eventUUID,
		FieldPath: fieldPath,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get canonical field value: %w", err)
	}

	info := provenance.FieldProvenanceInfo{
		FieldPath:   row.FieldPath,
		SourceID:    uuidToString(row.SourceID),
		SourceName:  row.SourceName,
		SourceType:  row.SourceType,
		TrustLevel:  int(row.TrustLevel),
		LicenseURL:  row.LicenseUrl,
		LicenseType: row.LicenseType,
		Confidence:  float64(row.Confidence.Int.Int64()) / 100.0,
		ObservedAt:  row.ObservedAt.Time,
	}

	if row.ValuePreview.Valid {
		info.ValuePreview = &row.ValuePreview.String
	}

	return &info, nil
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

func mapFieldProvenanceRows(rows []GetFieldProvenanceRow) []provenance.FieldProvenanceInfo {
	result := make([]provenance.FieldProvenanceInfo, 0, len(rows))
	for _, row := range rows {
		info := provenance.FieldProvenanceInfo{
			FieldPath:   row.FieldPath,
			SourceID:    uuidToString(row.SourceID),
			SourceName:  row.SourceName,
			SourceType:  row.SourceType,
			TrustLevel:  int(row.TrustLevel),
			LicenseURL:  row.LicenseUrl,
			LicenseType: row.LicenseType,
			Confidence:  float64(row.Confidence.Int.Int64()) / 100.0,
			ObservedAt:  row.ObservedAt.Time,
		}

		if row.ValuePreview.Valid {
			info.ValuePreview = &row.ValuePreview.String
		}

		result = append(result, info)
	}
	return result
}

func mapFieldProvenanceRowsFromPaths(rows []GetFieldProvenanceForPathsRow) []provenance.FieldProvenanceInfo {
	result := make([]provenance.FieldProvenanceInfo, 0, len(rows))
	for _, row := range rows {
		info := provenance.FieldProvenanceInfo{
			FieldPath:   row.FieldPath,
			SourceID:    uuidToString(row.SourceID),
			SourceName:  row.SourceName,
			SourceType:  row.SourceType,
			TrustLevel:  int(row.TrustLevel),
			LicenseURL:  row.LicenseUrl,
			LicenseType: row.LicenseType,
			Confidence:  float64(row.Confidence.Int.Int64()) / 100.0,
			ObservedAt:  row.ObservedAt.Time,
		}

		if row.ValuePreview.Valid {
			info.ValuePreview = &row.ValuePreview.String
		}

		result = append(result, info)
	}
	return result
}

func parseUUID(s string) (pgtype.UUID, error) {
	var uuid pgtype.UUID
	if err := uuid.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return uuid, nil
}

func uuidToString(uuid pgtype.UUID) string {
	if !uuid.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid.Bytes[0:4],
		uuid.Bytes[4:6],
		uuid.Bytes[6:8],
		uuid.Bytes[8:10],
		uuid.Bytes[10:16])
}
