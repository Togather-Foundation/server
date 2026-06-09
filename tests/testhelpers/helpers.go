// Package testhelpers provides shared test utilities for integration, contract, and
// batch test packages. Functions here are pure utilities that take explicit DB/context
// arguments rather than a package-specific testEnv struct.
package testhelpers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestLogger returns a no-op logger suitable for tests.
func TestLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

// TestConfig returns a standard config.Config for tests using the given DB URL.
// Rate limits are set high so they never interfere with test logic.
// AllowTestDomains is always true so test code constructing example.com URLs passes validation.
func TestConfig(dbURL string) config.Config {
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
			JWTSecret:              "test-secret-32-bytes-minimum----",
			JWTExpiry:              time.Hour,
			TokenExchangeJWTExpiry: 5 * time.Minute,
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
		Validation: config.ValidationConfig{
			AllowTestDomains: true,
		},
		DefaultTimezone: "America/Toronto",
		Environment:     "test",
	}
}

// ProjectRoot walks up from the caller's file location to return the repository root.
func ProjectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(1)
	require.True(t, ok)
	// helpers.go lives at tests/testhelpers/helpers.go — two levels up is repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// MigrateWithRetry runs MigrateUp, retrying until timeout if the DB isn't ready yet.
func MigrateWithRetry(databaseURL string, migrationsPath string, timeout time.Duration) error {
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

// ResetDatabase truncates all public tables (except schema_migrations and river_migration)
// and restarts sequences. Call at the start of each test to ensure isolation.
func ResetDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if pool == nil {
		require.Fail(t, "shared pool is nil")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, `
SELECT tablename
  FROM pg_tables
 WHERE schemaname = 'public'
   AND tablename <> 'schema_migrations'
   AND tablename <> 'river_migration'
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
		safe := strings.ReplaceAll(name, `"`, `""`)
		tables = append(tables, `"public"."`+safe+`"`)
	}
	require.NoError(t, rows.Err())

	if len(tables) == 0 {
		return
	}

	truncateSQL := "TRUNCATE TABLE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE;"
	_, err = pool.Exec(ctx, truncateSQL)
	require.NoError(t, err)
}

// InsertAPIKey creates a test API key with a random prefix and SHA-256 hash, inserts
// it into the database, and returns the raw key string. Uses crypto/rand (not ULID) to
// avoid prefix collisions when many keys are created in rapid succession. Uses SHA-256
// (not bcrypt) to avoid ~300ms/hash overhead; ValidateAPIKey supports both hash versions.
func InsertAPIKey(t *testing.T, pool *pgxpool.Pool, ctx context.Context, name string) string {
	t.Helper()

	rawBytes := make([]byte, 16)
	_, err := rand.Read(rawBytes)
	require.NoError(t, err, "failed to generate random key bytes")
	key := hex.EncodeToString(rawBytes) // 32 hex chars, fully random
	prefix := key[:8]
	hash := auth.HashAPIKeySHA256(key)

	_, err = pool.Exec(ctx,
		`INSERT INTO api_keys (prefix, key_hash, hash_version, name) VALUES ($1, $2, $3, $4)`,
		prefix, hash, auth.HashVersionSHA256, name,
	)
	require.NoError(t, err, "failed to insert API key")

	return key
}

// InsertAPIKeyWithRole inserts an API key with a specific role. Unlike InsertAPIKey
// (which defaults to 'agent'), this allows creating admin keys for token exchange tests.
// Uses crypto/rand + SHA-256 hashing (same as InsertAPIKey).
func InsertAPIKeyWithRole(t *testing.T, pool *pgxpool.Pool, ctx context.Context, name, role string) string {
	t.Helper()

	rawBytes := make([]byte, 16)
	_, err := rand.Read(rawBytes)
	require.NoError(t, err, "failed to generate random key bytes")
	key := hex.EncodeToString(rawBytes)
	prefix := key[:8]
	hash := auth.HashAPIKeySHA256(key)

	_, err = pool.Exec(ctx,
		`INSERT INTO api_keys (prefix, key_hash, hash_version, name, role) VALUES ($1, $2, $3, $4, $5)`,
		prefix, hash, auth.HashVersionSHA256, name, role,
	)
	require.NoError(t, err, "failed to insert API key with role")

	return key
}

// InsertAdminUser inserts a user with a bcrypt-hashed password.
// Note: bcrypt is unavoidable here because the production login handler verifies
// passwords with bcrypt. Use sparingly — it adds ~100ms per call.
func InsertAdminUser(t *testing.T, pool *pgxpool.Pool, ctx context.Context, username, password, email, role string) {
	t.Helper()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err, "failed to hash password")

	_, err = pool.Exec(ctx,
		`INSERT INTO users (username, email, password_hash, role, is_active) VALUES ($1, $2, $3, $4, $5)`,
		username, email, string(hashedPassword), role, true,
	)
	require.NoError(t, err, "failed to insert admin user")
}
