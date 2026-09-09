package artsdata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file implements the Artsdata mock↔real parity harness.
//
//   - TestArtsdataReplayParity: default CI, no network. Serves the recorded
//     response bytes via httptest and runs the real client against them,
//     asserting wire-bytes (request body / Accept header) match the recorded
//     golden bytes and that parsing/score semantics hold.
//   - TestArtsdataRecord: opt-in (ARTSDATA_RECORD=1). Sends the fixed corpus to
//     the live https://api.artsdata.ca/recon and writes golden fixtures.
//   - TestArtsdataLiveCompare: opt-in (ARTSDATA_LIVE_COMPARE=1). Re-fetches the
//     corpus at <=1 rps and fails loudly on structural drift vs the fixtures.

var shortIDPattern = regexp.MustCompile(`^K\d+-\d+$`)

// dereferenceURI is a stable Artsdata resource URI (Massey Hall, the exact-match
// place in the corpus) captured with Accept: application/ld+json. The real
// endpoint answers with a 303 redirect to /entity.jsonld?uri=... which the
// recorder follows; the fixture stores the final 200 JSON-LD body.
const dereferenceURI = "https://kg.artsdata.ca/resource/K11-24"

// dereferenceOrgURI is a stable Artsdata resource URI (Canadian Opera Company,
// the exact-match organization in the recon-org-exact fixture). Organization
// dereferences return a different shape from places: a plain-string `type`,
// `en`-only language maps for name/description, a string-array sameAs, and no
// address/url — so a dedicated fixture exercises that path.
const dereferenceOrgURI = "https://kg.artsdata.ca/resource/K2-5143"

// corpus is the fixed, small recording corpus (<=4 recon queries + 2 dereferences).
// Names were chosen against the live API to exercise each score-semantic class:
//   - "Massey Hall" -> exact match (match:true, score ~880, short ID K11-24)
//   - "Art Gallery of Ontario" -> partial only (match:false, score ~3-5); note this
//     contradicts the hand-authored mock, which assumed an exact match here.
//   - "Definitely No Such Venue Xyzzy" -> the API still returns low-score partials
//     (match:false) rather than an empty result.
//   - "Canadian Opera Company" -> exact match (match:true, score ~1020).
type corpusEntry struct {
	name    string
	kind    FixtureKind
	queries map[string]ReconciliationQuery
	uri     string
}

func artsdataCorpus() []corpusEntry {
	return []corpusEntry{
		{
			name: "recon-place-exact",
			kind: KindRecon,
			queries: map[string]ReconciliationQuery{
				"q0": {Query: "Massey Hall", Type: "schema:Place"},
			},
		},
		{
			name: "recon-place-partial",
			kind: KindRecon,
			queries: map[string]ReconciliationQuery{
				"q0": {Query: "Art Gallery of Ontario", Type: "schema:Place"},
			},
		},
		{
			name: "recon-place-nomatch",
			kind: KindRecon,
			queries: map[string]ReconciliationQuery{
				"q0": {Query: "Definitely No Such Venue Xyzzy", Type: "schema:Place"},
			},
		},
		{
			name: "recon-org-exact",
			kind: KindRecon,
			queries: map[string]ReconciliationQuery{
				"q0": {Query: "Canadian Opera Company", Type: "schema:Organization"},
			},
		},
		{
			name: "dereference",
			kind: KindDereference,
			uri:  dereferenceURI,
		},
		{
			name: "dereference-org",
			kind: KindDereference,
			uri:  dereferenceOrgURI,
		},
	}
}

func fixturesDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "tests", "testdata", "artsdata")
}

// ---------------------------------------------------------------------------
// Replay (default CI, no network)
// ---------------------------------------------------------------------------

