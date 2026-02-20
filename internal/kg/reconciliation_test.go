package kg

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/kg/artsdata"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockArtsdataClient is a test double for ArtsdataClient.
type mockArtsdataClient struct {
	reconcileFunc    func(ctx context.Context, queries map[string]artsdata.ReconciliationQuery) (map[string][]artsdata.ReconciliationResult, error)
	dereferenceFunc  func(ctx context.Context, uri string) (*artsdata.EntityData, error)
	reconcileCalls   atomic.Int32
	dereferenceCalls atomic.Int32
}

func (m *mockArtsdataClient) Reconcile(ctx context.Context, queries map[string]artsdata.ReconciliationQuery) (map[string][]artsdata.ReconciliationResult, error) {
	m.reconcileCalls.Add(1)
	if m.reconcileFunc != nil {
		return m.reconcileFunc(ctx, queries)
	}
	return nil, nil
}

func (m *mockArtsdataClient) Dereference(ctx context.Context, uri string) (*artsdata.EntityData, error) {
	m.dereferenceCalls.Add(1)
	if m.dereferenceFunc != nil {
		return m.dereferenceFunc(ctx, uri)
	}
	return nil, nil
}

// mockReconciliationCacheStore is a test double for ReconciliationCacheStore.
type mockReconciliationCacheStore struct {
	getFunc               func(ctx context.Context, arg postgres.GetReconciliationCacheParams) (postgres.ReconciliationCache, error)
	upsertCacheFunc       func(ctx context.Context, arg postgres.UpsertReconciliationCacheParams) (postgres.ReconciliationCache, error)
	upsertIdentifierFunc  func(ctx context.Context, arg postgres.UpsertEntityIdentifierParams) (postgres.EntityIdentifier, error)
	getCalls              atomic.Int32
	upsertCacheCalls      atomic.Int32
	upsertIdentifierCalls atomic.Int32
}

func (m *mockReconciliationCacheStore) GetReconciliationCache(ctx context.Context, arg postgres.GetReconciliationCacheParams) (postgres.ReconciliationCache, error) {
	m.getCalls.Add(1)
	if m.getFunc != nil {
		return m.getFunc(ctx, arg)
	}
	return postgres.ReconciliationCache{}, pgx.ErrNoRows
}

func (m *mockReconciliationCacheStore) UpsertReconciliationCache(ctx context.Context, arg postgres.UpsertReconciliationCacheParams) (postgres.ReconciliationCache, error) {
	m.upsertCacheCalls.Add(1)
	if m.upsertCacheFunc != nil {
		return m.upsertCacheFunc(ctx, arg)
	}
	return postgres.ReconciliationCache{}, nil
}

func (m *mockReconciliationCacheStore) UpsertEntityIdentifier(ctx context.Context, arg postgres.UpsertEntityIdentifierParams) (postgres.EntityIdentifier, error) {
	m.upsertIdentifierCalls.Add(1)
	if m.upsertIdentifierFunc != nil {
		return m.upsertIdentifierFunc(ctx, arg)
	}
	return postgres.EntityIdentifier{}, nil
}

