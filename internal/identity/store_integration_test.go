package identity

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var (
	identityOnce      sync.Once
	identityInitErr   error
	identityContainer *testpostgres.PostgresContainer
	identityPool      *pgxpool.Pool
)

const identityContainerName = "togather-identity-db"

func TestMain(m *testing.M) {
	code := m.Run()
	cleanupIdentity()
	os.Exit(code)
}

func initIdentity(t *testing.T) {
	t.Helper()
	identityOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

		container, err := testpostgres.Run(
			ctx,
			"postgis/postgis:16-3.4",
			testpostgres.WithDatabase("sel"),
			testpostgres.WithUsername("sel"),
			testpostgres.WithPassword("sel_dev"),
			testcontainers.WithReuseByName(identityContainerName),
		)
		if err != nil {
			identityInitErr = err
			return
		}
		identityContainer = container

		dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			identityInitErr = err
			return
		}

		migrationsPath := filepath.Join(identityProjectRoot(), postgres.DefaultMigrationsPath)
		if err := identityMigrateWithRetry(dbURL, migrationsPath, 10*time.Second); err != nil {
			identityInitErr = err
			return
		}

		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			identityInitErr = err
			return
		}
		identityPool = pool
	})

	require.NoError(t, identityInitErr)
}

func cleanupIdentity() {
	if identityPool != nil {
		identityPool.Close()
	}
	if identityContainer != nil {
		_ = identityContainer.Terminate(context.Background())
	}
}

func setupIdentity(t *testing.T) *pgxpool.Pool {
	t.Helper()
	initIdentity(t)
	resetIdentityTables(t, identityPool)
	return identityPool
}

// resetIdentityTables truncates only the identity tables, preserving the seeded
// knowledge_graph_authorities rows that entity_identifiers references.
func resetIdentityTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `TRUNCATE TABLE entity_identifiers, identity_decisions, identity_not_duplicates RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
}

func identityProjectRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func identityMigrateWithRetry(databaseURL, migrationsPath string, timeout time.Duration) error {
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

// primaryState is a snapshot of an entity_identifiers row's primary-slot columns.
type primaryState struct {
	ID             int32
	URI            string
	Method         string
	Confidence     float64
	IsPrimary      bool
	SupersededByID *int32
	ObservedAt     time.Time
}

func loadPrimaryStates(t *testing.T, pool *pgxpool.Pool, entityType, entityID string) []primaryState {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
		SELECT id, identifier_uri, reconciliation_method, confidence, is_primary, superseded_by_id, observed_at
		FROM entity_identifiers
		WHERE entity_type = $1 AND entity_id = $2
		ORDER BY id`, entityType, entityID)
	require.NoError(t, err)
	defer rows.Close()

	var states []primaryState
	for rows.Next() {
		var s primaryState
		var confidence pgtype.Numeric
		var superseded pgtype.Int4
		require.NoError(t, rows.Scan(&s.ID, &s.URI, &s.Method, &confidence, &s.IsPrimary, &superseded, &s.ObservedAt))
		f, err := confidence.Float64Value()
		require.NoError(t, err)
		s.Confidence = f.Float64
		if superseded.Valid {
			id := superseded.Int32
			s.SupersededByID = &id
		}
		states = append(states, s)
	}
	require.NoError(t, rows.Err())
	return states
}