func TestArtsdataReplayParity(t *testing.T) {
	fixtures, err := LoadFixtures(fixturesDir())
	require.NoError(t, err)
	require.NotEmpty(t, fixtures, "no artsdata fixtures — run ARTSDATA_RECORD=1 go test ./internal/kg/artsdata/ -run TestArtsdataRecord")

	for _, f := range fixtures {
		f := f
		t.Run(f.Name, func(t *testing.T) {
			switch f.Kind {
			case KindRecon:
				runReconReplay(t, f)
			case KindDereference:
				runDereferenceReplay(t, f)
			default:
				t.Fatalf("unknown fixture kind %q", f.Kind)
			}
		})
	}
}

func runReconReplay(t *testing.T, f *Fixture) {
	var gotMethod, gotPath, gotContentType, gotAccept string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", f.Response.ContentType)
		w.WriteHeader(f.Response.Status)
		_, _ = w.Write([]byte(f.Response.Body))
	}))
	defer server.Close()

	client := NewClient(server.URL+f.Request.Path, WithRateLimit(1000.0))
	results, err := client.Reconcile(context.Background(), f.Request.Queries)
	require.NoError(t, err, "client must parse the recorded response")

	// Wire-bytes assertions: the client's outgoing request must match the recorded
	// golden bytes exactly (catches wire-format drift, tests/AGENTS.md wire-bytes rule).
	assert.Equal(t, f.Request.Method, gotMethod)
	assert.Equal(t, f.Request.Path, gotPath)
	assert.Equal(t, f.Request.ContentType, gotContentType)
	assert.Equal(t, f.Request.Accept, gotAccept)
	assert.Equal(t, f.Request.Body, string(gotBody), "client wire body drifted from recorded golden bytes")

	// Constraint: the client must never send `properties` (Artsdata returns HTTP 500).
	require.NoError(t, assertNoProperties(gotBody))

	// Score semantics are unbounded: exact matches ~1000+, partials ~3-12.
	for _, r := range results["q0"] {
		require.NotEmpty(t, r.ID)
		require.NotEmpty(t, r.Name)
		if r.Match {
			assert.Greater(t, r.Score, 100.0, "exact Artsdata matches are unbounded (~1000+), not 0-100")
		} else {
			assert.Less(t, r.Score, 20.0, "partial Artsdata matches score ~3-12")
		}
	}
}

func runDereferenceReplay(t *testing.T, f *Fixture) {
	var gotMethod, gotPath, gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", f.Response.ContentType)
		w.WriteHeader(f.Response.Status)
		_, _ = w.Write([]byte(f.Response.Body))
	}))
	defer server.Close()

	client := NewClient(server.URL, WithRateLimit(1000.0))
	entity, err := client.Dereference(context.Background(), server.URL+f.Request.Path)
	require.NoError(t, err, "client must parse the recorded dereference response")

	assert.Equal(t, f.Request.Method, gotMethod)
	assert.Equal(t, f.Request.Path, gotPath)
	assert.Equal(t, f.Request.Accept, gotAccept, "dereference Accept header drifted from recorded golden value")

	// The recorded response is compacted JSON-LD (id/type aliases, language maps,
	// @none-wrapped address). Assert the client actually populated the fields.
	// Places and organizations dereference into different shapes, so dispatch on
	// the fixture name and assert the shape specific to each entity kind.
	require.NotNil(t, entity)
	switch f.Name {
	case "dereference":
		assertPlaceDereference(t, entity)
	case "dereference-org":
		assertOrgDereference(t, entity)
	default:
		t.Fatalf("unknown dereference fixture %q", f.Name)
	}
	assert.NotEmpty(t, entity.RawJSON, "raw response must be preserved")
}

// assertPlaceDereference asserts the Massey Hall (Place/MusicVenue) shape: an
// array `type`, a full language-map name (preferring @none), and a @none-wrapped
// address block.
func assertPlaceDereference(t *testing.T, entity *EntityData) {
	t.Helper()
	assert.Equal(t, "http://kg.artsdata.ca/resource/K11-24", entity.ID)
	require.NotNil(t, entity.Address, "compacted address must decode")
	assert.Equal(t, "178 Victoria St", entity.Address.StreetAddress)
	assert.Equal(t, "Toronto", entity.Address.AddressLocality)
	assert.Equal(t, "CA", entity.Address.AddressCountry)
	assert.Equal(t, "Massey Hall", resolveString(entity.Name))
	assert.Len(t, ExtractSameAsURIs(entity), 7)
}

