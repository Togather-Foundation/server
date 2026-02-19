package kg

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// Note: MockArtsdataClient was removed because ReconciliationService takes a concrete
// *artsdata.Client, not an interface. For unit testing the service, consider introducing
// a reconciler interface (see follow-up bead). Integration tests use real HTTP mock servers.

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
		nil, // logger
		30*24*time.Hour,
		7*24*time.Hour,
	)

	require.NotNil(t, service)
	assert.Equal(t, 30*24*time.Hour, service.cacheTTL)
	assert.Equal(t, 7*24*time.Hour, service.failureTTL)
}
