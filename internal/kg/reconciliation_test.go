package kg

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/kg/artsdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPlaceQuery(t *testing.T) {
	tests := []struct {
		name       string
		placeName  string
		city       string
		postalCode string
		wantProps  int
	}{
		{
			name:       "full address",
			placeName:  "Massey Hall",
			city:       "Toronto",
			postalCode: "M5B",
			wantProps:  2,
		},
		{
			name:       "city only",
			placeName:  "Arts Centre",
			city:       "Ottawa",
			postalCode: "",
			wantProps:  1,
		},
		{
			name:       "postal code only",
			placeName:  "Community Hall",
			city:       "",
			postalCode: "K1A",
			wantProps:  1,
		},
		{
			name:       "name only",
			placeName:  "Unknown Venue",
			city:       "",
			postalCode: "",
			wantProps:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := BuildPlaceQuery(tt.placeName, tt.city, tt.postalCode)

			assert.Equal(t, tt.placeName, query.Query)
			assert.Equal(t, "schema:Place", query.Type)
			assert.Len(t, query.Properties, tt.wantProps)

			if tt.city != "" {
				found := false
				for _, prop := range query.Properties {
					if prop.P == "schema:address/schema:addressLocality" && prop.V == tt.city {
						found = true
						break
					}
				}
				assert.True(t, found, "city property not found")
			}

			if tt.postalCode != "" {
				found := false
				for _, prop := range query.Properties {
					if prop.P == "schema:address/schema:postalCode" && prop.V == tt.postalCode {
						found = true
						break
					}
				}
				assert.True(t, found, "postal code property not found")
			}
		})
	}
}

func TestBuildOrgQuery(t *testing.T) {
	tests := []struct {
		name      string
		orgName   string
		url       string
		wantProps int
	}{
		{
			name:      "with URL",
			orgName:   "Canadian Opera Company",
			url:       "https://coc.ca",
			wantProps: 1,
		},
		{
			name:      "without URL",
			orgName:   "Local Theatre",
			url:       "",
			wantProps: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := BuildOrgQuery(tt.orgName, tt.url)

			assert.Equal(t, tt.orgName, query.Query)
			assert.Equal(t, "schema:Organization", query.Type)
			assert.Len(t, query.Properties, tt.wantProps)

			if tt.url != "" {
				found := false
				for _, prop := range query.Properties {
					if prop.P == "schema:url" && prop.V == tt.url {
						found = true
						break
					}
				}
				assert.True(t, found, "url property not found")
			}
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
			uri:      "http://kg.artsdata.ca/resource/K11-211",
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

// MockArtsdataClient implements a mock Artsdata client for testing.
type MockArtsdataClient struct {
	reconcileFunc   func(ctx context.Context, queries map[string]artsdata.ReconciliationQuery) (map[string][]artsdata.ReconciliationResult, error)
	dereferenceFunc func(ctx context.Context, uri string) (*artsdata.EntityData, error)
	extractFunc     func(data *artsdata.EntityData) []string
}

func (m *MockArtsdataClient) Reconcile(ctx context.Context, queries map[string]artsdata.ReconciliationQuery) (map[string][]artsdata.ReconciliationResult, error) {
	if m.reconcileFunc != nil {
		return m.reconcileFunc(ctx, queries)
	}
	return map[string][]artsdata.ReconciliationResult{}, nil
}

func (m *MockArtsdataClient) Dereference(ctx context.Context, uri string) (*artsdata.EntityData, error) {
	if m.dereferenceFunc != nil {
		return m.dereferenceFunc(ctx, uri)
	}
	return &artsdata.EntityData{}, nil
}

func (m *MockArtsdataClient) ExtractSameAsURIs(data *artsdata.EntityData) []string {
	if m.extractFunc != nil {
		return m.extractFunc(data)
	}
	return []string{}
}

func TestReconcileEntity_CacheHit(t *testing.T) {
	// This test would require database mocking, which is complex.
	// For now, we test the business logic functions in isolation.
	// Integration tests with a real database would be in a separate test suite.
	t.Skip("Integration test - requires database")
}

func TestReconcileEntity_HighConfidenceMatch(t *testing.T) {
	// This test would require database mocking.
	t.Skip("Integration test - requires database")
}

func TestReconcileEntity_NegativeCache(t *testing.T) {
	// This test would require database mocking.
	t.Skip("Integration test - requires database")
}

func TestMatchResult_JSON(t *testing.T) {
	// Test that MatchResult can be marshaled/unmarshaled for caching
	original := MatchResult{
		AuthorityCode: "artsdata",
		IdentifierURI: "http://kg.artsdata.ca/resource/K11-211",
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
			IdentifierURI: "http://kg.artsdata.ca/resource/K11-211",
			Confidence:    0.98,
			Method:        "auto_high",
			SameAsURIs:    []string{"http://www.wikidata.org/entity/Q1234567"},
		},
		{
			AuthorityCode: "artsdata",
			IdentifierURI: "http://kg.artsdata.ca/resource/K11-999",
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
		nil, // pool
		nil, // logger
		30*24*time.Hour,
		7*24*time.Hour,
	)

	require.NotNil(t, service)
	assert.Equal(t, 30*24*time.Hour, service.cacheTTL)
	assert.Equal(t, 7*24*time.Hour, service.failureTTL)
}
