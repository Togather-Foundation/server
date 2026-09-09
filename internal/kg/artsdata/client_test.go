package artsdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestReconcile_Success(t *testing.T) {
	tests := []struct {
		name       string
		queries    map[string]ReconciliationQuery
		serverResp string
		wantCount  int
	}{
		{
			name: "place with high confidence match",
			queries: map[string]ReconciliationQuery{
				"q0": {
					Query: "Massey Hall",
					Type:  "schema:Place",
					Properties: []QueryProperty{
						{P: "schema:address/schema:addressLocality", V: "Toronto"},
						{P: "schema:address/schema:postalCode", V: "M5B"},
					},
				},
			},
			serverResp: `{
				"q0": {
					"result": [
						{
							"id": "http://kg.artsdata.ca/resource/K11-211",
							"name": "Massey Hall",
							"score": 98.5,
							"match": true,
							"type": [{"id": "http://schema.org/Place", "name": "Place"}]
						}
					]
				}
			}`,
			wantCount: 1,
		},
		{
			name: "organization with URL",
			queries: map[string]ReconciliationQuery{
				"q0": {
					Query: "Canadian Opera Company",
					Type:  "schema:Organization",
					Properties: []QueryProperty{
						{P: "schema:url", V: "https://coc.ca"},
					},
				},
			},
			serverResp: `{
				"q0": {
					"result": [
						{
							"id": "http://kg.artsdata.ca/resource/K23-456",
							"name": "Canadian Opera Company",
							"score": 95.0,
							"match": true,
							"type": [{"id": "http://schema.org/Organization", "name": "Organization"}]
						}
					]
				}
			}`,
			wantCount: 1,
		},
		{
			name: "multiple results with varying confidence",
			queries: map[string]ReconciliationQuery{
				"q0": {
					Query: "Arts Centre",
					Type:  "schema:Place",
				},
			},
			serverResp: `{
				"q0": {
					"result": [
						{
							"id": "http://kg.artsdata.ca/resource/K11-100",
							"name": "National Arts Centre",
							"score": 85.0,
							"match": false
						},
						{
							"id": "http://kg.artsdata.ca/resource/K11-200",
							"name": "Toronto Arts Centre",
							"score": 82.0,
							"match": false
						}
					]
				}
			}`,
			wantCount: 2,
		},
		{
			name: "no results",
			queries: map[string]ReconciliationQuery{
				"q0": {
					Query: "Nonexistent Venue",
					Type:  "schema:Place",
				},
			},
			serverResp: `{
				"q0": {
					"result": []
				}
			}`,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
				assert.Contains(t, r.Header.Get("User-Agent"), "Togather")

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.serverResp))
			}))
			defer server.Close()

			client := NewClient(server.URL)
			ctx := context.Background()

			results, err := client.Reconcile(ctx, tt.queries)
			require.NoError(t, err)

			require.Contains(t, results, "q0")
			assert.Len(t, results["q0"], tt.wantCount)

			if tt.wantCount > 0 {
				result := results["q0"][0]
				assert.NotEmpty(t, result.ID)
				assert.NotEmpty(t, result.Name)
				assert.Greater(t, result.Score, 0.0)
			}
		})
	}
}

func TestReconcile_EmptyQueries(t *testing.T) {
	client := NewClient(DefaultEndpoint)
	ctx := context.Background()

	_, err := client.Reconcile(ctx, map[string]ReconciliationQuery{})
	assert.ErrorContains(t, err, "queries cannot be empty")
}

