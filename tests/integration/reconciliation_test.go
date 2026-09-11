package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/identity"
	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/Togather-Foundation/server/internal/kg"
	"github.com/Togather-Foundation/server/internal/kg/artsdata"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedAuthorityCodesIfNeeded ensures authority codes exist in the database.
// The resetDatabase function truncates all tables, so we need to reseed.
func seedAuthorityCodesIfNeeded(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	// Check if authority codes exist
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM knowledge_graph_authorities`).Scan(&count)
	require.NoError(t, err)

	if count > 0 {
		return // Already seeded
	}

	// Reseed authority codes (same as migration 000030)
	_, err = pool.Exec(ctx, `
		INSERT INTO knowledge_graph_authorities (authority_code, authority_name, base_uri_pattern, reconciliation_endpoint, applicable_domains, trust_level, priority_order, rate_limit_per_minute, rate_limit_per_day, documentation_url) VALUES
		('artsdata', 'Artsdata Knowledge Graph', '^http://kg\.artsdata\.ca/resource/K\d+-\d+$', 'https://api.artsdata.ca/recon', ARRAY['arts', 'culture', 'music'], 9, 10, 60, 10000, 'https://docs.artsdata.ca/'),
		('wikidata', 'Wikidata', '^http://www\.wikidata\.org/entity/Q\d+$', NULL, ARRAY['arts', 'culture', 'music', 'sports', 'community', 'education', 'general'], 8, 20, 30, 5000, 'https://www.wikidata.org/'),
		('musicbrainz', 'MusicBrainz', '^https://musicbrainz\.org/(artist|event|place)/[0-9a-f-]+$', NULL, ARRAY['music'], 9, 15, 30, 5000, 'https://musicbrainz.org/doc/MusicBrainz_API'),
		('isni', 'ISNI', '^https?://isni\.org/isni/\d{16}$', NULL, ARRAY['arts', 'culture', 'music', 'education'], 9, 30, 10, 1000, 'https://isni.org/'),
		('osm', 'OpenStreetMap', '^https://www\.openstreetmap\.org/(node|way|relation)/\d+$', NULL, ARRAY['arts', 'culture', 'music', 'sports', 'community', 'education', 'general'], 7, 40, 60, 10000, 'https://wiki.openstreetmap.org/')
	`)
	require.NoError(t, err, "failed to seed authority codes")
}

// newMockArtsdataServer creates an httptest server that simulates the Artsdata API.
func newMockArtsdataServer(t *testing.T, callCounter *atomic.Int32) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/recon" && r.Method == "POST":
			// Increment call counter for cache testing
			if callCounter != nil {
				callCounter.Add(1)
			}

			// Parse form-encoded request per W3C Reconciliation API v0.2 §4.3
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}

			queriesParam := r.FormValue("queries")
			if queriesParam == "" {
				http.Error(w, "missing queries parameter", http.StatusBadRequest)
				return
			}

			// The queries parameter is a JSON batch object (flat, not wrapped)
			var queries map[string]interface{}
			if err := json.Unmarshal([]byte(queriesParam), &queries); err != nil {
				http.Error(w, "invalid queries JSON", http.StatusBadRequest)
				return
			}

			// Build response based on query
			// Use mock server URL for the ID so dereference calls come back to the mock
			baseURL := "http://" + r.Host
			response := make(map[string]interface{})
			for queryID, queryData := range queries {
				queryMap, ok := queryData.(map[string]interface{})
				if !ok {
					continue
				}

				queryName, _ := queryMap["query"].(string)

				// Return no results for "No Match Place"
				if queryName == "No Match Place" {
					response[queryID] = map[string]interface{}{
						"result": []interface{}{},
					}
					continue
				}

				// Return results for other queries, using mock server URL
				response[queryID] = map[string]interface{}{
					"result": []interface{}{
						map[string]interface{}{
							"id":    baseURL + "/resource/K11-211",
							"name":  "Art Gallery of Ontario",
							"score": 98.5,
							"match": true,
							"type": []map[string]string{
								{"id": "schema:Place", "name": "Place"},
							},
						},
					},
				}
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				http.Error(w, "failed to encode response", http.StatusInternalServerError)
				return
			}

		case strings.HasPrefix(r.URL.Path, "/resource/") && r.Method == "GET":
			// Return a dereferenced entity with sameAs links, using the realistic
			// *compacted* JSON-LD shape (id/type aliases, @none-wrapped address
			// scalars) the live Artsdata API returns — see docs/interop/artsdata.md §3.4.
			baseURL := "http://" + r.Host
			entity := map[string]interface{}{
				"@context": "http://schema.org",
				"id":       baseURL + r.URL.Path,
				"type":     "Place",
				"name": map[string]interface{}{
					"en":    "Art Gallery of Ontario",
					"@none": "Art Gallery of Ontario",
				},
				"sameAs": []string{
					"http://www.wikidata.org/entity/Q319378",
					"http://www.openstreetmap.org/relation/123456",
				},
				"address": map[string]interface{}{
					"id":              baseURL + r.URL.Path + "#PostalAddress",
					"type":            "PostalAddress",
					"streetAddress":   map[string]interface{}{"@none": "317 Dundas Street West"},
					"addressLocality": map[string]interface{}{"@none": "Toronto"},
					"addressRegion":   map[string]interface{}{"@none": "ON"},
					"postalCode":      "M5T 1G4",
					"addressCountry":  map[string]interface{}{"@none": "CA"},
				},
			}
			w.Header().Set("Content-Type", "application/ld+json")
			if err := json.NewEncoder(w).Encode(entity); err != nil {
				http.Error(w, "failed to encode entity", http.StatusInternalServerError)
				return
			}

		default:
			http.NotFound(w, r)
		}
	}))
}

// TestReconciliationServiceDirectly tests the ReconciliationService with a mock Artsdata API.
func TestReconciliationServiceDirectly(t *testing.T) {
	env := setupTestEnv(t)

	// Ensure authority codes are seeded (resetDatabase truncates them)
	seedAuthorityCodesIfNeeded(env.Context, t, env.Pool)

	// Create mock Artsdata server
	mockServer := newMockArtsdataServer(t, nil)
	defer mockServer.Close()

	// Create Artsdata client pointing to mock server
	artsdataClient := artsdata.NewClient(mockServer.URL + "/recon")

	// Create queries instance
	queries := postgres.New(env.Pool)

	// Create reconciliation service
	service := kg.NewReconciliationService(
		artsdataClient,
		queries,
		identity.NewStore(env.Pool),
		slog.Default(),
		30*24*time.Hour, // 30 days cache TTL
		7*24*time.Hour,  // 7 days failure TTL
	)

	// Create a test place in the database
	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality, postal_code, address_country) 
		 VALUES ($1, $2, $3, $4, $5) 
		 RETURNING id`,
		placeULID, "Art Gallery of Ontario", "Toronto", "M5T 1L9", "Canada",
	).Scan(&placeID)
	require.NoError(t, err, "failed to insert test place")

	// Call ReconcileEntity
	req := kg.ReconcileRequest{
		EntityType: "place",
		EntityID:   placeULID,
		Name:       "Art Gallery of Ontario",
		Properties: map[string]string{
			"addressLocality": "Toronto",
			"postalCode":      "M5T 1L9",
		},
	}

	matches, err := service.ReconcileEntity(env.Context, req)
	require.NoError(t, err, "ReconcileEntity should succeed")
	require.NotEmpty(t, matches, "should have at least one match")

	// Verify match properties
	match := matches[0]
	assert.Equal(t, "artsdata", match.AuthorityCode)
	assert.Contains(t, match.IdentifierURI, "/resource/K11-211", "identifier URI should contain the resource path")
	assert.Greater(t, match.Confidence, 0.95, "confidence should be high")
	assert.Equal(t, "auto_high", match.Method)

	// Verify sameAs URIs were extracted from dereference
	assert.NotEmpty(t, match.SameAsURIs, "should have sameAs URIs from dereference")
	assert.Contains(t, match.SameAsURIs, "http://www.wikidata.org/entity/Q319378")
	assert.Contains(t, match.SameAsURIs, "http://www.openstreetmap.org/relation/123456")

	// Verify entity_identifiers were created in the database
	// Should have: artsdata (primary) + wikidata + osm (transitive sameAs)
	var count int
	err = env.Pool.QueryRow(env.Context,
		`SELECT COUNT(*) FROM entity_identifiers 
		 WHERE entity_type = 'place' AND entity_id = $1`,
		placeULID,
	).Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 3, "should have at least 3 identifiers: artsdata + wikidata + osm")

	// Verify reconciliation_cache was populated
	lookupKey := kg.NormalizeLookupKey("place", "Art Gallery of Ontario", map[string]string{
		"addressLocality": "Toronto",
		"postalCode":      "M5T 1L9",
	})

	cached, err := queries.GetReconciliationCache(env.Context, postgres.GetReconciliationCacheParams{
		EntityType:    "place",
		AuthorityCode: "artsdata",
		LookupKey:     lookupKey,
	})
	require.NoError(t, err, "cache entry should exist")
	assert.False(t, cached.IsNegative, "cache should not be negative")

	var cachedMatches []kg.MatchResult
	err = json.Unmarshal(cached.ResultJson, &cachedMatches)
	require.NoError(t, err)
	assert.Len(t, cachedMatches, len(matches), "cached matches should match result")
}

