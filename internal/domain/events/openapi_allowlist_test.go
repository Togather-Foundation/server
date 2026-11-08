package events

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOpenAPISuppressReviewWarningsEnumMatchesAllowlist keeps the Go allowlist
// and the OpenAPI enums from drifting apart.
func TestOpenAPISuppressReviewWarningsEnumMatchesAllowlist(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "api", "openapi.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Items struct {
						Enum []string `yaml:"enum"`
					} `yaml:"items"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}

	want := PayloadSuppressibleReviewWarningCodes()
	if len(want) == 0 {
		t.Fatal("allowlist is empty")
	}

	for _, schema := range []string{"EventInput", "ScraperSourceSummary"} {
		s, ok := doc.Components.Schemas[schema]
		if !ok {
			t.Fatalf("schema %s not found in openapi.yaml", schema)
		}
		prop, ok := s.Properties["suppress_review_warnings"]
		if !ok {
			t.Fatalf("%s.suppress_review_warnings not found in openapi.yaml", schema)
		}
		if !reflect.DeepEqual(prop.Items.Enum, want) {
			t.Errorf("%s.suppress_review_warnings enum = %v, want %v", schema, prop.Items.Enum, want)
		}
	}
}