func TestReconcile_RateLimiting(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"q0": {"result": []}}`))
	}))
	defer server.Close()

	// Set very low rate limit for testing
	client := NewClient(server.URL, WithRateLimit(5.0)) // 5 req/sec
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 3; i++ {
		queries := map[string]ReconciliationQuery{
			"q0": {Query: "Test", Type: "schema:Place"},
		}
		_, err := client.Reconcile(ctx, queries)
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	assert.Equal(t, 3, callCount)
	// With 5 req/sec, 3 requests should take at least 400ms (not instant)
	assert.Greater(t, elapsed, 200*time.Millisecond)
}

func TestReconcile_RetryOn429(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"q0": {"result": []}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, WithRateLimit(100.0)) // High rate to test retry logic
	ctx := context.Background()

	queries := map[string]ReconciliationQuery{
		"q0": {Query: "Test", Type: "schema:Place"},
	}
	_, err := client.Reconcile(ctx, queries)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount) // First attempt failed, second succeeded
}

func TestReconcile_RetryOn5xx(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"q0": {"result": []}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, WithRateLimit(100.0))
	ctx := context.Background()

	queries := map[string]ReconciliationQuery{
		"q0": {Query: "Test", Type: "schema:Place"},
	}
	_, err := client.Reconcile(ctx, queries)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestReconcile_MaxRetriesExceeded(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, WithRateLimit(100.0))
	ctx := context.Background()

	queries := map[string]ReconciliationQuery{
		"q0": {Query: "Test", Type: "schema:Place"},
	}
	_, err := client.Reconcile(ctx, queries)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "max retries exceeded")
	assert.Equal(t, MaxRetries+1, callCount) // Initial attempt + MaxRetries
}

func TestDereference_Success(t *testing.T) {
	serverResp := `{
		"@context": "https://schema.org",
		"@id": "http://kg.artsdata.ca/resource/K11-211",
		"@type": "Place",
		"name": "Massey Hall",
		"address": {
			"@type": "PostalAddress",
			"streetAddress": "178 Victoria St",
			"addressLocality": "Toronto",
			"addressRegion": "ON",
			"postalCode": "M5B 1T7",
			"addressCountry": "CA"
		},
		"sameAs": [
			"http://www.wikidata.org/entity/Q1234567",
			"https://www.openstreetmap.org/way/12345"
		],
		"url": "https://masseyhall.com"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "application/ld+json", r.Header.Get("Accept"))

		w.Header().Set("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(serverResp))
	}))
	defer server.Close()

	client := NewClient(DefaultEndpoint)
	ctx := context.Background()

	entity, err := client.Dereference(ctx, server.URL+"/resource/K11-211")
	require.NoError(t, err)
	require.NotNil(t, entity)

	assert.Equal(t, "http://kg.artsdata.ca/resource/K11-211", entity.ID)
	assert.Equal(t, "Place", entity.Type)
	assert.NotNil(t, entity.Address)
	assert.Equal(t, "Toronto", entity.Address.AddressLocality)
	assert.NotEmpty(t, entity.RawJSON)
}

func TestDereference_EmptyURI(t *testing.T) {
	client := NewClient(DefaultEndpoint)
	ctx := context.Background()

	_, err := client.Dereference(ctx, "")
	assert.ErrorContains(t, err, "uri cannot be empty")
}

func TestDereference_CompactedArtsdataJSONLD(t *testing.T) {
	// Load the live-recorded dereference fixture (tests/testdata/artsdata/dereference.json)
	// so the unit test pins the REAL compacted JSON-LD shape, not a hand-authored one.
	fixtures, err := LoadFixtures(fixturesDir())
	require.NoError(t, err)

	var body []byte
	for _, f := range fixtures {
		if f.Name == "dereference" {
			body = f.Response.Body
			break
		}
	}
	require.NotEmpty(t, body, "dereference fixture body must be present (run ARTSDATA_RECORD=1 to refresh)")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client := NewClient(server.URL, WithRateLimit(1000.0))
	entity, err := client.Dereference(context.Background(), server.URL+"/resource/K11-24")
	require.NoError(t, err)
	require.NotNil(t, entity)

	// id/type aliases are accepted (not just @id/@type).
	assert.Equal(t, "http://kg.artsdata.ca/resource/K11-24", entity.ID)
	typeArr, ok := entity.Type.([]interface{})
	require.True(t, ok, "type should decode as an array of strings")
	assert.Contains(t, typeArr, "Place")
	assert.Contains(t, typeArr, "MusicVenue")

	// name is a language map that resolves preferring "@none".
	assert.Equal(t, "Massey Hall", resolveString(entity.Name))

	// address scalars resolve from @none wrappers and plain strings; the address's
	// own id/type keys are ignored.
	require.NotNil(t, entity.Address)
	assert.Equal(t, "178 Victoria St", entity.Address.StreetAddress)
	assert.Equal(t, "Toronto", entity.Address.AddressLocality)
	assert.Equal(t, "ON", entity.Address.AddressRegion)
	assert.Equal(t, "M5B 1T7", entity.Address.PostalCode)
	assert.Equal(t, "CA", entity.Address.AddressCountry)

	// sameAs is still extracted.
	uris := ExtractSameAsURIs(entity)
	require.Len(t, uris, 7)
	assert.Contains(t, uris, "http://www.wikidata.org/entity/Q1122776")

	// url is a plain string.
	assert.Equal(t, "https://www.masseyhall.com", resolveString(entity.URL))

	// typed value (maximumAttendeeCapacity) decodes without error and is preserved
	// in the raw JSON.
	assert.True(t, strings.Contains(string(entity.RawJSON), "maximumAttendeeCapacity"))
	assert.True(t, strings.Contains(string(entity.RawJSON), "xsd:integer"))
}