// assertOrgDereference asserts the Canadian Opera Company (Organization) shape:
// a plain-string `type`, an `en`-only language-map name/description (no @none),
// a string-array sameAs, and no address/url (Artsdata organizations do not carry
// a PostalAddress block).
func assertOrgDereference(t *testing.T, entity *EntityData) {
	t.Helper()
	assert.Equal(t, "http://kg.artsdata.ca/resource/K2-5143", entity.ID)

	typeStr, ok := entity.Type.(string)
	require.True(t, ok, "organization type should decode as a plain string, not an array")
	assert.Equal(t, "Organization", typeStr)

	assert.Equal(t, "Canadian Opera Company", resolveString(entity.Name), "en-only language map name must resolve")
	assert.Contains(t, resolveString(entity.Description), "Canadian Opera Company")

	uris := ExtractSameAsURIs(entity)
	assert.Len(t, uris, 4)
	assert.Contains(t, uris, "http://www.wikidata.org/entity/Q2915268")
	assert.Contains(t, uris, "https://isni.org/isni/0000000121842483")

	assert.Nil(t, entity.Address, "organizations carry no PostalAddress block in Artsdata")
	assert.Empty(t, resolveString(entity.URL), "organization dereference has no url")
}

// assertNoProperties decodes a form-encoded `queries=...` body and fails if any
// query carries a `properties` array (Artsdata /recon returns HTTP 500 for those).
func assertNoProperties(reqBody []byte) error {
	values, err := url.ParseQuery(string(reqBody))
	if err != nil {
		return fmt.Errorf("parse form body: %w", err)
	}
	q := values.Get("queries")
	if q == "" {
		return fmt.Errorf("body missing `queries` form parameter")
	}
	var queries map[string]ReconciliationQuery
	if err := json.Unmarshal([]byte(q), &queries); err != nil {
		return fmt.Errorf("decode queries: %w", err)
	}
	for id, query := range queries {
		if len(query.Properties) > 0 {
			return fmt.Errorf("query %q sends %d properties; Artsdata /recon returns HTTP 500 when any properties array is included", id, len(query.Properties))
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Recording (opt-in, ARTSDATA_RECORD=1)
// ---------------------------------------------------------------------------

func TestArtsdataRecord(t *testing.T) {
	if os.Getenv("ARTSDATA_RECORD") != "1" {
		t.Skip("set ARTSDATA_RECORD=1 to record the Artsdata corpus to tests/testdata/artsdata/")
	}
	dir := fixturesDir()
	require.NoError(t, os.MkdirAll(dir, 0o755))

	ctx := context.Background()
	for _, entry := range artsdataCorpus() {
		entry := entry
		t.Run(entry.name, func(t *testing.T) {
			f, err := recordEntry(ctx, entry)
			require.NoError(t, err)
			f.Provenance = ProvenanceLiveRecorded
			f.RecordedAt = time.Now().UTC().Format(time.RFC3339)
			require.NoError(t, SaveFixture(filepath.Join(dir, entry.name+".json"), f))
			t.Logf("recorded %s -> %s.json", entry.name, entry.name)
		})
		// Be polite to the public API: <=1 rps.
		time.Sleep(time.Second)
	}
}

func recordEntry(ctx context.Context, entry corpusEntry) (*Fixture, error) {
	switch entry.kind {
	case KindRecon:
		return recordRecon(ctx, entry)
	case KindDereference:
		return recordDereference(ctx, entry)
	default:
		return nil, fmt.Errorf("unknown kind %q", entry.kind)
	}
}

func recordRecon(ctx context.Context, entry corpusEntry) (*Fixture, error) {
	tr := &captureTransport{base: http.DefaultTransport}
	client := NewClient(DefaultEndpoint, WithHTTPClient(&http.Client{Timeout: DefaultTimeout, Transport: tr}))
	if _, err := client.Reconcile(ctx, entry.queries); err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	if tr.c == nil {
		return nil, fmt.Errorf("no exchange captured")
	}
	c := tr.c
	if c.status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", c.status)
	}
	if err := assertNoProperties(c.reqBody); err != nil {
		return nil, err
	}
	u, _ := url.Parse(c.url)
	return &Fixture{
		Name: entry.name,
		Kind: KindRecon,
		Request: FixtureRequest{
			Method:      c.method,
			URL:         c.url,
			Path:        u.Path,
			Accept:      c.reqHeader.Get("Accept"),
			ContentType: c.reqHeader.Get("Content-Type"),
			Body:        string(c.reqBody),
			Queries:     entry.queries,
		},
		Response: FixtureResponse{
			Status:      c.status,
			ContentType: c.respHeader.Get("Content-Type"),
			Body:        indentJSON(c.respBody),
		},
	}, nil
}

func recordDereference(ctx context.Context, entry corpusEntry) (*Fixture, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, entry.uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/ld+json")
	req.Header.Set("User-Agent", DefaultUserAgent)

	// The default client follows the 303 -> /entity.jsonld?uri=... redirect.
	hc := &http.Client{Timeout: DefaultTimeout}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dereference: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read dereference body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dereference returned status %d", resp.StatusCode)
	}
	u, _ := url.Parse(entry.uri)
	return &Fixture{
		Name: entry.name,
		Kind: KindDereference,
		Request: FixtureRequest{
			Method: http.MethodGet,
			URL:    entry.uri,
			Path:   u.Path,
			Accept: "application/ld+json",
		},
		Response: FixtureResponse{
			Status:      resp.StatusCode,
			ContentType: resp.Header.Get("Content-Type"),
			Body:        indentJSON(body),
		},
	}, nil
}

// indentJSON re-indents a JSON body without changing its token values, so score
// semantics stay human-readable in the committed fixture.
func indentJSON(raw []byte) json.RawMessage {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return json.RawMessage(append([]byte(nil), raw...))
	}
	return json.RawMessage(buf.Bytes())
}

