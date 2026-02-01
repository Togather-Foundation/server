package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/provenance"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestProvenanceRepositoryCreateAndGetByBaseURL(t *testing.T) {
	ctx := context.Background()
	container, dbURL := setupPostgres(t, ctx)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	repo := &ProvenanceRepository{pool: pool}

	created, err := repo.Create(ctx, provenance.CreateSourceParams{
		Name:        "Example Source",
		SourceType:  "api",
		BaseURL:     "https://source.example.org",
		LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
		LicenseType: "CC0",
		TrustLevel:  8,
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, "Example Source", created.Name)
	require.Equal(t, "https://source.example.org", created.BaseURL)

	loaded, err := repo.GetByBaseURL(ctx, "https://source.example.org")
	require.NoError(t, err)
	require.Equal(t, created.ID, loaded.ID)
	require.Equal(t, created.Name, loaded.Name)
}

func TestProvenanceRepositoryGetEventSourcesAndFields(t *testing.T) {
	ctx := context.Background()
	container, dbURL := setupPostgres(t, ctx)
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	defer pool.Close()

	repo := &ProvenanceRepository{pool: pool}

	source, err := repo.Create(ctx, provenance.CreateSourceParams{
		Name:        "Catalog",
		SourceType:  "api",
		BaseURL:     "https://catalog.example.org",
		LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
		LicenseType: "CC0",
		TrustLevel:  7,
	})
	require.NoError(t, err)

	org := insertOrganization(t, ctx, pool, "Toronto Arts Org")
	place := insertPlace(t, ctx, pool, "Centennial Park", "Toronto", "ON")
	start := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	ulidValue := insertEvent(t, ctx, pool, "Jazz in the Park", "Live jazz", org, place, "music", "published", []string{"jazz"}, start)

	var eventID string
	err = pool.QueryRow(ctx, `SELECT id FROM events WHERE ulid = $1`, ulidValue).Scan(&eventID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
INSERT INTO event_sources (event_id, source_id, source_url, source_event_id, retrieved_at, payload, payload_hash, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, eventID, source.ID, "https://catalog.example.org/events/1", "ext-1", time.Now().UTC(), []byte("{}"), "hash", 0.9)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
INSERT INTO field_provenance (event_id, field_path, value_hash, value_preview, source_id, confidence, observed_at, applied_to_canonical)
VALUES ($1, $2, $3, $4, $5, $6, $7, true)
`, eventID, "/name", "hash-name", "Jazz in the Park", source.ID, 0.8, time.Now().UTC())
	require.NoError(t, err)

	attrs, err := repo.GetEventSources(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, attrs, 1)
	require.Equal(t, source.ID, attrs[0].SourceID)
	require.Equal(t, "Catalog", attrs[0].SourceName)

	fields, err := repo.GetFieldProvenance(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, fields, 1)
	require.Equal(t, "/name", fields[0].FieldPath)
	require.Equal(t, "Catalog", fields[0].SourceName)
}