// TestRecordObservation_Election verifies that two writes with different top
// matches leave exactly one primary, the higher-ranked row wins, superseded_by_id
// points at the winner, observed_at is refreshed, and both rows are retained.
func TestRecordObservation_Election(t *testing.T) {
	pool := setupIdentity(t)
	store := NewStore(pool)
	ctx := context.Background()

	ref := IdentityRef{Type: EntityTypePlace, ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"}

	_, err := store.RecordObservation(ctx, ref, IdentifierObservation{
		Authority:  "artsdata",
		URI:        "https://kg.artsdata.ca/resource/K11-24",
		Method:     "auto_low",
		Confidence: 0.90,
		Source:     "reconciliation",
	})
	require.NoError(t, err)

	winner, err := store.RecordObservation(ctx, ref, IdentifierObservation{
		Authority:  "artsdata",
		URI:        "https://kg.artsdata.ca/resource/K11-99",
		Method:     "auto_high",
		Confidence: 0.99,
		Source:     "reconciliation",
	})
	require.NoError(t, err)
	require.True(t, winner.IsPrimary)
	require.Equal(t, "https://kg.artsdata.ca/resource/K11-99", winner.URI)

	states := loadPrimaryStates(t, pool, string(ref.Type), ref.ULID)
	require.Len(t, states, 2, "both rows must be retained")

	var primaries int
	var demoted *primaryState
	for i := range states {
		s := &states[i]
		if s.IsPrimary {
			primaries++
			require.Equal(t, "https://kg.artsdata.ca/resource/K11-99", s.URI, "higher-ranked row must win")
			require.Nil(t, s.SupersededByID, "winner must not be superseded")
		} else {
			demoted = s
		}
	}
	require.Equal(t, 1, primaries, "exactly one primary must exist")
	require.NotNil(t, demoted)
	require.Equal(t, "https://kg.artsdata.ca/resource/K11-24", demoted.URI)
	require.NotNil(t, demoted.SupersededByID, "demoted row must point at the winner")
	require.Equal(t, winner.ID, *demoted.SupersededByID)
	require.True(t, winner.ObservedAt.After(demoted.ObservedAt) || winner.ObservedAt.Equal(demoted.ObservedAt),
		"winner observed_at must be refreshed")
}

// TestRecordObservation_Idempotent verifies that re-observing the current winner
// leaves it primary.
func TestRecordObservation_Idempotent(t *testing.T) {
	pool := setupIdentity(t)
	store := NewStore(pool)
	ctx := context.Background()

	ref := IdentityRef{Type: EntityTypePlace, ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"}
	obs := IdentifierObservation{
		Authority:  "artsdata",
		URI:        "https://kg.artsdata.ca/resource/K11-99",
		Method:     "auto_high",
		Confidence: 0.99,
		Source:     "reconciliation",
	}

	first, err := store.RecordObservation(ctx, ref, obs)
	require.NoError(t, err)
	require.True(t, first.IsPrimary)

	// Re-observe the current winner; must remain primary with the same id.
	again, err := store.RecordObservation(ctx, ref, obs)
	require.NoError(t, err)
	require.True(t, again.IsPrimary)
	require.Equal(t, first.ID, again.ID, "re-observing the winner must not change its row id")

	states := loadPrimaryStates(t, pool, string(ref.Type), ref.ULID)
	require.Len(t, states, 1, "idempotent re-observation must not create a new row")
	require.True(t, states[0].IsPrimary)
	require.Nil(t, states[0].SupersededByID)
}

// TestRecordObservation_Concurrent verifies that concurrent observations on one
// group yield exactly one primary.
func TestRecordObservation_Concurrent(t *testing.T) {
	pool := setupIdentity(t)
	store := NewStore(pool)
	ctx := context.Background()

	ref := IdentityRef{Type: EntityTypeOrganization, ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"}

	uris := []string{
		"https://kg.artsdata.ca/resource/K11-1",
		"https://kg.artsdata.ca/resource/K11-2",
		"https://kg.artsdata.ca/resource/K11-3",
		"https://kg.artsdata.ca/resource/K11-4",
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(uris))
	for _, uri := range uris {
		wg.Add(1)
		go func(uri string) {
			defer wg.Done()
			_, err := store.RecordObservation(ctx, ref, IdentifierObservation{
				Authority:  "artsdata",
				URI:        uri,
				Method:     "auto_high",
				Confidence: 0.95,
				Source:     "reconciliation",
			})
			errs <- err
		}(uri)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	states := loadPrimaryStates(t, pool, string(ref.Type), ref.ULID)
	require.Len(t, states, len(uris), "all concurrent rows must be retained")

	var primaries int
	for i := range states {
		if states[i].IsPrimary {
			primaries++
			require.Nil(t, states[i].SupersededByID)
		}
	}
	require.Equal(t, 1, primaries, "concurrent writes must leave exactly one primary")
}
