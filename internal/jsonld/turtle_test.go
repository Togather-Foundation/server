package jsonld

import (
	"strings"
	"testing"
)

func TestSerializeToTurtle_Event(t *testing.T) {
	eventData := map[string]any{
		"@context":  "https://schema.org",
		"@id":       "https://example.org/events/01HW5ZQXJ9K3N2P4R6T8V0Y2Z4",
		"@type":     "Event",
		"name":      "Concert in the Park",
		"startDate": "2024-07-15T19:00:00Z",
		"location": map[string]any{
			"@type": "Place",
			"@id":   "https://example.org/places/01HW5ZQXJ9K3N2P4R6T8V0Y2Z5",
			"name":  "Central Park",
		},
		"organizer": map[string]any{
			"@type": "Organization",
			"@id":   "https://example.org/organizations/01HW5ZQXJ9K3N2P4R6T8V0Y2Z6",
			"name":  "City Arts Council",
		},
	}

	turtle, err := SerializeToTurtle(eventData)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Check for prefixes
	if !strings.Contains(turtle, "@prefix schema:") {
		t.Error("Missing @prefix schema:")
	}
	if !strings.Contains(turtle, "@prefix sel:") {
		t.Error("Missing @prefix sel:")
	}

	// Check for subject URI
	if !strings.Contains(turtle, "<https://example.org/events/01HW5ZQXJ9K3N2P4R6T8V0Y2Z4>") {
		t.Error("Missing subject URI")
	}

	// Check for type declaration
	if !strings.Contains(turtle, "a schema:Event") {
		t.Error("Missing type declaration")
	}

	// Check for properties
	if !strings.Contains(turtle, "schema:name \"Concert in the Park\"") {
		t.Error("Missing name property")
	}
	if !strings.Contains(turtle, "schema:startDate \"2024-07-15T19:00:00Z\"") {
		t.Error("Missing startDate property")
	}

	// Check for nested object references
	if !strings.Contains(turtle, "schema:location <https://example.org/places/01HW5ZQXJ9K3N2P4R6T8V0Y2Z5>") {
		t.Error("Missing location reference")
	}
	if !strings.Contains(turtle, "schema:organizer <https://example.org/organizations/01HW5ZQXJ9K3N2P4R6T8V0Y2Z6>") {
		t.Error("Missing organizer reference")
	}

	// Check for proper Turtle syntax (semicolons and final period)
	if !strings.Contains(turtle, " ;") {
		t.Error("Missing semicolons between properties")
	}
	if !strings.HasSuffix(strings.TrimSpace(turtle), ".") {
		t.Error("Missing final period")
	}
}

func TestSerializeToTurtle_Place(t *testing.T) {
	placeData := map[string]any{
		"@context": "https://schema.org",
		"@id":      "https://example.org/places/01HW5ZQXJ9K3N2P4R6T8V0Y2Z5",
		"@type":    "Place",
		"name":     "Central Park",
		"address": map[string]any{
			"@type":           "PostalAddress",
			"addressLocality": "New York",
			"addressRegion":   "NY",
			"addressCountry":  "US",
		},
	}

	turtle, err := SerializeToTurtle(placeData)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Check basics
	if !strings.Contains(turtle, "<https://example.org/places/01HW5ZQXJ9K3N2P4R6T8V0Y2Z5>") {
		t.Error("Missing subject URI")
	}
	if !strings.Contains(turtle, "a schema:Place") {
		t.Error("Missing type declaration")
	}
	if !strings.Contains(turtle, "schema:name \"Central Park\"") {
		t.Error("Missing name property")
	}
}

