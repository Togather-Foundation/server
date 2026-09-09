package kg

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/kg/artsdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test replays the recorded Artsdata fixtures through the REAL
// ReconciliationService classification pipeline (client parsing + normalizeArtsdataScore
// + ClassifyConfidence + expandArtsdataID), asserting that the recorded response
// bytes classify into the expected buckets without touching the network or the
// shared testcontainers DB (it uses the in-package mockReconciliationCacheStore).

func artsdataFixturesDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "tests", "testdata", "artsdata")
}

func TestReconciliationServiceParity(t *testing.T) {
	fixtures, err := artsdata.LoadFixtures(artsdataFixturesDir())
	require.NoError(t, err)
	require.NotEmpty(t, fixtures, "no artsdata fixtures — run ARTSDATA_RECORD=1 go test ./internal/kg/artsdata/ -run TestArtsdataRecord")

	deref := findFixture(fixtures, "dereference")
	require.NotNil(t, deref, "dereference fixture required")

	for _, f := range fixtures {
		if f.Kind != artsdata.KindRecon {
			continue
		}
		f := f
		t.Run(f.Name, func(t *testing.T) {
			runClassificationReplay(t, f, deref)
		})
	}
}

func runClassificationReplay(t *testing.T, f *artsdata.Fixture, deref *artsdata.Fixture) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/recon" {
			gotBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", f.Response.ContentType)
			w.WriteHeader(f.Response.Status)
			_, _ = w.Write([]byte(f.Response.Body))
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/resource/") {
			w.Header().Set("Content-Type", deref.Response.ContentType)
			w.WriteHeader(deref.Response.Status)
			_, _ = w.Write([]byte(deref.Response.Body))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// The client is configured against the real endpoints; a transport rewrites the
	// artsdata hosts to the httptest server so replay can never reach the network.
	client := artsdata.NewClient(
		artsdata.DefaultEndpoint,
		artsdata.WithHTTPClient(&http.Client{Transport: hostRewriteTransport(t, server.URL)}),
		artsdata.WithRateLimit(1000.0),
	)

	mockCache := &mockReconciliationCacheStore{}
	svc := NewReconciliationService(client, mockCache, nil, 30*24*time.Hour, 7*24*time.Hour)

	q0, ok := f.Request.Queries["q0"]
	require.True(t, ok, "fixture %s missing q0 query", f.Name)
	entityType := "place"
	if q0.Type == "schema:Organization" {
		entityType = "organization"
	}

	matches, err := svc.ReconcileEntity(context.Background(), ReconcileRequest{
		EntityType: entityType,
		EntityID:   "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Name:       q0.Query,
	})
	require.NoError(t, err, "classification replay must succeed")

	// Wire-bytes assertion: the client's form-encoded body matches the recorded golden bytes.
	assert.Equal(t, f.Request.Body, string(gotBody), "client wire body drifted from recorded golden bytes")

	top := topResult(t, f)
	if top == nil {
		// Recorded response has no results: everything is rejected, nothing stored.
		assert.Empty(t, matches, "no-match fixture must produce no stored identifiers")
		return
	}

	if top.Match {
		require.NotEmpty(t, matches, "exact match must be classified")
		assert.Equal(t, "auto_high", matches[0].Method)
		assert.Equal(t, 0.99, matches[0].Confidence)
		// Short-ID expansion: the recorded id is a short ID (K\d+-\d+); the stored
		// IdentifierURI must be the expanded full https://kg.artsdata.ca/resource/ URI.
		wantURI := top.ID
		if !strings.HasPrefix(wantURI, "http") {
			wantURI = "https://kg.artsdata.ca/resource/" + wantURI
		}
		assert.Equal(t, wantURI, matches[0].IdentifierURI, "short ID must be expanded to the full Artsdata URI")
		// The exact match dereferences the recorded compacted fixture; the client
		// must parse it and extract the sameAs identifiers.
		require.NotEmpty(t, matches[0].SameAsURIs, "dereference must extract sameAs from the recorded fixture")
		assert.Contains(t, matches[0].SameAsURIs, "http://www.wikidata.org/entity/Q1122776")
	} else {
		// Partial/no-match: raw score ~3-12 -> normalizeArtsdataScore/15 < 0.8 -> reject.
		assert.Empty(t, matches, "partial matches must be rejected by classification")
	}
}

func topResult(t *testing.T, f *artsdata.Fixture) *artsdata.ReconciliationResult {
	t.Helper()
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(f.Response.Body), &raw))
	q0, ok := raw["q0"]
	require.True(t, ok, "fixture %s response missing q0", f.Name)
	var wrapper struct {
		Result []artsdata.ReconciliationResult `json:"result"`
	}
	require.NoError(t, json.Unmarshal(q0, &wrapper))
	if len(wrapper.Result) == 0 {
		return nil
	}
	return &wrapper.Result[0]
}

func findFixture(fixtures []*artsdata.Fixture, name string) *artsdata.Fixture {
	for _, f := range fixtures {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// hostRewriteTransport rewrites any outgoing request to the target httptest URL,
// guaranteeing replay never escapes to the live Artsdata network.
func hostRewriteTransport(t *testing.T, target string) http.RoundTripper {
	t.Helper()
	u, err := url.Parse(target)
	require.NoError(t, err)
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = u.Scheme
		req.URL.Host = u.Host
		req.Host = u.Host
		return http.DefaultTransport.RoundTrip(req)
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
