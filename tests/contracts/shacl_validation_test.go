package contracts_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const shaclSampleTTL = `@prefix schema: <https://schema.org/> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .

<https://example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF> a schema:Event ;
  schema:name "Jazz Night" ;
  schema:startDate "2026-01-01T19:00:00Z"^^xsd:dateTime ;
  schema:location <https://example.org/places/01HYX3KQW7ERTV9XNBM2P8QJZG> .

<https://example.org/places/01HYX3KQW7ERTV9XNBM2P8QJZG> a schema:Place ;
  schema:name "Massey Hall" ;
  schema:address [ a schema:PostalAddress ; schema:streetAddress "178 Victoria St" ; schema:addressLocality "Toronto" ] .

<https://example.org/orgs/01HYX3KQW7ERTV9XNBM2P8QJZH> a schema:Organization ;
  schema:name "Togather Foundation" .
`

const shaclAllDaySampleTTL = `@prefix schema: <https://schema.org/> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix sel: <https://schema.togather.foundation/ns#> .

<https://example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF> a schema:Event ;
  schema:name "All Day Exhibition" ;
  schema:startDate "2026-11-01"^^xsd:date ;
  sel:allDay true ;
  schema:location <https://example.org/places/01HYX3KQW7ERTV9XNBM2P8QJZG> .

<https://example.org/places/01HYX3KQW7ERTV9XNBM2P8QJZG> a schema:Place ;
  schema:name "Gallery" ;
  schema:address [ a schema:PostalAddress ; schema:streetAddress "1 Queen St" ; schema:addressLocality "Toronto" ] .
`

const shaclDateOnlyWithoutAllDayTTL = `@prefix schema: <https://schema.org/> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix sel: <https://schema.togather.foundation/ns#> .

<https://example.org/events/01HYX3KQW7ERTV9XNBM2P8QJZF> a schema:Event ;
  schema:name "Missing AllDay Flag" ;
  schema:startDate "2026-11-01"^^xsd:date ;
  schema:location <https://example.org/places/01HYX3KQW7ERTV9XNBM2P8QJZG> .

<https://example.org/places/01HYX3KQW7ERTV9XNBM2P8QJZG> a schema:Place ;
  schema:name "Gallery" ;
  schema:address [ a schema:PostalAddress ; schema:streetAddress "1 Queen St" ; schema:addressLocality "Toronto" ] .
`

func TestSHACLValidation(t *testing.T) {
	runSHACLValidation(t, shaclSampleTTL, true)
}

// TestSHACLAllDayEventPasses verifies a date-only startDate with sel:allDay true
// conforms to the shape (date-only is permitted for all-day events).
func TestSHACLAllDayEventPasses(t *testing.T) {
	runSHACLValidation(t, shaclAllDaySampleTTL, true)
}

// TestSHACLDateOnlyWithoutAllDayFails verifies a date-only startDate without
// sel:allDay true is rejected by the gate.
func TestSHACLDateOnlyWithoutAllDayFails(t *testing.T) {
	runSHACLValidation(t, shaclDateOnlyWithoutAllDayTTL, false)
}

func runSHACLValidation(t *testing.T, data string, wantConform bool) {
	t.Helper()

	pyshaclPath, err := exec.LookPath("pyshacl")
	if err != nil {
		t.Skip("pyshacl not installed; skipping SHACL validation")
	}

	root := repoRoot(t)
	dataFile := filepath.Join(t.TempDir(), "sample.ttl")
	if err := os.WriteFile(dataFile, []byte(data), 0o644); err != nil {
		t.Fatalf("write sample ttl: %v", err)
	}

	// The three shape files define distinct sh:NodeShape instances with distinct
	// prefixes, so they are concatenated into a single shapes graph. pyshacl's
	// CLI only honours a single `-s` shapes file — passing multiple `-s` flags
	// silently drops all but the last, making validation vacuous.
	shapeFiles := []string{
		filepath.Join(root, "shapes", "event-v0.1.ttl"),
		filepath.Join(root, "shapes", "place-v0.1.ttl"),
		filepath.Join(root, "shapes", "organization-v0.1.ttl"),
	}
	var combined bytes.Buffer
	for _, shape := range shapeFiles {
		b, err := os.ReadFile(shape)
		if err != nil {
			t.Fatalf("read shape %s: %v", shape, err)
		}
		combined.Write(b)
		combined.WriteString("\n")
	}
	shapeFile := filepath.Join(t.TempDir(), "shapes.ttl")
	if err := os.WriteFile(shapeFile, combined.Bytes(), 0o644); err != nil {
		t.Fatalf("write combined shapes ttl: %v", err)
	}

	args := []string{"-s", shapeFile, dataFile}

	cmd := exec.Command(pyshaclPath, args...)
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		if wantConform {
			t.Fatalf("shacl validation failed (expected conform): %v\n%s", err, out.String())
		}
		// Non-conformance is the expected outcome.
		return
	}
	if !wantConform {
		t.Fatalf("shacl validation unexpectedly conformed\n%s", out.String())
	}
}

func repoRoot(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("repo root not found")
	return ""
}