func TestDereference_CompactedArtsdataOrganizationJSONLD(t *testing.T) {
	// Load the live-recorded organization dereference fixture
	// (tests/testdata/artsdata/dereference-org.json) so the unit test pins the
	// REAL compacted organization shape, not a hand-authored one. Organizations
	// dereference differently from places: a plain-string `type`, an `en`-only
	// language-map name/description (no @none), a string-array sameAs, and no
	// address/url block.
	fixtures, err := LoadFixtures(fixturesDir())
	require.NoError(t, err)

	var body []byte
	for _, f := range fixtures {
		if f.Name == "dereference-org" {
			body = f.Response.Body
			break
		}
	}
	require.NotEmpty(t, body, "organization dereference fixture body must be present (run ARTSDATA_RECORD=1 to refresh)")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client := NewClient(server.URL, WithRateLimit(1000.0))
	entity, err := client.Dereference(context.Background(), server.URL+"/resource/K2-5143")
	require.NoError(t, err)
	require.NotNil(t, entity)

	// id/type aliases are accepted; organization `type` is a plain string (not an array).
	assert.Equal(t, "http://kg.artsdata.ca/resource/K2-5143", entity.ID)
	typeStr, ok := entity.Type.(string)
	require.True(t, ok, "organization type should decode as a plain string")
	assert.Equal(t, "Organization", typeStr)

	// name/description are en-only language maps that resolve via the "en" key.
	assert.Equal(t, "Canadian Opera Company", resolveString(entity.Name))
	assert.Contains(t, resolveString(entity.Description), "Canadian Opera Company")

	// sameAs is a string array (Wikidata + ISNI + Wikipedia).
	uris := ExtractSameAsURIs(entity)
	require.Len(t, uris, 4)
	assert.Contains(t, uris, "http://www.wikidata.org/entity/Q2915268")
	assert.Contains(t, uris, "https://isni.org/isni/0000000121842483")

	// organizations carry no PostalAddress and no url — the parser must leave
	// these empty rather than error (the enrichment worker guards entity.Address).
	assert.Nil(t, entity.Address)
	assert.Empty(t, resolveString(entity.URL))
}

func TestExtractSameAsURIs(t *testing.T) {
	tests := []struct {
		name     string
		sameAs   interface{}
		wantURIs []string
	}{
		{
			name:     "single string",
			sameAs:   "http://www.wikidata.org/entity/Q1234567",
			wantURIs: []string{"http://www.wikidata.org/entity/Q1234567"},
		},
		{
			name: "array of strings",
			sameAs: []interface{}{
				"http://www.wikidata.org/entity/Q1234567",
				"https://musicbrainz.org/artist/abc-123",
			},
			wantURIs: []string{
				"http://www.wikidata.org/entity/Q1234567",
				"https://musicbrainz.org/artist/abc-123",
			},
		},
		{
			name: "array of objects with @id",
			sameAs: []interface{}{
				map[string]interface{}{"@id": "http://www.wikidata.org/entity/Q1234567"},
				map[string]interface{}{"@id": "https://isni.org/isni/0000000123456789"},
			},
			wantURIs: []string{
				"http://www.wikidata.org/entity/Q1234567",
				"https://isni.org/isni/0000000123456789",
			},
		},
		{
			name:     "single object with @id",
			sameAs:   map[string]interface{}{"@id": "http://www.wikidata.org/entity/Q1234567"},
			wantURIs: []string{"http://www.wikidata.org/entity/Q1234567"},
		},
		{
			name: "mixed array (strings and objects)",
			sameAs: []interface{}{
				"http://www.wikidata.org/entity/Q1234567",
				map[string]interface{}{"@id": "https://isni.org/isni/0000000123456789"},
			},
			wantURIs: []string{
				"http://www.wikidata.org/entity/Q1234567",
				"https://isni.org/isni/0000000123456789",
			},
		},
		{
			name:     "nil sameAs",
			sameAs:   nil,
			wantURIs: nil,
		},
		{
			name:     "empty array",
			sameAs:   []interface{}{},
			wantURIs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity := &EntityData{SameAs: tt.sameAs}
			uris := ExtractSameAsURIs(entity)

			if tt.wantURIs == nil {
				assert.Nil(t, uris)
			} else {
				assert.Equal(t, tt.wantURIs, uris)
			}
		})
	}
}

func TestExtractSameAsURIs_NilEntity(t *testing.T) {
	uris := ExtractSameAsURIs(nil)
	assert.Nil(t, uris)
}

func TestNewClient_CustomOptions(t *testing.T) {
	customHTTP := &http.Client{Timeout: 5 * time.Second}
	customUA := "TestClient/1.0"
	customRate := 2.0

	client := NewClient(
		"https://custom.endpoint",
		WithHTTPClient(customHTTP),
		WithUserAgent(customUA),
		WithRateLimit(customRate),
	)

	assert.Equal(t, "https://custom.endpoint", client.endpoint)
	assert.Equal(t, customUA, client.userAgent)
	assert.Equal(t, customHTTP, client.httpClient)
	assert.Equal(t, rate.Limit(customRate), client.limiter.Limit())
}