// TestReconciliationCacheHit tests that the cache works and prevents duplicate API calls.
func TestReconciliationCacheHit(t *testing.T) {
	env := setupTestEnv(t)

	// Ensure authority codes are seeded
	seedAuthorityCodesIfNeeded(env.Context, t, env.Pool)

	// Create mock Artsdata server with call counter
	var callCounter atomic.Int32
	mockServer := newMockArtsdataServer(t, &callCounter)
	defer mockServer.Close()

	// Create Artsdata client pointing to mock server
	artsdataClient := artsdata.NewClient(mockServer.URL + "/recon")

	// Create queries instance
	queries := postgres.New(env.Pool)

	// Create reconciliation service
	service := kg.NewReconciliationService(
		artsdataClient,
		queries,
		identity.NewStore(env.Pool),
		slog.Default(),
		30*24*time.Hour,
		7*24*time.Hour,
	)

	// Create a test place
	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality, postal_code, address_country) 
		 VALUES ($1, $2, $3, $4, $5) 
		 RETURNING id`,
		placeULID, "Toronto Reference Library", "Toronto", "M4W 2G8", "Canada",
	).Scan(&placeID)
	require.NoError(t, err)

	req := kg.ReconcileRequest{
		EntityType: "place",
		EntityID:   placeULID,
		Name:       "Toronto Reference Library",
		Properties: map[string]string{
			"addressLocality": "Toronto",
			"postalCode":      "M4W 2G8",
		},
	}

	// First reconciliation - should call API
	matches1, err := service.ReconcileEntity(env.Context, req)
	require.NoError(t, err)
	require.NotEmpty(t, matches1)

	// Verify API was called once
	firstCallCount := callCounter.Load()
	assert.Equal(t, int32(1), firstCallCount, "API should be called once")

	// Second reconciliation with same data - should hit cache
	matches2, err := service.ReconcileEntity(env.Context, req)
	require.NoError(t, err)
	require.NotEmpty(t, matches2)

	// Verify API was NOT called again
	secondCallCount := callCounter.Load()
	assert.Equal(t, firstCallCount, secondCallCount, "API should not be called again (cache hit)")

	// Verify results are identical
	assert.Equal(t, matches1[0].IdentifierURI, matches2[0].IdentifierURI)
	assert.Equal(t, matches1[0].Confidence, matches2[0].Confidence)
}

// TestReconciliationNegativeCache tests negative cache (no match found).
func TestReconciliationNegativeCache(t *testing.T) {
	env := setupTestEnv(t)

	// Ensure authority codes are seeded
	seedAuthorityCodesIfNeeded(env.Context, t, env.Pool)

	// Create mock Artsdata server with call counter
	var callCounter atomic.Int32
	mockServer := newMockArtsdataServer(t, &callCounter)
	defer mockServer.Close()

	// Create Artsdata client
	artsdataClient := artsdata.NewClient(mockServer.URL + "/recon")
	queries := postgres.New(env.Pool)

	// Create reconciliation service
	service := kg.NewReconciliationService(
		artsdataClient,
		queries,
		identity.NewStore(env.Pool),
		slog.Default(),
		30*24*time.Hour,
		7*24*time.Hour,
	)

	// Create a test place with a name that returns no matches
	placeULID := ulid.Make().String()
	var placeID string
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO places (ulid, name, address_locality, postal_code, address_country) 
		 VALUES ($1, $2, $3, $4, $5) 
		 RETURNING id`,
		placeULID, "No Match Place", "Toronto", "M1M 1M1", "Canada",
	).Scan(&placeID)
	require.NoError(t, err)

	req := kg.ReconcileRequest{
		EntityType: "place",
		EntityID:   placeULID,
		Name:       "No Match Place",
		Properties: map[string]string{
			"addressLocality": "Toronto",
			"postalCode":      "M1M 1M1",
		},
	}

	// First reconciliation - should call API and get no results
	matches1, err := service.ReconcileEntity(env.Context, req)
	require.NoError(t, err)
	assert.Empty(t, matches1, "should have no matches")

	// Verify API was called
	firstCallCount := callCounter.Load()
	assert.Equal(t, int32(1), firstCallCount)

	// Verify negative cache entry was created
	lookupKey := kg.NormalizeLookupKey("place", "No Match Place", map[string]string{
		"addressLocality": "Toronto",
		"postalCode":      "M1M 1M1",
	})

	cached, err := queries.GetReconciliationCache(env.Context, postgres.GetReconciliationCacheParams{
		EntityType:    "place",
		AuthorityCode: "artsdata",
		LookupKey:     lookupKey,
	})
	require.NoError(t, err, "negative cache entry should exist")
	assert.True(t, cached.IsNegative, "cache should be marked as negative")

	// Second reconciliation - should hit negative cache
	matches2, err := service.ReconcileEntity(env.Context, req)
	require.NoError(t, err)
	assert.Empty(t, matches2, "should still have no matches")

	// Verify API was NOT called again
	secondCallCount := callCounter.Load()
	assert.Equal(t, firstCallCount, secondCallCount, "API should not be called again (negative cache hit)")
}