// TestExpandArtsdataID verifies that short IDs returned by the W3C Reconciliation
// API are expanded to full HTTPS URIs before being stored or dereferenced.
func TestExpandArtsdataID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "short ID is expanded",
			input: "K2-183",
			want:  "https://kg.artsdata.ca/resource/K2-183",
		},
		{
			name:  "short ID with larger numbers",
			input: "K11-211",
			want:  "https://kg.artsdata.ca/resource/K11-211",
		},
		{
			name:  "full https URI is unchanged",
			input: "https://kg.artsdata.ca/resource/K2-183",
			want:  "https://kg.artsdata.ca/resource/K2-183",
		},
		{
			name:  "full http URI is unchanged",
			input: "http://kg.artsdata.ca/resource/K11-211",
			want:  "http://kg.artsdata.ca/resource/K11-211",
		},
		{
			name:  "unrecognised string is unchanged",
			input: "some-other-id",
			want:  "some-other-id",
		},
		{
			name:  "empty string is unchanged",
			input: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := expandArtsdataID(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestReconcileEntities_ShortIDExpanded verifies that when the Artsdata API
// returns a short ID (e.g. "K2-183"), ReconcileEntity stores the full URI
// in MatchResult.IdentifierURI and passes the full URI to Dereference.
func TestReconcileEntities_ShortIDExpanded(t *testing.T) {
	t.Parallel()
	const shortID = "K2-183"
	const fullURI = "https://kg.artsdata.ca/resource/K2-183"

	var dereferencedURI string
	mockClient := &mockArtsdataClient{
		reconcileFunc: func(ctx context.Context, queries map[string]artsdata.ReconciliationQuery) (map[string][]artsdata.ReconciliationResult, error) {
			return map[string][]artsdata.ReconciliationResult{
				"q0": {
					{
						ID:    shortID, // API returns short form
						Name:  "Some Venue",
						Score: 1000.0,
						Match: true,
					},
				},
			}, nil
		},
		dereferenceFunc: func(ctx context.Context, uri string) (*artsdata.EntityData, error) {
			dereferencedURI = uri // capture what was passed
			return &artsdata.EntityData{ID: uri}, nil
		},
	}

	var upsertedIdentifierURI string
	mockCache := &mockReconciliationCacheStore{
		upsertIdentifierFunc: func(ctx context.Context, arg postgres.UpsertEntityIdentifierParams) (postgres.EntityIdentifier, error) {
			upsertedIdentifierURI = arg.IdentifierUri // capture DB write URI
			return postgres.EntityIdentifier{}, nil
		},
	}

	svc := NewReconciliationService(mockClient, mockCache, nil, 30*24*time.Hour, 7*24*time.Hour)
	results, err := svc.ReconcileEntity(context.Background(), ReconcileRequest{
		EntityType: "place",
		EntityID:   "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Name:       "Some Venue",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, fullURI, results[0].IdentifierURI, "IdentifierURI must be the expanded full URI, not the short ID")
	assert.Equal(t, fullURI, dereferencedURI, "Dereference must be called with the expanded full URI, not the short ID")
	assert.Equal(t, fullURI, upsertedIdentifierURI, "DB write (UpsertEntityIdentifier) must use the expanded full URI, not the short ID")
}

// TestReconcileEntities_WithMockClient tests ReconcileEntity end-to-end with mock client and cache store.
// Covers the high-confidence match + cache miss + dereference path.
func TestReconcileEntities_WithMockClient(t *testing.T) {
	const artsdataURI = "https://kg.artsdata.ca/resource/K11-211"

	mockClient := &mockArtsdataClient{
		reconcileFunc: func(ctx context.Context, queries map[string]artsdata.ReconciliationQuery) (map[string][]artsdata.ReconciliationResult, error) {
			return map[string][]artsdata.ReconciliationResult{
				"q0": {
					{
						ID:    artsdataURI,
						Name:  "Massey Hall",
						Score: 1247.4,
						Match: true, // normalizeArtsdataScore → 0.99, ClassifyConfidence → "auto_high"
					},
				},
			}, nil
		},
		dereferenceFunc: func(ctx context.Context, uri string) (*artsdata.EntityData, error) {
			return &artsdata.EntityData{
				ID: uri,
			}, nil
		},
	}

	mockCache := &mockReconciliationCacheStore{
		// Default getFunc returns pgx.ErrNoRows (cache miss) — already the default behaviour.
	}

	svc := NewReconciliationService(mockClient, mockCache, nil, 30*24*time.Hour, 7*24*time.Hour)
	require.NotNil(t, svc)

	ctx := context.Background()
	results, err := svc.ReconcileEntity(ctx, ReconcileRequest{
		EntityType: "place",
		EntityID:   "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Name:       "Massey Hall",
		Properties: map[string]string{"addressLocality": "Toronto"},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, artsdataURI, results[0].IdentifierURI)
	assert.Equal(t, "artsdata", results[0].AuthorityCode)
	assert.Equal(t, "auto_high", results[0].Method)
	assert.Equal(t, int32(1), mockClient.reconcileCalls.Load())
	assert.Equal(t, int32(1), mockClient.dereferenceCalls.Load())
	assert.Equal(t, int32(1), mockCache.getCalls.Load())
	assert.Equal(t, int32(1), mockCache.upsertCacheCalls.Load())
	assert.Equal(t, int32(1), mockCache.upsertIdentifierCalls.Load())
}

func TestBuildPlaceQuery(t *testing.T) {
	tests := []struct {
		name       string
		placeName  string
		city       string
		postalCode string
	}{
		{
			name:       "full address",
			placeName:  "Massey Hall",
			city:       "Toronto",
			postalCode: "M5B",
		},
		{
			name:       "city only",
			placeName:  "Arts Centre",
			city:       "Ottawa",
			postalCode: "",
		},
		{
			name:       "name only",
			placeName:  "Unknown Venue",
			city:       "",
			postalCode: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := BuildPlaceQuery(tt.placeName, tt.city, tt.postalCode)

			assert.Equal(t, tt.placeName, query.Query)
			assert.Equal(t, "schema:Place", query.Type)
			// Properties are intentionally omitted because Artsdata's API returns 500
			// when any properties are included. See BuildPlaceQuery doc comment.
			assert.Empty(t, query.Properties)
		})
	}
}

func TestBuildOrgQuery(t *testing.T) {
	tests := []struct {
		name    string
		orgName string
		url     string
	}{
		{
			name:    "with URL",
			orgName: "Canadian Opera Company",
			url:     "https://coc.ca",
		},
		{
			name:    "without URL",
			orgName: "Local Theatre",
			url:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := BuildOrgQuery(tt.orgName, tt.url)

			assert.Equal(t, tt.orgName, query.Query)
			assert.Equal(t, "schema:Organization", query.Type)
			// Properties are intentionally omitted because Artsdata's API returns 500
			// when any properties are included. See BuildOrgQuery doc comment.
			assert.Empty(t, query.Properties)
		})
	}
}

func TestNormalizeLookupKey(t *testing.T) {
	tests := []struct {
		name       string
		entityType string
		entityName string
		props      map[string]string
		wantKey    string
	}{
		{
			name:       "place with city and postal code",
			entityType: "place",
			entityName: "Massey Hall",
			props: map[string]string{
				"addressLocality": "Toronto",
				"postalCode":      "M5B",
			},
			wantKey: "place|massey hall|addresslocality=toronto|postalcode=m5b",
		},
		{
			name:       "organization with URL",
			entityType: "organization",
			entityName: "Canadian Opera Company",
			props: map[string]string{
				"url": "https://coc.ca",
			},
			wantKey: "organization|canadian opera company|url=https://coc.ca",
		},
		{
			name:       "no properties",
			entityType: "place",
			entityName: "Unknown Venue",
			props:      map[string]string{},
			wantKey:    "place|unknown venue",
		},
		{
			name:       "whitespace normalization",
			entityType: "  Place  ",
			entityName: "  Arts Centre  ",
			props: map[string]string{
				"addressLocality": "  Ottawa  ",
			},
			wantKey: "place|arts centre|addresslocality=ottawa",
		},
		{
			name:       "empty property values ignored",
			entityType: "place",
			entityName: "Test Venue",
			props: map[string]string{
				"addressLocality": "Toronto",
				"postalCode":      "",
				"url":             "",
			},
			wantKey: "place|test venue|addresslocality=toronto",
		},
		{
			name:       "properties sorted alphabetically",
			entityType: "place",
			entityName: "Venue",
			props: map[string]string{
				"z_prop": "z",
				"a_prop": "a",
				"m_prop": "m",
			},
			wantKey: "place|venue|a_prop=a|m_prop=m|z_prop=z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := NormalizeLookupKey(tt.entityType, tt.entityName, tt.props)
			assert.Equal(t, tt.wantKey, key)
		})
	}
}

func TestClassifyConfidence(t *testing.T) {
	tests := []struct {
		name       string
		score      float64
		match      bool
		wantMethod string
	}{
		{
			name:       "high confidence with match flag",
			score:      0.98,
			match:      true,
			wantMethod: "auto_high",
		},
		{
			name:       "95% with match flag (boundary)",
			score:      0.95,
			match:      true,
			wantMethod: "auto_high",
		},
		{
			name:       "high score without match flag",
			score:      0.98,
			match:      false,
			wantMethod: "auto_low",
		},
		{
			name:       "medium confidence",
			score:      0.85,
			match:      false,
			wantMethod: "auto_low",
		},
		{
			name:       "80% confidence (boundary)",
			score:      0.80,
			match:      false,
			wantMethod: "auto_low",
		},
		{
			name:       "low confidence",
			score:      0.75,
			match:      false,
			wantMethod: "reject",
		},
		{
			name:       "very low confidence",
			score:      0.40,
			match:      false,
			wantMethod: "reject",
		},
		{
			name:       "below 80% with match flag",
			score:      0.79,
			match:      true,
			wantMethod: "reject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := ClassifyConfidence(tt.score, tt.match)
			assert.Equal(t, tt.wantMethod, method)
		})
	}
}

func TestNormalizeArtsdataScore(t *testing.T) {
	tests := []struct {
		name           string
		score          float64
		match          bool
		wantConfidence float64
	}{
		{
			name:           "exact match returns 0.99",
			score:          1247.4,
			match:          true,
			wantConfidence: 0.99,
		},
		{
			name:           "exact match with low score still 0.99",
			score:          5.0,
			match:          true,
			wantConfidence: 0.99,
		},
		{
			name:           "partial match normalized",
			score:          6.0,
			match:          false,
			wantConfidence: 0.4,
		},
		{
			name:           "high partial match capped at 0.95",
			score:          20.0,
			match:          false,
			wantConfidence: 0.95,
		},
		{
			name:           "low partial match",
			score:          3.0,
			match:          false,
			wantConfidence: 0.2,
		},
		{
			name:           "zero score",
			score:          0.0,
			match:          false,
			wantConfidence: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := normalizeArtsdataScore(tt.score, tt.match)
			assert.InDelta(t, tt.wantConfidence, confidence, 0.01)
		})
	}
}