// ---------------------------------------------------------------------------
// Live compare (opt-in, ARTSDATA_LIVE_COMPARE=1)
// ---------------------------------------------------------------------------

func TestArtsdataLiveCompare(t *testing.T) {
	if os.Getenv("ARTSDATA_LIVE_COMPARE") != "1" {
		t.Skip("set ARTSDATA_LIVE_COMPARE=1 to re-fetch the corpus and diff against the committed fixtures")
	}
	fixtures, err := LoadFixtures(fixturesDir())
	require.NoError(t, err)
	require.NotEmpty(t, fixtures)

	ctx := context.Background()
	for _, f := range fixtures {
		f := f
		t.Run(f.Name, func(t *testing.T) {
			live, err := fetchLive(ctx, f)
			require.NoError(t, err, "live fetch failed (endpoint moved / network unreachable)")
			compareLive(t, f, live)
		})
		time.Sleep(time.Second)
	}
}

func fetchLive(ctx context.Context, f *Fixture) (*capturedExchange, error) {
	switch f.Kind {
	case KindRecon:
		tr := &captureTransport{base: http.DefaultTransport}
		client := NewClient(DefaultEndpoint, WithHTTPClient(&http.Client{Timeout: DefaultTimeout, Transport: tr}))
		if _, err := client.Reconcile(ctx, f.Request.Queries); err != nil {
			return nil, err
		}
		if tr.c == nil {
			return nil, fmt.Errorf("no exchange captured")
		}
		return tr.c, nil
	case KindDereference:
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.Request.URL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/ld+json")
		req.Header.Set("User-Agent", DefaultUserAgent)
		hc := &http.Client{Timeout: DefaultTimeout}
		resp, err := hc.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return &capturedExchange{status: resp.StatusCode, respHeader: resp.Header.Clone(), respBody: body}, nil
	default:
		return nil, fmt.Errorf("unknown kind %q", f.Kind)
	}
}

