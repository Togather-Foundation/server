package integration_batch

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/Togather-Foundation/server/tests/testhelpers"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedAuthorityCodes reseeds the knowledge_graph_authorities table after
// testhelpers.ResetDatabase truncates it. The entity_identifiers.authority_code
// column is a foreign key to this table, so reconciliation would fail without it.
func seedAuthorityCodes(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	var count int
	err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM knowledge_graph_authorities`).Scan(&count)
	require.NoError(t, err)
	if count > 0 {
		return
	}

	_, err = pool.Exec(context.Background(), `
		INSERT INTO knowledge_graph_authorities (authority_code, authority_name, base_uri_pattern, reconciliation_endpoint, applicable_domains, trust_level, priority_order, rate_limit_per_minute, rate_limit_per_day, documentation_url) VALUES
		('artsdata', 'Artsdata Knowledge Graph', '^http://kg\.artsdata\.ca/resource/K\d+-\d+$', 'https://api.artsdata.ca/recon', ARRAY['arts', 'culture', 'music'], 9, 10, 60, 10000, 'https://docs.artsdata.ca/'),
		('wikidata', 'Wikidata', '^http://www\.wikidata\.org/entity/Q\d+$', NULL, ARRAY['arts', 'culture', 'music', 'sports', 'community', 'education', 'general'], 8, 20, 30, 5000, 'https://www.wikidata.org/'),
		('musicbrainz', 'MusicBrainz', '^https://musicbrainz\.org/(artist|event|place)/[0-9a-f-]+$', NULL, ARRAY['music'], 9, 15, 30, 5000, 'https://musicbrainz.org/doc/MusicBrainz_API'),
		('isni', 'ISNI', '^https?://isni\.org/isni/\d{16}$', NULL, ARRAY['arts', 'culture', 'music', 'education'], 9, 30, 10, 1000, 'https://isni.org/'),
		('osm', 'OpenStreetMap', '^https://www\.openstreetmap\.org/(node|way|relation)/\d+$', NULL, ARRAY['arts', 'culture', 'music', 'sports', 'community', 'education', 'general'], 7, 40, 60, 10000, 'https://wiki.openstreetmap.org/')
	`)
	require.NoError(t, err, "failed to seed authority codes")
}

// newMockArtsdataServer simulates the Artsdata W3C Reconciliation API with
// REALISTIC score semantics (see docs/interop/artsdata.md §3.1):
// exact matches carry an unbounded score (~1000+) with match:true, which the
// ReconciliationService normalizes to 0.99 and classifies as auto_high.
// The returned identifier URI points back at this mock so the follow-up
// dereference (GET /resource/...) also lands here.
//
// The returned *atomic.Int32 counts every dereference (GET /resource/...) the mock
// serves, so tests can assert enrichment freshness (a fresh entity must NOT trigger
// another dereference).
func newMockArtsdataServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var derefCounter atomic.Int32

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/recon" && r.Method == "POST":
			// Form-encoded batch per W3C Reconciliation API v0.2 §4.3.
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			queriesParam := r.FormValue("queries")
			if queriesParam == "" {
				http.Error(w, "missing queries parameter", http.StatusBadRequest)
				return
			}

			var queries map[string]interface{}
			if err := json.Unmarshal([]byte(queriesParam), &queries); err != nil {
				http.Error(w, "invalid queries JSON", http.StatusBadRequest)
				return
			}

			baseURL := "http://" + r.Host
			response := make(map[string]interface{})
			for queryID, queryData := range queries {
				queryMap, ok := queryData.(map[string]interface{})
				if !ok {
					continue
				}
				queryName, _ := queryMap["query"].(string)

				// Realistic exact-match semantics: match=true with an unbounded
				// score (~1247.4), normalized to 0.99 → auto_high.
				response[queryID] = map[string]interface{}{
					"result": []interface{}{
						map[string]interface{}{
							"id":    baseURL + "/resource/K11-211",
							"name":  queryName,
							"score": 1247.4,
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
			derefCounter.Add(1)
			// Dereferenced entity with sameAs links and enrichment fields, using the
			// realistic *compacted* JSON-LD shape (id/type aliases, @none-wrapped
			// address scalars) the live Artsdata API returns — see docs/interop/artsdata.md §3.4.
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
				"description": "The Art Gallery of Ontario (AGO) is an art museum in Toronto.",
				"url":         "https://ago.ca",
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
	})), &derefCounter
}

// waitForJobState polls the river_job table until a job of the given kind for the
// given entity reaches the expected state (e.g. 'completed'). This is the DB-backed
// equivalent of awaitBatchCompletion, which relies on the shared River client that
// this test does not own.
func waitForJobState(t *testing.T, pool *pgxpool.Pool, kind, entityType, entityID, state string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var count int
		err := pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM river_job
			 WHERE kind = $1 AND args->>'entity_type' = $2 AND args->>'entity_id' = $3 AND state = $4`,
			kind, entityType, entityID, state).Scan(&count)
		require.NoError(t, err)
		if count > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for %s job for %s %s to reach state %q", timeout, kind, entityType, entityID, state)
}