func TestInferAuthorityCode(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		wantCode string
	}{
		{
			name:     "Wikidata",
			uri:      "http://www.wikidata.org/entity/Q1234567",
			wantCode: "wikidata",
		},
		{
			name:     "MusicBrainz",
			uri:      "https://musicbrainz.org/artist/abc-123",
			wantCode: "musicbrainz",
		},
		{
			name:     "ISNI",
			uri:      "https://isni.org/isni/0000000123456789",
			wantCode: "isni",
		},
		{
			name:     "OpenStreetMap",
			uri:      "https://www.openstreetmap.org/way/12345",
			wantCode: "osm",
		},
		{
			name:     "Artsdata",
			uri:      "https://kg.artsdata.ca/resource/K11-211",
			wantCode: "artsdata",
		},
		{
			name:     "unknown authority",
			uri:      "https://example.com/entity/123",
			wantCode: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := inferAuthorityCode(tt.uri)
			assert.Equal(t, tt.wantCode, code)
		})
	}
}

func TestReconcileEntity_CacheHit(t *testing.T) {
	const artsdataURI = "https://kg.artsdata.ca/resource/K11-211"

	// Pre-build the JSON that the service will unmarshal from the cache row.
	cachedMatches := []MatchResult{
		{
			AuthorityCode: "artsdata",
			IdentifierURI: artsdataURI,
			Confidence:    0.99,
			Method:        "auto_high",
			SameAsURIs:    []string{},
		},
	}
	resultJSON, err := json.Marshal(cachedMatches)
	require.NoError(t, err)

	mockCache := &mockReconciliationCacheStore{
		getFunc: func(ctx context.Context, arg postgres.GetReconciliationCacheParams) (postgres.ReconciliationCache, error) {
			// Return a positive cache hit
			return postgres.ReconciliationCache{
				EntityType:    arg.EntityType,
				AuthorityCode: arg.AuthorityCode,
				LookupKey:     arg.LookupKey,
				ResultJson:    resultJSON,
				IsNegative:    false,
			}, nil
		},
	}
	mockClient := &mockArtsdataClient{}

	svc := NewReconciliationService(mockClient, mockCache, nil, 30*24*time.Hour, 7*24*time.Hour)

	ctx := context.Background()
	results, err := svc.ReconcileEntity(ctx, ReconcileRequest{
		EntityType: "place",
		EntityID:   "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Name:       "Massey Hall",
		Properties: map[string]string{"addressLocality": "Toronto"},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, artsdataURI, results[0].IdentifierURI)
	// Client should never be called on a cache hit
	assert.Equal(t, int32(0), mockClient.reconcileCalls.Load())
	assert.Equal(t, int32(1), mockCache.getCalls.Load())
}

func TestReconcileEntity_NegativeCache(t *testing.T) {
	// A negative cache entry means a previous lookup found no match.
	// The service stores is_negative=true and an empty JSON array.
	// On a cache hit with is_negative=true, the service returns an empty slice
	// without calling the Artsdata client.
	emptyJSON, err := json.Marshal([]MatchResult{})
	require.NoError(t, err)

	mockCache := &mockReconciliationCacheStore{
		getFunc: func(ctx context.Context, arg postgres.GetReconciliationCacheParams) (postgres.ReconciliationCache, error) {
			return postgres.ReconciliationCache{
				EntityType:    arg.EntityType,
				AuthorityCode: arg.AuthorityCode,
				LookupKey:     arg.LookupKey,
				ResultJson:    emptyJSON,
				IsNegative:    true,
			}, nil
		},
	}
	mockClient := &mockArtsdataClient{}

	svc := NewReconciliationService(mockClient, mockCache, nil, 30*24*time.Hour, 7*24*time.Hour)

	ctx := context.Background()
	results, err := svc.ReconcileEntity(ctx, ReconcileRequest{
		EntityType: "place",
		EntityID:   "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Name:       "Unknown Venue",
		Properties: map[string]string{},
	})
	require.NoError(t, err)
	assert.Empty(t, results)
	// Client must NOT be called when there is a negative cache entry
	assert.Equal(t, int32(0), mockClient.reconcileCalls.Load())
	assert.Equal(t, int32(1), mockCache.getCalls.Load())
}

func TestMatchResult_JSON(t *testing.T) {
	// Test that MatchResult can be marshaled/unmarshaled for caching
	original := MatchResult{
		AuthorityCode: "artsdata",
		IdentifierURI: "https://kg.artsdata.ca/resource/K11-211",
		Confidence:    0.98,
		Method:        "auto_high",
		SameAsURIs: []string{
			"http://www.wikidata.org/entity/Q1234567",
			"https://www.openstreetmap.org/way/12345",
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded MatchResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.AuthorityCode, decoded.AuthorityCode)
	assert.Equal(t, original.IdentifierURI, decoded.IdentifierURI)
	assert.Equal(t, original.Confidence, decoded.Confidence)
	assert.Equal(t, original.Method, decoded.Method)
	assert.Equal(t, original.SameAsURIs, decoded.SameAsURIs)
}

func TestMatchResult_JSONArray(t *testing.T) {
	// Test that []MatchResult can be marshaled/unmarshaled for caching
	original := []MatchResult{
		{
			AuthorityCode: "artsdata",
			IdentifierURI: "https://kg.artsdata.ca/resource/K11-211",
			Confidence:    0.98,
			Method:        "auto_high",
			SameAsURIs:    []string{"http://www.wikidata.org/entity/Q1234567"},
		},
		{
			AuthorityCode: "artsdata",
			IdentifierURI: "https://kg.artsdata.ca/resource/K11-999",
			Confidence:    0.85,
			Method:        "auto_low",
			SameAsURIs:    []string{},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded []MatchResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Len(t, decoded, 2)
	assert.Equal(t, original[0].IdentifierURI, decoded[0].IdentifierURI)
	assert.Equal(t, original[1].IdentifierURI, decoded[1].IdentifierURI)
}

func TestNewReconciliationService(t *testing.T) {
	// Test service creation
	// Note: In real usage, would use artsdata.NewClient()
	// This test just verifies the service constructor
	service := NewReconciliationService(
		nil, // artsdataClient (would be real client in production)
		nil, // queries
		nil, // logger
		30*24*time.Hour,
		7*24*time.Hour,
	)

	require.NotNil(t, service)
	assert.Equal(t, 30*24*time.Hour, service.cacheTTL)
	assert.Equal(t, 7*24*time.Hour, service.failureTTL)
}
