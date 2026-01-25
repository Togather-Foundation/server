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

func TestSHACLValidation(t *testing.T) {
	pyshaclPath, err := exec.LookPath("pyshacl")
	if err != nil {
		t.Skip("pyshacl not installed; skipping SHACL validation")
	}

	root := repoRoot(t)
	dataFile := filepath.Join(t.TempDir(), "sample.ttl")
	if err := os.WriteFile(dataFile, []byte(shaclSampleTTL), 0o644); err != nil {
		t.Fatalf("write sample ttl: %v", err)
	}

	shapeFiles := []string{
		filepath.Join(root, "shapes", "event-v0.1.ttl"),
		filepath.Join(root, "shapes", "place-v0.1.ttl"),
		filepath.Join(root, "shapes", "organization-v0.1.ttl"),
	}

	args := []string{}
	for _, shape := range shapeFiles {
		args = append(args, "-s", shape)
	}
	args = append(args, dataFile)

	cmd := exec.Command(pyshaclPath, args...)
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("shacl validation failed: %v\n%s", err, out.String())
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
