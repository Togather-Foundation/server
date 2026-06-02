package apitypes

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type schemaMapping struct {
	goStruct     any
	schemaName   string
	endpointDesc string
}

func TestScraperTypesMatchOpenAPI(t *testing.T) {
	openAPIPath := filepath.Join("..", "..", "..", "docs", "api", "openapi.yaml")
	data, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}

	schemas, err := getSchemaMap(doc)
	if err != nil {
		t.Fatalf("failed to extract schemas: %v", err)
	}

	mappings := []schemaMapping{
		{goStruct: ScraperRunResponse{}, schemaName: "ScraperRunDetail", endpointDesc: "/api/v1/admin/scraper/diagnostics response item"},
		{goStruct: EventFailureResponse{}, schemaName: "EventFailure", endpointDesc: "/api/v1/admin/scraper/diagnostics response item (event_failures)"},
		{goStruct: DiagnosticsResponse{}, schemaName: "SourceDiagnostics", endpointDesc: "/api/v1/admin/scraper/sources/{name}/diagnostics"},
		{goStruct: AllDiagnosticsResponse{}, schemaName: "AllDiagnostics", endpointDesc: "/api/v1/admin/scraper/diagnostics"},
	}

	for _, m := range mappings {
		t.Run(m.schemaName, func(t *testing.T) {
			schema, ok := schemas[m.schemaName]
			if !ok {
				t.Fatalf("OpenAPI schema %q not found in components/schemas", m.schemaName)
			}

			props, ok := getSchemaProperties(schema)
			if !ok {
				t.Fatalf("schema %q has no properties", m.schemaName)
			}

			verifyStructFields(t, m.goStruct, props, m.endpointDesc)
		})
	}
}

func getSchemaMap(doc map[string]any) (map[string]any, error) {
	components, ok := doc["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("components not found in openapi.yaml")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schemas not found in components")
	}
	return schemas, nil
}

func getSchemaProperties(schema any) (map[string]any, bool) {
	s, ok := schema.(map[string]any)
	if !ok {
		return nil, false
	}
	props, ok := s["properties"].(map[string]any)
	return props, ok
}

func verifyStructFields(t *testing.T, goStruct any, oasProps map[string]any, endpointDesc string) {
	t.Helper()

	tp := reflect.TypeOf(goStruct)
	for i := 0; i < tp.NumField(); i++ {
		field := tp.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		name, _ := parseJSONTag(jsonTag)

		if _, ok := oasProps[name]; !ok {
			t.Errorf("Go field %s.%s (json: %q) not found in OpenAPI schema for endpoint %s",
				tp.Name(), field.Name, name, endpointDesc)
		}
	}
}

func parseJSONTag(tag string) (name string, omitempty bool) {
	if idx := strings.IndexByte(tag, ','); idx != -1 {
		name = tag[:idx]
		omitempty = tag[idx+1:] == "omitempty"
	} else {
		name = tag
	}
	return
}
