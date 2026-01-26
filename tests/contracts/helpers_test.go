package contracts_test

import (
	"context"
	"io"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/config"
	storagepostgres "github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

type testEnv struct {
	Context context.Context
	DBURL   string
	Pool    *pgxpool.Pool
	Server  *httptest.Server
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	container, err := postgres.Run(
		ctx,
		"postgis/postgis:16-3.4",
		postgres.WithDatabase("sel"),
		postgres.WithUsername("sel"),
		postgres.WithPassword("sel_dev"),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	migrationsPath := filepath.Join(projectRoot(t), "internal", "storage", "postgres", "migrations")
	require.NoError(t, migrateWithRetry(dbURL, migrationsPath, 10*time.Second))

	pool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	server := httptest.NewServer(api.NewRouter(testConfig(dbURL), testLogger()))
	t.Cleanup(server.Close)

	return &testEnv{
		Context: ctx,
		DBURL:   dbURL,
		Pool:    pool,
		Server:  server,
	}
}

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func testConfig(dbURL string) config.Config {
	return config.Config{
		Server: config.ServerConfig{
			Host:    "127.0.0.1",
			Port:    0,
			BaseURL: "http://localhost",
		},
		Database: config.DatabaseConfig{
			URL:            dbURL,
			MaxConnections: 5,
			MaxIdle:        2,
		},
		Auth: config.AuthConfig{
			JWTSecret: "test-secret-32-bytes-minimum----",
			JWTExpiry: time.Hour,
		},
		RateLimit: config.RateLimitConfig{
			PublicPerMinute: 1000,
			AgentPerMinute:  1000,
			AdminPerMinute:  0,
		},
		AdminBootstrap: config.AdminBootstrapConfig{},
		Jobs: config.JobsConfig{
			RetryDeduplication:  1,
			RetryReconciliation: 1,
			RetryEnrichment:     1,
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "json",
		},
		Environment: "test",
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func migrateWithRetry(databaseURL string, migrationsPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := storagepostgres.MigrateUp(databaseURL, migrationsPath); err != nil {
			if time.Now().After(deadline) {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return nil
	}
}

type seededEntity struct {
	ID   string
	ULID string
}

func insertOrganization(t *testing.T, env *testEnv, name string) seededEntity {
	t.Helper()
	ulidValue := ulid.Make().String()
	var id string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO organizations (ulid, name, address_locality) VALUES ($1, $2, $3) RETURNING id`,
		ulidValue, name, "Toronto",
	).Scan(&id)
	require.NoError(t, err)
	return seededEntity{ID: id, ULID: ulidValue}
}

func insertPlace(t *testing.T, env *testEnv, name string, city string) seededEntity {
	t.Helper()
	ulidValue := ulid.Make().String()
	var id string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality, address_region) VALUES ($1, $2, $3, $4) RETURNING id`,
		ulidValue, name, city, "ON",
	).Scan(&id)
	require.NoError(t, err)
	return seededEntity{ID: id, ULID: ulidValue}
}

func insertEventWithOccurrence(t *testing.T, env *testEnv, name string, organizerID string, venueID string, domain string, state string, keywords []string, start time.Time) string {
	t.Helper()

	ulidValue := ulid.Make().String()
	var eventID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO events (ulid, name, organizer_id, primary_venue_id, event_domain, lifecycle_state, keywords)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		ulidValue, name, organizerID, venueID, domain, state, keywords,
	).Scan(&eventID)
	require.NoError(t, err)

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_occurrences (event_id, start_time, end_time, venue_id)
		 VALUES ($1, $2, $3, $4)`,
		eventID, start, start.Add(2*time.Hour), venueID,
	)
	require.NoError(t, err)

	return ulidValue
}

func insertAPIKey(t *testing.T, env *testEnv, name string) string {
	t.Helper()

	key := ulid.Make().String() + "secret"
	prefix := key[:8]
	hash, err := auth.HashAPIKey(key)
	require.NoError(t, err, "failed to hash API key")

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO api_keys (prefix, key_hash, hash_version, name) VALUES ($1, $2, $3, $4)`,
		prefix, hash, auth.HashVersionBcrypt, name,
	)
	require.NoError(t, err)

	return key
}

func eventNameFromPayload(payload map[string]any) string {
	if value, ok := payload["name"].(string); ok {
		return value
	}
	if value, ok := payload["name"].(map[string]any); ok {
		if text, ok := value["value"].(string); ok {
			return text
		}
	}
	return ""
}