func TestSerializeToTurtle_Organization(t *testing.T) {
	orgData := map[string]any{
		"@context": "https://schema.org",
		"@id":      "https://example.org/organizations/01HW5ZQXJ9K3N2P4R6T8V0Y2Z6",
		"@type":    "Organization",
		"name":     "City Arts Council",
		"url":      "https://cityarts.example.org",
	}

	turtle, err := SerializeToTurtle(orgData)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Check basics
	if !strings.Contains(turtle, "<https://example.org/organizations/01HW5ZQXJ9K3N2P4R6T8V0Y2Z6>") {
		t.Error("Missing subject URI")
	}
	if !strings.Contains(turtle, "a schema:Organization") {
		t.Error("Missing type declaration")
	}
	if !strings.Contains(turtle, "schema:name \"City Arts Council\"") {
		t.Error("Missing name property")
	}
	// URL should be serialized as a URI, not a literal
	if !strings.Contains(turtle, "schema:url <https://cityarts.example.org>") {
		t.Error("URL should be serialized as URI")
	}
}

func TestSerializeToTurtle_MissingID(t *testing.T) {
	data := map[string]any{
		"@context": "https://schema.org",
		"@type":    "Event",
		"name":     "Test Event",
	}

	_, err := SerializeToTurtle(data)
	if err == nil {
		t.Error("Expected error for missing @id, got nil")
	}
	if !strings.Contains(err.Error(), "missing @id") {
		t.Errorf("Expected 'missing @id' error, got: %v", err)
	}
}

func TestSerializeToTurtle_NilInput(t *testing.T) {
	_, err := SerializeToTurtle(nil)
	if err == nil {
		t.Error("Expected error for nil input, got nil")
	}
}

func TestSerializeToTurtle_SpecialCharacters(t *testing.T) {
	data := map[string]any{
		"@context":    "https://schema.org",
		"@id":         "https://example.org/events/test",
		"@type":       "Event",
		"name":        "Event with \"quotes\" and\nnewlines\tand\ttabs",
		"description": "Special chars: \\ backslash",
	}

	turtle, err := SerializeToTurtle(data)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Check that special characters are escaped
	if !strings.Contains(turtle, "\\\"") {
		t.Error("Quotes should be escaped")
	}
	if !strings.Contains(turtle, "\\n") {
		t.Error("Newlines should be escaped")
	}
	if !strings.Contains(turtle, "\\t") {
		t.Error("Tabs should be escaped")
	}
	if !strings.Contains(turtle, "\\\\") {
		t.Error("Backslashes should be escaped")
	}
}

func TestSerializeToTurtle_NumericValues(t *testing.T) {
	data := map[string]any{
		"@context":      "https://schema.org",
		"@id":           "https://example.org/events/test",
		"@type":         "Event",
		"name":          "Test Event",
		"attendeeCount": float64(150),
		"price":         float64(25.50),
	}

	turtle, err := SerializeToTurtle(data)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Check that numeric values are not quoted
	if !strings.Contains(turtle, "schema:attendeeCount 150") {
		t.Error("Numeric value should not be quoted")
	}
	if !strings.Contains(turtle, "schema:price 25.5") {
		t.Error("Float value should not be quoted")
	}
}

func TestSerializeToTurtle_BooleanValues(t *testing.T) {
	data := map[string]any{
		"@context":            "https://schema.org",
		"@id":                 "https://example.org/events/test",
		"@type":               "Event",
		"name":                "Test Event",
		"isAccessibleForFree": true,
	}

	turtle, err := SerializeToTurtle(data)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Check that boolean values are not quoted
	if !strings.Contains(turtle, "schema:isAccessibleForFree true") {
		t.Error("Boolean value should not be quoted")
	}
}

func TestSerializeToTurtle_ArrayValues(t *testing.T) {
	data := map[string]any{
		"@context": "https://schema.org",
		"@id":      "https://example.org/events/test",
		"@type":    []interface{}{"Event", "SocialEvent"},
		"name":     "Test Event",
		"performer": []interface{}{
			"https://example.org/people/performer1",
			"https://example.org/people/performer2",
		},
	}

	turtle, err := SerializeToTurtle(data)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// For arrays, we currently take the first item
	if !strings.Contains(turtle, "a schema:Event") {
		t.Error("Should extract first type from array")
	}
	if !strings.Contains(turtle, "schema:performer") {
		t.Error("Should serialize array property")
	}
}

func TestEscapeLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`test "quote"`, `test \"quote\"`},
		{"line1\nline2", `line1\nline2`},
		{"tab\there", `tab\there`},
		{`back\slash`, `back\\slash`},
		{"carriage\rreturn", `carriage\rreturn`},
	}

	for _, tt := range tests {
		result := escapeLiteral(tt.input)
		if result != tt.expected {
			t.Errorf("escapeLiteral(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestSerializeToTurtle_EmptyValues(t *testing.T) {
	data := map[string]any{
		"@context":    "https://schema.org",
		"@id":         "https://example.org/events/test",
		"@type":       "Event",
		"name":        "",           // empty string
		"description": "Valid text", // non-empty for comparison
	}

	turtle, err := SerializeToTurtle(data)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Empty string should still be serialized as ""
	if !strings.Contains(turtle, `schema:name ""`) {
		t.Error("Empty string should be serialized as empty literal")
	}
}

func TestSerializeToTurtle_DateTimeCoercion(t *testing.T) {
	data := map[string]any{
		"@context":  "https://schema.org",
		"@id":       "https://example.org/events/test",
		"@type":     "Event",
		"name":      "Test Event",
		"startDate": "2024-07-15T19:00:00Z",
		"endDate":   "2024-07-15T21:00:00Z",
	}

	turtle, err := SerializeToTurtle(data)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Check that dateTime properties get xsd:dateTime type
	if !strings.Contains(turtle, `"2024-07-15T19:00:00Z"^^xsd:dateTime`) {
		t.Error("startDate should have xsd:dateTime type coercion")
	}
	if !strings.Contains(turtle, `"2024-07-15T21:00:00Z"^^xsd:dateTime`) {
		t.Error("endDate should have xsd:dateTime type coercion")
	}
}

func TestSerializeToTurtle_DateCoercion(t *testing.T) {
	data := map[string]any{
		"@context":     "https://schema.org",
		"@id":          "https://example.org/organizations/test",
		"@type":        "Organization",
		"name":         "Test Org",
		"foundingDate": "1995-03-20",
	}

	turtle, err := SerializeToTurtle(data)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Check that date properties get xsd:date type
	if !strings.Contains(turtle, `"1995-03-20"^^xsd:date`) {
		t.Error("foundingDate should have xsd:date type coercion")
	}
}

func TestSerializeToTurtle_EmptyArray(t *testing.T) {
	data := map[string]any{
		"@context":  "https://schema.org",
		"@id":       "https://example.org/events/test",
		"@type":     "Event",
		"name":      "Test Event",
		"performer": []interface{}{}, // empty array
	}

	turtle, err := SerializeToTurtle(data)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Empty array should be skipped (not cause errors)
	// The property should not appear in output
	performerCount := strings.Count(turtle, "schema:performer")
	if performerCount != 0 {
		t.Error("Empty array should not produce property triples")
	}
}

func TestSerializeToTurtle_NestedObjectWithoutID(t *testing.T) {
	data := map[string]any{
		"@context": "https://schema.org",
		"@id":      "https://example.org/events/test",
		"@type":    "Event",
		"name":     "Test Event",
		"location": map[string]any{
			// No @id - should fall back to JSON serialization
			"@type": "Place",
			"name":  "Unnamed Venue",
		},
	}

	turtle, err := SerializeToTurtle(data)
	if err != nil {
		t.Fatalf("SerializeToTurtle failed: %v", err)
	}

	// Should contain schema:location with a JSON-serialized value
	if !strings.Contains(turtle, "schema:location") {
		t.Error("Missing location property")
	}
	// Should be serialized as JSON string literal (not a URI reference)
	if strings.Contains(turtle, "schema:location <") {
		t.Error("Nested object without @id should not be serialized as URI reference")
	}
}
