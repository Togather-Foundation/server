package integration

import (
	"context"
	"io"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
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

	container, err := tcpostgres.Run(
		ctx,
		"postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("sel"),
		tcpostgres.WithUsername("sel"),
		tcpostgres.WithPassword("sel_dev"),
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
		if err := postgres.MigrateUp(databaseURL, migrationsPath); err != nil {
			if time.Now().After(deadline) {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return nil
	}
}
