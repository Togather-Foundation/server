package postgres

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

var (
	sharedOnce      sync.Once
	sharedInitErr   error
	sharedContainer *postgres.PostgresContainer
	sharedPool      *pgxpool.Pool
	sharedDBURL     string
)

const sharedContainerName = "togather-storage-db"

type seededEntity struct {
	ID   string
	ULID string
}

func TestMain(m *testing.M) {
	code := m.Run()
	cleanupShared()
	os.Exit(code)
}

func setupPostgres(t *testing.T, ctx context.Context) (*pgxpool.Pool, string) {
	t.Helper()

	initShared(t)
	resetDatabase(t, sharedPool)

	return sharedPool, sharedDBURL
}

func initShared(t *testing.T) {
	t.Helper()
	sharedOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Disable ryuk (resource reaper) to prevent premature container cleanup
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

		container, err := postgres.Run(
			ctx,
			"postgis/postgis:16-3.4",
			postgres.WithDatabase("sel"),
			postgres.WithUsername("sel"),
			postgres.WithPassword("sel_dev"),
			testcontainers.WithReuseByName(sharedContainerName),
		)
		if err != nil {
			sharedInitErr = err
			return
		}
		sharedContainer = container

		dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			sharedInitErr = err
			return
		}
		sharedDBURL = dbURL

		migrationsPath := filepath.Join(projectRoot(), DefaultMigrationsPath)
		if err := migrateWithRetry(dbURL, migrationsPath, 10*time.Second); err != nil {
			sharedInitErr = err
			return
		}

		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			sharedInitErr = err
			return
		}

		sharedPool = pool
	})

	require.NoError(t, sharedInitErr)
}

func cleanupShared() {
	if sharedPool != nil {
		sharedPool.Close()
	}
	// Note: Do NOT terminate the shared container - testcontainers will clean it up
	// Terminating it here causes connection errors in tests that haven't run yet
}

func resetDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if pool == nil {
		require.Fail(t, "shared pool is nil")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// WORKAROUND: PostGIS extension doesn't always populate spatial_ref_sys automatically
	// Manually insert SRID 4326 if not present to support geography/geometry operations
	// Do this on every reset to ensure it's present even if container was just created
	_, err := pool.Exec(ctx, `
		INSERT INTO spatial_ref_sys (srid, auth_name, auth_srid, proj4text, srtext)
		VALUES (4326, 'EPSG', 4326, '+proj=longlat +datum=WGS84 +no_defs',
		'GEOGCS["WGS 84",DATUM["WGS_1984",SPHEROID["WGS 84",6378137,298.257223563,AUTHORITY["EPSG","7030"]],AUTHORITY["EPSG","6326"]],PRIMEM["Greenwich",0,AUTHORITY["EPSG","8901"]],UNIT["degree",0.0174532925199433,AUTHORITY["EPSG","9122"]],AUTHORITY["EPSG","4326"]]')
		ON CONFLICT (srid) DO NOTHING
	`)
	require.NoError(t, err, "Failed to populate SRID 4326 in spatial_ref_sys")

	rows, err := pool.Query(ctx, `
SELECT tablename
  FROM pg_tables
 WHERE schemaname = 'public'
   AND tablename <> 'schema_migrations'
   AND tablename <> 'spatial_ref_sys'
 ORDER BY tablename;
`)
	require.NoError(t, err)
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		if name == "" {
			continue
		}
		safe := strings.ReplaceAll(name, "\"", "\"\"")
		tables = append(tables, "\"public\".\""+safe+"\"")
	}
	require.NoError(t, rows.Err())

	if len(tables) == 0 {
		return
	}

	truncateSQL := "TRUNCATE TABLE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE;"
	_, err = pool.Exec(ctx, truncateSQL)
	require.NoError(t, err)
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
