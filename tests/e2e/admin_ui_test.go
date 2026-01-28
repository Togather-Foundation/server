package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"golang.org/x/crypto/bcrypt"
)

// TestAdminLoginPageRendersHTML tests that /admin/login returns HTML
func TestAdminLoginPageRendersHTML(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/login", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should return HTML (or 404 if not implemented yet, but not 500)
	assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, "should not error")

	if resp.StatusCode == http.StatusOK {
		contentType := resp.Header.Get("Content-Type")
		assert.True(t, strings.HasPrefix(contentType, "text/html"), "should return HTML content type")
	}
}

// TestAdminDashboardRedirectsWhenUnauthenticated tests that /admin/dashboard redirects to login
func TestAdminDashboardRedirectsWhenUnauthenticated(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/dashboard", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should redirect to login or return 401
	assert.True(t,
		resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther ||
			resp.StatusCode == http.StatusTemporaryRedirect,
		"unauthenticated request should redirect or return 401, got %d", resp.StatusCode)
}

// TestAdminEventsPageAccessible tests that /admin/events is accessible with auth
func TestAdminEventsPageAccessible(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// This test requires admin authentication
	// For now, just verify the route exists and requires auth
	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/events", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should require authentication (401 or redirect)
	assert.True(t,
		resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther,
		"should require authentication")
}

// TestAdminAPIKeysPageAccessible tests that /admin/api-keys is accessible with auth
func TestAdminAPIKeysPageAccessible(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/api-keys", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/html")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should require authentication
	assert.True(t,
		resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther,
		"should require authentication")
}

// TestAdminStaticAssetsAccessible tests that admin static assets are served
func TestAdminStaticAssetsAccessible(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	staticPaths := []string{
		"/admin/static/css/admin.css",
		"/admin/static/js/admin.js",
	}

	for _, path := range staticPaths {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
			require.NoError(t, err)

			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Should either serve the asset (200) or not found (404), but not error (500)
			assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, "should not error serving static assets")
		})
	}
}

// TestAdminLoginPostAcceptsCredentials tests that POST /api/v1/admin/login accepts credentials
func TestAdminLoginPostAcceptsCredentials(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	loginPayload := map[string]string{
		"username": "admin",
		"password": "test123",
	}
	body, err := json.Marshal(loginPayload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/admin/login", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should not error (might be 401 for invalid credentials, but not 500)
	assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, "login endpoint should not error")
}

// TestAdminRoutesRejectPublicAccess tests that admin routes reject unauthenticated access
func TestAdminRoutesRejectPublicAccess(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	adminRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/admin/dashboard"},
		{http.MethodGet, "/admin/events"},
		{http.MethodGet, "/admin/events/pending"},
		{http.MethodGet, "/admin/duplicates"},
		{http.MethodGet, "/admin/api-keys"},
	}

	for _, route := range adminRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req, err := http.NewRequest(route.method, server.URL+route.path, nil)
			require.NoError(t, err)
			req.Header.Set("Accept", "text/html")

			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Should require authentication (not 200)
			assert.NotEqual(t, http.StatusOK, resp.StatusCode, "admin route should require authentication")
		})
	}
}

var (
	sharedOnce      sync.Once
	sharedInitErr   error
	sharedContainer *tcpostgres.PostgresContainer
	sharedPool      *pgxpool.Pool
	sharedDBURL     string
	sharedConfig    config.Config
)

const sharedContainerName = "togather-e2e-db"

func TestMain(m *testing.M) {
	code := m.Run()
	cleanupShared()
	os.Exit(code)
}

// setupTestServer creates a test HTTP server for E2E tests with full database
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	initShared(t)
	resetDatabase(t, sharedPool)

	// Insert an admin user for authentication tests
	insertAdminUser(t, ctx, sharedPool, "admin", "test123", "admin@example.com", "admin")

	server := httptest.NewServer(api.NewRouter(sharedConfig, testLogger(), "test", "test-commit", "test-date"))
	t.Cleanup(server.Close)

	return server
}

func initShared(t *testing.T) {
	t.Helper()
	sharedOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

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
		sharedPool = pool
		sharedConfig = testConfig(dbURL)
	})

	require.NoError(t, sharedInitErr)
}

func cleanupShared() {
	if sharedPool != nil {
		sharedPool.Close()
	}
	if sharedContainer != nil {
		_ = sharedContainer.Terminate(context.Background())
	}
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

// testLogger returns a no-op logger for tests
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

// insertAdminUser inserts a user into the database with hashed password
func insertAdminUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, username, password, email, role string) {
	t.Helper()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err, "failed to hash password")

	_, err = pool.Exec(ctx,
		`INSERT INTO users (username, email, password_hash, role, is_active) VALUES ($1, $2, $3, $4, $5)`,
		username, email, string(hashedPassword), role, true,
	)
	require.NoError(t, err, "failed to insert admin user")
}
