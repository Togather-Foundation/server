package integration_batch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"golang.org/x/crypto/bcrypt"
)

type testEnv struct {
	Context context.Context
	DBURL   string
	Pool    *pgxpool.Pool
	Server  *httptest.Server
	Config  config.Config
}

var (
	sharedOnce      sync.Once
	sharedInitErr   error
	sharedContainer *tcpostgres.PostgresContainer
	sharedPool      *pgxpool.Pool
	sharedDBURL     string
	sharedConfig    config.Config
)

const sharedContainerName = "togather-integration-batch-db"

func TestMain(m *testing.M) {
	code := m.Run()
	cleanupShared()
	os.Exit(code)
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	initShared(t)
	resetDatabase(t, sharedPool)

	routerWithClient := api.NewRouter(sharedConfig, testLogger(), sharedPool, "test", "test-commit", "test-date")

	// Start River workers for batch ingestion tests
	// This is the key difference from tests/integration - we NEED River workers here
	if routerWithClient.RiverClient != nil {
		if err := routerWithClient.RiverClient.Start(ctx); err != nil {
			t.Fatalf("failed to start river workers: %v", err)
		}
		t.Cleanup(func() {
			if err := routerWithClient.RiverClient.Stop(context.Background()); err != nil {
				t.Logf("failed to stop river workers: %v", err)
			}
		})
	}

	server := httptest.NewServer(routerWithClient.Handler)
	t.Cleanup(server.Close)

	return &testEnv{
		Context: ctx,
		DBURL:   sharedDBURL,
		Pool:    sharedPool,
		Server:  server,
		Config:  sharedConfig,
	}
}

func initShared(t *testing.T) {
	t.Helper()
	sharedOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Disable ryuk (resource reaper) to prevent premature container cleanup
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

		container, err := tcpostgres.Run(
			ctx,
			"postgis/postgis:16-3.4",
			tcpostgres.WithDatabase("sel"),
			tcpostgres.WithUsername("sel"),
			tcpostgres.WithPassword("sel_dev"),
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

		migrationsPath := filepath.Join(projectRoot(t), "internal", "storage", "postgres", "migrations")
		if err := migrateWithRetry(dbURL, migrationsPath, 10*time.Second); err != nil {
			sharedInitErr = err
			return
		}

		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			sharedInitErr = err
			return
		}

		// Run River migrations programmatically
		migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
		if err != nil {
			sharedInitErr = err
			pool.Close()
			return
		}
		_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{})
		if err != nil {
			sharedInitErr = err
			pool.Close()
			return
		}

		sharedPool = pool
		sharedConfig = testConfig(dbURL)
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

// nolint:unused
func createdEventLocation(payload map[string]any) (map[string]any, error) {
	if payload == nil {
		return nil, errors.New("missing payload")
	}
	location, ok := payload["location"].(map[string]any)
	if !ok {
		return nil, errors.New("missing location")
	}
	return location, nil
}

// nolint:unused
func eventIDFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if value, ok := payload["@id"].(string); ok {
		candidate := strings.TrimSpace(value)
		if candidate == "" {
			return ""
		}
		if ids.IsULID(candidate) {
			return strings.ToUpper(candidate)
		}
		parsed, err := url.Parse(candidate)
		if err == nil && parsed.Host != "" {
			nodeDomain := parsed.Host
			if parsed.Scheme != "" {
				nodeDomain = parsed.Scheme + "://" + parsed.Host
			}
			entity, err := ids.ParseEntityURI(nodeDomain, "events", candidate, "")
			if err == nil {
				return entity.ULID
			}
			pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(pathParts) > 1 {
				ulidCandidate := pathParts[len(pathParts)-1]
				if ids.IsULID(ulidCandidate) {
					return strings.ToUpper(ulidCandidate)
				}
			}
		}
	}
	return ""
}

// insertAdminUser inserts a user into the database with hashed password
// nolint:unused
func insertAdminUser(t *testing.T, env *testEnv, username, password, email, role string) {
	t.Helper()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err, "failed to hash password")

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO users (username, email, password_hash, role, is_active) VALUES ($1, $2, $3, $4, $5)`,
		username, email, string(hashedPassword), role, true,
	)
	require.NoError(t, err, "failed to insert admin user")
}

// adminLogin performs login and returns the JWT token
// nolint:unused
func adminLogin(t *testing.T, env *testEnv, username, password string) string {
	t.Helper()

	loginPayload := map[string]string{
		"username": username,
		"password": password,
	}
	body, err := json.Marshal(loginPayload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/admin/login", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode, "login failed")

	var loginResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&loginResp))
	token, ok := loginResp["token"].(string)
	require.True(t, ok, "expected token in response")
	require.NotEmpty(t, token, "expected non-empty token")

	return token
}

// Change Feed Test Helpers

// changeFeedResponse represents the response from /api/v1/feeds/changes
// nolint:unused
type changeFeedResponse struct {
	Cursor     string           `json:"cursor"`      // Current cursor position
	Changes    []changeFeedItem `json:"changes"`     // List of changes (renamed from Items per Interop Profile)
	NextCursor string           `json:"next_cursor"` // Next cursor for pagination
}

// changeFeedItem represents a single change entry in the feed
// nolint:unused
type changeFeedItem struct {
	SequenceNumber int64           `json:"sequence_number"`
	EventID        string          `json:"event_id"`
	Action         string          `json:"action"`
	ChangedAt      string          `json:"changed_at"`
	ChangedFields  json.RawMessage `json:"changed_fields,omitempty"`
	Snapshot       json.RawMessage `json:"snapshot,omitempty"`
}

// fetchChangeFeed makes a GET request to /api/v1/feeds/changes
// nolint:unused
func fetchChangeFeed(t *testing.T, env *testEnv, params url.Values) changeFeedResponse {
	t.Helper()

	u := env.Server.URL + "/api/v1/feeds/changes"
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	require.NoError(t, err, "failed to create HTTP GET request for change feed")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err, "failed to execute change feed HTTP request")
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode, "change feed request should succeed")

	var payload changeFeedResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload), "failed to decode change feed response JSON")
	return payload
}