// waitForJobCount polls the river_job table until at least minCount jobs of the given
// kind for the given entity have reached the expected state. Used when a prior job of
// the same kind/entity already completed and a subsequent ingest enqueues another.
func waitForJobCount(t *testing.T, pool *pgxpool.Pool, kind, entityType, entityID, state string, minCount int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var count int
		err := pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM river_job
			 WHERE kind = $1 AND args->>'entity_type' = $2 AND args->>'entity_id' = $3 AND state = $4`,
			kind, entityType, entityID, state).Scan(&count)
		require.NoError(t, err)
		if count >= minCount {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for %d %s jobs for %s %s in state %q", timeout, minCount, kind, entityType, entityID, state)
}

// ingestEvent POSTs a single event (JSON-LD) to the running test server and asserts
// a 201 Created response. Each call uses a distinct event name and source eventId so
// the event itself is unique, while the venue name stays constant to exercise the
// venue-resolution path.
func ingestEvent(t *testing.T, server *httptest.Server, apiKey, name, venueName, eventID string) {
	t.Helper()

	payload := map[string]any{
		"name":        name,
		"description": "Opening night for a new exhibition.",
		"startDate":   time.Now().Add(48 * time.Hour).Format(time.RFC3339),
		"location": map[string]any{
			"name":            venueName,
			"addressLocality": "Toronto",
			"addressRegion":   "ON",
			"addressCountry":  "CA",
			"postalCode":      "M5T 1G4",
		},
		"source": map[string]any{
			"url":     "https://example.com/events/" + eventID,
			"eventId": eventID,
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/api/v1/events", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set("Accept", "application/ld+json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusCreated {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		require.Failf(t, "unexpected status", "status=%d response=%v", resp.StatusCode, failure)
	}
}

// TestReconciliationEndToEnd exercises the full live reconciliation path with running
// River workers: event ingestion → reconciliation job → Artsdata lookup →
// entity_identifiers + reconciliation_cache → enrichment (dereference + sameAs + field fill).
func TestReconciliationEndToEnd(t *testing.T) {
	initShared(t)
	testhelpers.ResetDatabase(t, sharedPool)

	mockServer, derefCounter := newMockArtsdataServer(t)
	t.Cleanup(mockServer.Close)

	// Build a config with Artsdata enabled, pointed at the mock server.
	cfg := testhelpers.TestConfig(sharedDBURL)
	cfg.Artsdata = config.ArtsdataConfig{
		Endpoint:              mockServer.URL + "/recon",
		Enabled:               true,
		RateLimitPerSec:       100,
		TimeoutSeconds:        10,
		CacheTTLDays:          30,
		FailureTTLDays:        7,
		EnrichmentRefreshDays: 30,
	}

	routerWithClient := api.NewRouter(cfg, testhelpers.TestLogger(), sharedPool, "test", "test-commit", "test-date")
	require.NotNil(t, routerWithClient.RiverClient, "router must wire a River client when Artsdata is enabled")

	err := routerWithClient.RiverClient.Start(context.Background())
	require.NoError(t, err, "failed to start River workers")
	t.Cleanup(func() {
		if err := routerWithClient.RiverClient.Stop(context.Background()); err != nil {
			t.Logf("failed to stop river workers: %v", err)
		}
	})

	server := httptest.NewServer(routerWithClient.Handler)
	t.Cleanup(server.Close)

	// Authority codes are a foreign-key prerequisite for entity_identifiers.
	seedAuthorityCodes(t, sharedPool)

	apiKey := testhelpers.InsertAPIKey(t, sharedPool, context.Background(), "reconciliation-e2e-agent")

	// Ingest an event referencing a known venue. The ingest pipeline creates the
	// place and the events handler enqueues a reconciliation job for it.
	const venueName = "Art Gallery of Ontario"

	ingestEvent(t, server, apiKey, "AGO Exhibition Opening", venueName, "ago-opening-1")

	// Resolve the ULID of the newly created place.
	var placeULID string
	require.NoError(t, sharedPool.QueryRow(context.Background(),
		`SELECT ulid FROM places WHERE name = $1`, venueName).Scan(&placeULID))
	require.NotEmpty(t, placeULID)

	// Wait for the reconciliation job to run to completion.
	waitForJobState(t, sharedPool, jobs.JobKindReconciliation, "place", placeULID, "completed", 30*time.Second)

	// The Artsdata identifier must be stored for the place.
	var artsdataCount int
	require.NoError(t, sharedPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM entity_identifiers
		 WHERE entity_type = 'place' AND entity_id = $1 AND authority_code = 'artsdata'`,
		placeULID).Scan(&artsdataCount))
	assert.GreaterOrEqual(t, artsdataCount, 1, "expected an artsdata entity_identifiers row for the place")

	// The reconciliation result must be cached (positive, not negative).
	var cacheCount int
	require.NoError(t, sharedPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM reconciliation_cache
		 WHERE entity_type = 'place' AND authority_code = 'artsdata' AND is_negative = false`,
	).Scan(&cacheCount))
	assert.GreaterOrEqual(t, cacheCount, 1, "expected a positive reconciliation_cache row for the place")

	// The match classified as auto_high, so an enrichment job must have run too.
	waitForJobState(t, sharedPool, jobs.JobKindEnrichment, "place", placeULID, "completed", 30*time.Second)

	// Enrichment stores transitive sameAs identifiers (wikidata + osm).
	var sameAsCount int
	require.NoError(t, sharedPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM entity_identifiers
		 WHERE entity_type = 'place' AND entity_id = $1 AND authority_code IN ('wikidata', 'osm')`,
		placeULID).Scan(&sameAsCount))
	assert.GreaterOrEqual(t, sameAsCount, 1, "expected sameAs identifiers stored via enrichment")

	// Enrichment conservatively fills empty place fields from the Artsdata entity.
	var description, url, streetAddress string
	require.NoError(t, sharedPool.QueryRow(context.Background(),
		`SELECT COALESCE(description, ''), COALESCE(url, ''), COALESCE(street_address, '') FROM places WHERE ulid = $1`,
		placeULID).Scan(&description, &url, &streetAddress))
	assert.NotEmpty(t, description, "expected enrichment to fill the place description")
	assert.Equal(t, "https://ago.ca", url, "expected enrichment to fill the place URL")
	assert.NotEmpty(t, streetAddress, "expected enrichment to fill the place street address")

	// ── Enrichment freshness (enriched_at TTL marker) ─────────────────────────
	//
	// Note: a single auto_high reconciliation dereferences once in the reconciliation
	// service (to extract sameAs) AND once in the enrichment worker, so the first
	// cycle serves two /resource GETs. We measure deltas relative to that baseline,
	// not absolute counts, so the assertion holds regardless of that detail.
	var enrichedAt time.Time
	require.NoError(t, sharedPool.QueryRow(context.Background(),
		`SELECT enriched_at FROM places WHERE ulid = $1`, placeULID).Scan(&enrichedAt),
		"expected enriched_at to be set after first enrichment")
	firstDeref := derefCounter.Load()

	// A SECOND event at the same venue reuses the same place; its enrichment job must
	// be skipped because enriched_at is fresh — no new dereference, enriched_at unchanged.
	ingestEvent(t, server, apiKey, "AGO Members Preview", venueName, "ago-opening-2")

	waitForJobCount(t, sharedPool, jobs.JobKindReconciliation, "place", placeULID, "completed", 2, 30*time.Second)
	waitForJobCount(t, sharedPool, jobs.JobKindEnrichment, "place", placeULID, "completed", 2, 30*time.Second)

	assert.Equal(t, firstDeref, derefCounter.Load(), "expected no additional dereference for a fresh entity")

	var enrichedAtAfterSecond time.Time
	require.NoError(t, sharedPool.QueryRow(context.Background(),
		`SELECT enriched_at FROM places WHERE ulid = $1`, placeULID).Scan(&enrichedAtAfterSecond))
	assert.Equal(t, enrichedAt, enrichedAtAfterSecond, "enriched_at must be unchanged when enrichment is skipped")

	// Artificially age enriched_at past the refresh TTL; the next ingest must dereference again.
	_, err = sharedPool.Exec(context.Background(),
		`UPDATE places SET enriched_at = now() - interval '40 days' WHERE ulid = $1`, placeULID)
	require.NoError(t, err)

	ingestEvent(t, server, apiKey, "AGO Fall Gala", venueName, "ago-opening-3")

	waitForJobCount(t, sharedPool, jobs.JobKindReconciliation, "place", placeULID, "completed", 3, 30*time.Second)
	waitForJobCount(t, sharedPool, jobs.JobKindEnrichment, "place", placeULID, "completed", 3, 30*time.Second)

	assert.Equal(t, firstDeref+1, derefCounter.Load(), "expected exactly one re-dereference after enriched_at aged past TTL")

	var enrichedAtAfterThird time.Time
	require.NoError(t, sharedPool.QueryRow(context.Background(),
		`SELECT enriched_at FROM places WHERE ulid = $1`, placeULID).Scan(&enrichedAtAfterThird))
	assert.True(t, enrichedAtAfterThird.After(enrichedAt), "enriched_at must be refreshed after re-dereference")
}
