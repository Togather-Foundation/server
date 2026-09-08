package artsdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FixtureProvenance records how a golden fixture was produced. Fixtures recorded
// live from https://api.artsdata.ca/recon are "live-recorded"; fixtures authored
// from the documented response shapes (docs/interop/artsdata.md) without a live
// capture are "derived-from-docs-unverified" and should be refreshed later.
type FixtureProvenance string

const (
	// ProvenanceLiveRecorded marks a fixture captured from the real Artsdata API.
	ProvenanceLiveRecorded FixtureProvenance = "live-recorded"
	// ProvenanceDerivedFromDocs marks a fixture authored from documentation
	// without a live capture, pending refresh.
	ProvenanceDerivedFromDocs FixtureProvenance = "derived-from-docs-unverified"
)

// FixtureKind discriminates the two kinds of HTTP exchange the corpus records.
type FixtureKind string

const (
	// KindRecon is a POST to the W3C reconciliation endpoint (/recon).
	KindRecon FixtureKind = "recon"
	// KindDereference is a GET on a kg.artsdata.ca/resource/... URI.
	KindDereference FixtureKind = "dereference"
)

// Fixture is a single recorded HTTP exchange against the Artsdata API, storing
// enough to replay: the outgoing request (method, endpoint, exact body bytes,
// Accept/Content-Type headers, and the structured queries that produced them)
// and the raw response (status, content type, body bytes).
//
// The request Body is stored as a raw string (form-encoded `queries=...` for
// recon; empty for GET) so the replay test can assert the client's on-wire bytes
// exactly match the recorded bytes. The response Body is stored as json.RawMessage
// so score/match semantics stay human-readable in the committed fixture file.
type Fixture struct {
	Name       string            `json:"name"`
	Kind       FixtureKind       `json:"kind"`
	Provenance FixtureProvenance `json:"provenance"`
	RecordedAt string            `json:"recorded_at,omitempty"`
	Request    FixtureRequest    `json:"request"`
	Response   FixtureResponse   `json:"response"`
}

// FixtureRequest is the outgoing request captured for a fixture.
type FixtureRequest struct {
	Method      string                         `json:"method"`
	URL         string                         `json:"url"`
	Path        string                         `json:"path"`
	Accept      string                         `json:"accept,omitempty"`
	ContentType string                         `json:"content_type,omitempty"`
	Body        string                         `json:"body"`
	Queries     map[string]ReconciliationQuery `json:"queries,omitempty"`
}

// FixtureResponse is the raw response captured for a fixture.
type FixtureResponse struct {
	Status      int             `json:"status"`
	ContentType string          `json:"content_type,omitempty"`
	Body        json.RawMessage `json:"body"`
}

// LoadFixture reads and decodes a single fixture file.
func LoadFixture(path string) (*Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", path, err)
	}
	var f Fixture
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decode fixture %s: %w", path, err)
	}
	return &f, nil
}

// LoadFixtures reads every *.json fixture in dir, sorted by file name for
// deterministic ordering. Non-JSON files (e.g. README.md) are ignored.
func LoadFixtures(dir string) ([]*Fixture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read fixtures dir %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	fixtures := make([]*Fixture, 0, len(names))
	for _, name := range names {
		f, err := LoadFixture(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, f)
	}
	return fixtures, nil
}

// SaveFixture writes a fixture to path as indented JSON with a trailing newline.
func SaveFixture(path string, f *Fixture) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encode fixture: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write fixture %s: %w", path, err)
	}
	return nil
}
