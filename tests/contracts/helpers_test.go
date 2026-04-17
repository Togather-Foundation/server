package contracts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/tests/testhelpers"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"golang.org/x/crypto/bcrypt"
)

var (
	sharedOnce      sync.Once
	sharedInitErr   error
	sharedContainer *tcpostgres.PostgresContainer
	sharedPool      *pgxpool.Pool
	sharedDBURL     string
	sharedConfig    config.Config
)

const sharedContainerName = "togather-contracts-db"

type testEnv struct {
	Context context.Context
	DBURL   string
	Pool    *pgxpool.Pool
	Server  *httptest.Server
}

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

	server := httptest.NewServer(api.NewRouter(sharedConfig, testhelpers.TestLogger(), sharedPool, "test", "test-commit", "test-date").Handler)
	t.Cleanup(server.Close)

	return &testEnv{
		Context: ctx,
		DBURL:   sharedDBURL,
		Pool:    sharedPool,
		Server:  server,
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

func testConfig(dbURL string) config.Config {
	return testhelpers.TestConfig(dbURL)
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
	return testhelpers.InsertAPIKey(t, env.Pool, env.Context, name)
}

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
