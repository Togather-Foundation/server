package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderEventHTML(t *testing.T) {
	eventData := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Event",
		"@id":         "https://example.com/events/123",
		"name":        "Jazz in the Park",
		"description": "A wonderful jazz concert",
		"startDate":   "2026-07-10T19:00:00Z",
		"location": map[string]any{
			"@type":   "Place",
			"name":    "Centennial Park",
			"address": "Toronto, ON",
		},
		"organizer": map[string]any{
			"@type": "Organization",
			"name":  "Toronto Arts Org",
		},
	}

	html, err := RenderEventHTML(eventData)
	if err != nil {
		t.Fatalf("RenderEventHTML failed: %v", err)
	}

	// Verify HTML structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Missing DOCTYPE")
	}
	if !strings.Contains(html, "<html lang=\"en\">") {
		t.Error("Missing html tag with lang")
	}
	if !strings.Contains(html, "<head>") {
		t.Error("Missing head tag")
	}
	if !strings.Contains(html, "<body>") {
		t.Error("Missing body tag")
	}

	// Verify embedded JSON-LD
	if !strings.Contains(html, `<script type="application/ld+json">`) {
		t.Error("Missing JSON-LD script tag")
	}

	// Verify event name appears
	if !strings.Contains(html, "Jazz in the Park") {
		t.Error("Event name not in HTML")
	}

	// Verify JSON-LD is valid
	start := strings.Index(html, `<script type="application/ld+json">`)
	start += len(`<script type="application/ld+json">`)
	end := strings.Index(html[start:], "</script>")
	jsonldContent := strings.TrimSpace(html[start : start+end])

	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonldContent), &parsed); err != nil {
		t.Errorf("Embedded JSON-LD is invalid: %v", err)
	}

	if parsed["name"] != "Jazz in the Park" {
		t.Errorf("JSON-LD name mismatch: got %v", parsed["name"])
	}
}

func TestRenderPlaceHTML(t *testing.T) {
	placeData := map[string]any{
		"@context":        "https://schema.org",
		"@type":           "Place",
		"@id":             "https://example.com/places/456",
		"name":            "Centennial Park",
		"addressLocality": "Toronto",
		"addressRegion":   "ON",
	}

	html, err := RenderPlaceHTML(placeData)
	if err != nil {
		t.Fatalf("RenderPlaceHTML failed: %v", err)
	}

	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Missing DOCTYPE")
	}
	if !strings.Contains(html, "Centennial Park") {
		t.Error("Place name not in HTML")
	}
	if !strings.Contains(html, `<script type="application/ld+json">`) {
		t.Error("Missing JSON-LD script tag")
	}
}

func TestRenderOrganizationHTML(t *testing.T) {
	orgData := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Organization",
		"@id":         "https://example.com/organizations/789",
		"name":        "Toronto Arts Org",
		"description": "A community arts organization",
		"url":         "https://torontoarts.example.com",
	}

	html, err := RenderOrganizationHTML(orgData)
	if err != nil {
		t.Fatalf("RenderOrganizationHTML failed: %v", err)
	}

	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Missing DOCTYPE")
	}
	if !strings.Contains(html, "Toronto Arts Org") {
		t.Error("Organization name not in HTML")
	}
	if !strings.Contains(html, `<script type="application/ld+json">`) {
		t.Error("Missing JSON-LD script tag")
	}
}

func TestHTMLEscaping(t *testing.T) {
	eventData := map[string]any{
		"@context": "https://schema.org",
		"@type":    "Event",
		"name":     `Jazz "in the" Park <script>alert('xss')</script>`,
	}

	html, err := RenderEventHTML(eventData)
	if err != nil {
		t.Fatalf("RenderEventHTML failed: %v", err)
	}

	// Verify HTML escaping (should not contain unescaped < or >)
	if strings.Contains(html, "<script>alert") && !strings.Contains(html, `&lt;script&gt;`) {
		t.Error("HTML not properly escaped in body")
	}
}
