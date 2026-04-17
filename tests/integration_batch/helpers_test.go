package integration_batch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/Togather-Foundation/server/tests/testhelpers"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
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
	sharedOnce        sync.Once
	sharedInitErr     error
	sharedContainer   *tcpostgres.PostgresContainer
	sharedPool        *pgxpool.Pool
	sharedDBURL       string
	sharedConfig      config.Config
	sharedRiverClient *river.Client[pgx.Tx]
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
	testhelpers.ResetDatabase(t, sharedPool)

	routerWithClient := api.NewRouter(sharedConfig, testhelpers.TestLogger(), sharedPool, "test", "test-commit", "test-date")

	// Start River workers for batch ingestion tests
	// This is the key difference from tests/integration - we NEED River Workers here
	if routerWithClient.RiverClient != nil {
		sharedRiverClient = routerWithClient.RiverClient
		if err := routerWithClient.RiverClient.Start(ctx); err != nil {
			t.Fatalf("failed to start river workers: %v", err)
		}
		t.Cleanup(func() {
			sharedRiverClient = nil
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

		migrationsPath := filepath.Join(testhelpers.ProjectRoot(t), "internal", "storage", "postgres", "migrations")
		if err := testhelpers.MigrateWithRetry(dbURL, migrationsPath, 10*time.Second); err != nil {
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
	if sharedContainer != nil {
		_ = sharedContainer.Terminate(context.Background())
	}
}

func resetDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	testhelpers.ResetDatabase(t, pool)
}

func testConfig(dbURL string) config.Config {
	return testhelpers.TestConfig(dbURL)
}

func insertAPIKey(t *testing.T, env *testEnv, name string) string {
	t.Helper()
	return testhelpers.InsertAPIKey(t, env.Pool, env.Context, name)
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

// awaitBatchCompletion waits for a batch ingestion River job to complete by subscribing
// to River client events. This replaces HTTP polling loops in batch tests.
func awaitBatchCompletion(t *testing.T, batchID string, timeout time.Duration) {
	t.Helper()

	require.NotNil(t, sharedRiverClient, "sharedRiverClient must be set")

	subscribeChan, cancel := sharedRiverClient.Subscribe(
		river.EventKindJobCompleted,
		river.EventKindJobFailed,
	)
	defer cancel()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case event, ok := <-subscribeChan:
			if !ok {
				t.Fatal("subscription channel closed before batch job completed")
				return
			}
			if event == nil {
				continue
			}
			// Check if this is a batch_ingestion job for our batch
			if event.Job.Kind != jobs.JobKindBatchIngestion {
				continue
			}
			var args jobs.BatchIngestionArgs
			if err := json.Unmarshal(event.Job.EncodedArgs, &args); err != nil {
				t.Logf("failed to unmarshal job args: %v", err)
				continue
			}
			if args.BatchID == batchID {
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for batch job %s to complete", batchID)
			return
		}
	}
}