func TestReconcile_RealWorldResponse(t *testing.T) {
	// Test parsing of actual Artsdata API response structure
	serverResp := `{
		"q0": {
			"result": [
				{
					"id": "http://kg.artsdata.ca/resource/K11-211",
					"name": "Salle André-Mathieu",
					"score": 98.5,
					"match": true,
					"type": [
						{
							"id": "http://schema.org/Place",
							"name": "Place"
						}
					]
				},
				{
					"id": "http://kg.artsdata.ca/resource/K11-999",
					"name": "Similar Venue",
					"score": 85.0,
					"match": false,
					"type": [
						{
							"id": "http://schema.org/Place",
							"name": "Place"
						}
					]
				}
			]
		},
		"q1": {
			"result": []
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request is form-encoded per W3C Reconciliation API v0.2 §4.3
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		err := r.ParseForm()
		require.NoError(t, err)

		queriesParam := r.FormValue("queries")
		require.NotEmpty(t, queriesParam)

		var queries map[string]ReconciliationQuery
		err = json.Unmarshal([]byte(queriesParam), &queries)
		require.NoError(t, err)

		assert.Contains(t, queries, "q0")
		assert.Contains(t, queries, "q1")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(serverResp))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	queries := map[string]ReconciliationQuery{
		"q0": {
			Query: "Salle André-Mathieu",
			Type:  "schema:Place",
			Properties: []QueryProperty{
				{P: "schema:address/schema:addressLocality", V: "Laval"},
			},
		},
		"q1": {
			Query: "Nonexistent Venue",
			Type:  "schema:Place",
		},
	}

	results, err := client.Reconcile(ctx, queries)
	require.NoError(t, err)

	// Check q0 results
	require.Contains(t, results, "q0")
	assert.Len(t, results["q0"], 2)
	assert.Equal(t, "http://kg.artsdata.ca/resource/K11-211", results["q0"][0].ID)
	assert.Equal(t, "Salle André-Mathieu", results["q0"][0].Name)
	assert.Equal(t, 98.5, results["q0"][0].Score)
	assert.True(t, results["q0"][0].Match)
	assert.Len(t, results["q0"][0].Type, 1)
	assert.Equal(t, "http://schema.org/Place", results["q0"][0].Type[0].ID)

	// Check q1 results (empty)
	require.Contains(t, results, "q1")
	assert.Empty(t, results["q1"])
}

func TestDereference_ComplexJSONLD(t *testing.T) {
	// Test parsing of complex JSON-LD with multiple types and localized names
	serverResp := `{
		"@context": "https://schema.org",
		"@id": "http://kg.artsdata.ca/resource/K23-456",
		"@type": ["Organization", "PerformingGroup"],
		"name": {
			"@language": "en",
			"@value": "Canadian Opera Company"
		},
		"alternateName": [
			"COC",
			"Compagnie d'opéra canadienne"
		],
		"address": {
			"@type": "PostalAddress",
			"streetAddress": "227 Front St E",
			"addressLocality": "Toronto",
			"addressRegion": "ON",
			"postalCode": "M5A 1E8",
			"addressCountry": "CA"
		},
		"sameAs": {
			"@id": "http://www.wikidata.org/entity/Q2937014"
		},
		"url": "https://coc.ca",
		"description": "Canada's largest opera company"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/ld+json")
		_, _ = w.Write([]byte(serverResp))
	}))
	defer server.Close()

	client := NewClient(DefaultEndpoint)
	ctx := context.Background()

	entity, err := client.Dereference(ctx, server.URL+"/resource/K23-456")
	require.NoError(t, err)
	require.NotNil(t, entity)

	// Verify ID and Type (array variant)
	assert.Equal(t, "http://kg.artsdata.ca/resource/K23-456", entity.ID)
	typeArr, ok := entity.Type.([]interface{})
	require.True(t, ok)
	assert.Contains(t, typeArr, "Organization")
	assert.Contains(t, typeArr, "PerformingGroup")

	// Verify address
	require.NotNil(t, entity.Address)
	assert.Equal(t, "227 Front St E", entity.Address.StreetAddress)
	assert.Equal(t, "Toronto", entity.Address.AddressLocality)

	// Verify sameAs extraction (object with @id)
	uris := ExtractSameAsURIs(entity)
	require.Len(t, uris, 1)
	assert.Equal(t, "http://www.wikidata.org/entity/Q2937014", uris[0])

	// Verify raw JSON is stored
	assert.True(t, strings.Contains(string(entity.RawJSON), "Canadian Opera Company"))
}