func compareLive(t *testing.T, recorded *Fixture, live *capturedExchange) {
	if recorded.Response.Status != live.status {
		t.Fatalf("status drift: recorded %d, live %d (endpoint moved / non-200)", recorded.Response.Status, live.status)
	}
	switch recorded.Kind {
	case KindRecon:
		recSig, err := computeReconSignature(recorded.Response.Status, []byte(recorded.Response.Body))
		require.NoError(t, err)
		liveSig, err := computeReconSignature(live.status, live.respBody)
		require.NoError(t, err, "live response no longer matches the reconciliation shape")
		assert.Equal(t, recSig.HasResult, liveSig.HasResult, "result-presence drift")
		assert.Equal(t, recSig.TopMatch, liveSig.TopMatch, "match-flag semantic drift")
		assert.Equal(t, recSig.TopScoreBand, liveSig.TopScoreBand, "score-semantics drift")
		assert.Equal(t, recSig.TopIDKind, liveSig.TopIDKind, "id-shape drift (short id vs full URI)")
	case KindDereference:
		assert.Contains(t, live.respHeader.Get("Content-Type"), "ld+json", "dereference content-type drift")
		assert.NotEmpty(t, live.respBody, "dereference body is empty")
	}
}

type reconSignature struct {
	HasResult    bool
	TopMatch     bool
	TopScoreBand string
	TopIDKind    string
}

func computeReconSignature(status int, body []byte) (reconSignature, error) {
	sig := reconSignature{}
	if status != http.StatusOK {
		return sig, fmt.Errorf("status %d", status)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return sig, fmt.Errorf("response is not a JSON object: %w", err)
	}
	q0, ok := raw["q0"]
	if !ok {
		return sig, fmt.Errorf("response missing top-level q0 key")
	}
	var wrapper struct {
		Result []struct {
			ID    string  `json:"id"`
			Match bool    `json:"match"`
			Score float64 `json:"score"`
		} `json:"result"`
	}
	if err := json.Unmarshal(q0, &wrapper); err != nil {
		return sig, fmt.Errorf("result shape changed: %w", err)
	}
	sig.HasResult = len(wrapper.Result) > 0
	if sig.HasResult {
		top := wrapper.Result[0]
		sig.TopMatch = top.Match
		sig.TopScoreBand = classifyScoreBand(top.Score, top.Match)
		sig.TopIDKind = classifyIDKind(top.ID)
	}
	return sig, nil
}

func classifyScoreBand(score float64, match bool) string {
	if match {
		if score >= 100 {
			return "exact" // unbounded exact-match score (~1000+)
		}
		return "exact-low-score" // match=true but bounded score -> scale change
	}
	if score < 20 {
		return "partial" // partial-match score (~3-12)
	}
	return "partial-high-score"
}

func classifyIDKind(id string) string {
	switch {
	case id == "":
		return "none"
	case shortIDPattern.MatchString(id):
		return "short"
	default:
		return "uri"
	}
}

// ---------------------------------------------------------------------------
// Capture transport (records exact request bytes + raw response bytes)
// ---------------------------------------------------------------------------

type capturedExchange struct {
	method     string
	url        string
	reqHeader  http.Header
	reqBody    []byte
	status     int
	respHeader http.Header
	respBody   []byte
}

type captureTransport struct {
	base http.RoundTripper
	c    *capturedExchange
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c := &capturedExchange{
		method:    req.Method,
		url:       req.URL.String(),
		reqHeader: req.Header.Clone(),
	}
	if req.Body != nil {
		c.reqBody, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if req.GetBody != nil {
			if rc, err := req.GetBody(); err == nil {
				req.Body = rc
			} else {
				req.Body = io.NopCloser(bytes.NewReader(c.reqBody))
			}
		} else {
			req.Body = io.NopCloser(bytes.NewReader(c.reqBody))
		}
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	c.status = resp.StatusCode
	c.respHeader = resp.Header.Clone()
	c.respBody = body
	t.c = c
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}
