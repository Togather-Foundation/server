package postgres

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

type seededEntity struct {
	ID   string
	ULID string
}

func setupPostgres(t *testing.T, ctx context.Context) (*postgres.PostgresContainer, string) {
	container, err := postgres.Run(
		ctx,
		"postgis/postgis:16-3.4",
		postgres.WithDatabase("sel"),
		postgres.WithUsername("sel"),
		postgres.WithPassword("sel_dev"),
	)
	require.NoError(t, err)

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	migrationsPath := filepath.Join(projectRoot(), DefaultMigrationsPath)
	require.NoError(t, migrateWithRetry(dbURL, migrationsPath, 10*time.Second))
	return container, dbURL
}

func insertOrganization(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) seededEntity {
	ulidValue := ulid.Make().String()
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO organizations (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		ulidValue, name, "Toronto",
	).Scan(&id)
	require.NoError(t, err)
	return seededEntity{ID: id, ULID: ulidValue}
}

func insertPlace(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, city string, region string) seededEntity {
	ulidValue := ulid.Make().String()
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO places (ulid, name, address_locality, address_region) VALUES ($1, $2, $3, $4) RETURNING id`,
		ulidValue, name, city, region,
	).Scan(&id)
	require.NoError(t, err)
	return seededEntity{ID: id, ULID: ulidValue}
}

func insertEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, description string, org seededEntity, place seededEntity, domain string, state string, keywords []string, start time.Time) string {
	ulidValue := ulid.Make().String()
	var eventID string
	err := pool.QueryRow(ctx,
		`INSERT INTO events (ulid, name, description, organizer_id, primary_venue_id, event_domain, lifecycle_state, keywords)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
         RETURNING id`,
		ulidValue, name, description, org.ID, place.ID, domain, state, keywords,
	).Scan(&eventID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO event_occurrences (event_id, start_time, end_time, venue_id)
         VALUES ($1, $2, $3, $4)`,
		eventID, start, start.Add(2*time.Hour), place.ID,
	)
	require.NoError(t, err)

	return ulidValue
}

func setOrganizationCreatedAt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string, createdAt time.Time) {
	_, err := pool.Exec(ctx, `UPDATE organizations SET created_at = $2 WHERE id = $1`, id, createdAt)
	require.NoError(t, err)
}

func setOrganizationLegalName(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string, legalName string) {
	_, err := pool.Exec(ctx, `UPDATE organizations SET legal_name = $2 WHERE id = $1`, id, legalName)
	require.NoError(t, err)
}

func setPlaceCreatedAt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string, createdAt time.Time) {
	_, err := pool.Exec(ctx, `UPDATE places SET created_at = $2 WHERE id = $1`, id, createdAt)
	require.NoError(t, err)
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func projectRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func migrateWithRetry(databaseURL string, migrationsPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := MigrateUp(databaseURL, migrationsPath); err != nil {
			if time.Now().After(deadline) {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return nil
	}
}