// TestReconciliationViaEventIngestion exercises the REAL ingest pipeline through the running
// httptest router and asserts that reconciliation jobs are enqueued for the newly created place
// and organization. River workers are intentionally NOT started in this package (see
// setupTestEnv in helpers_test.go), so this test only asserts enqueue — full job execution is
// covered by tests/integration_batch.
func TestReconciliationViaEventIngestion(t *testing.T) {
	env := setupTestEnv(t)

	apiKey := insertAPIKey(t, env, "reconciliation-ingest-agent")

	// POST an event that references a place (venue) and an organization (organizer).
	// The ingest pipeline must create both, populating IngestResult.PlaceULID/OrganizerULID,
	// which is what drives the reconciliation job enqueues in the events handler.
	const venueName = "Reconciliation Test Venue"
	const orgName = "Reconciliation Test Organization"

	payload := map[string]any{
		"name":        "Reconciliation Ingest Test Event",
		"description": "Event used to verify reconciliation jobs are enqueued through the ingest pipeline.",
		"startDate":   time.Now().Add(48 * time.Hour).Format(time.RFC3339),
		"location": map[string]any{
			"name":            venueName,
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
			"addressCountry":  "CA",
		},
		"organizer": map[string]any{
			"name": orgName,
		},
		"source": map[string]any{
			"url":     "https://example.com/events/reconciliation-ingest",
			"eventId": "reconciliation-ingest-1",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusCreated {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "status=%d response=%v", resp.StatusCode, failure)
	}

	// The ingest pipeline must have created both the venue and the organizer.
	var placeCount, orgCount int
	require.NoError(t, env.Pool.QueryRow(env.Context,
		`SELECT COUNT(*) FROM places WHERE name = $1`, venueName).Scan(&placeCount))
	require.NoError(t, env.Pool.QueryRow(env.Context,
		`SELECT COUNT(*) FROM organizations WHERE name = $1`, orgName).Scan(&orgCount))
	require.Equal(t, 1, placeCount, "ingest should create exactly one place for the venue")
	require.Equal(t, 1, orgCount, "ingest should create exactly one organization for the organizer")

	// Assert reconciliation jobs were enqueued for the newly created place and organization.
	var jobCount int
	require.NoError(t, env.Pool.QueryRow(env.Context,
		`SELECT COUNT(*) FROM river_job WHERE kind = $1`, jobs.JobKindReconciliation).Scan(&jobCount))
	assert.Greater(t, jobCount, 0, "expected at least one reconciliation job to be enqueued")

	// Precisely: one place reconciliation job and one organization reconciliation job.
	var placeJobs, orgJobs int
	require.NoError(t, env.Pool.QueryRow(env.Context,
		`SELECT COUNT(*) FROM river_job WHERE kind = $1 AND args->>'entity_type' = 'place'`,
		jobs.JobKindReconciliation).Scan(&placeJobs))
	require.NoError(t, env.Pool.QueryRow(env.Context,
		`SELECT COUNT(*) FROM river_job WHERE kind = $1 AND args->>'entity_type' = 'organization'`,
		jobs.JobKindReconciliation).Scan(&orgJobs))
	assert.Equal(t, 1, placeJobs, "expected exactly one place reconciliation job")
	assert.Equal(t, 1, orgJobs, "expected exactly one organization reconciliation job")
}
