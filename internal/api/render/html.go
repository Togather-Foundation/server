package render

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"
)

// RenderEventHTML renders an event as HTML with embedded JSON-LD
func RenderEventHTML(eventData map[string]any) (string, error) {
	name := extractString(eventData, "name")
	description := extractString(eventData, "description")
	startDate := extractString(eventData, "startDate")
	location := extractLocation(eventData)
	organizer := extractOrganizer(eventData)

	jsonldBytes, err := json.MarshalIndent(eventData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON-LD: %w", err)
	}

	return buildHTML(name, "Event", buildEventBody(name, description, startDate, location, organizer), string(jsonldBytes)), nil
}

// RenderPlaceHTML renders a place as HTML with embedded JSON-LD
func RenderPlaceHTML(placeData map[string]any) (string, error) {
	name := extractString(placeData, "name")
	address := extractAddress(placeData)

	jsonldBytes, err := json.MarshalIndent(placeData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON-LD: %w", err)
	}

	return buildHTML(name, "Place", buildPlaceBody(name, address), string(jsonldBytes)), nil
}

// RenderOrganizationHTML renders an organization as HTML with embedded JSON-LD
func RenderOrganizationHTML(orgData map[string]any) (string, error) {
	name := extractString(orgData, "name")
	description := extractString(orgData, "description")
	url := extractString(orgData, "url")

	jsonldBytes, err := json.MarshalIndent(orgData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON-LD: %w", err)
	}

	return buildHTML(name, "Organization", buildOrganizationBody(name, description, url), string(jsonldBytes)), nil
}

// buildHTML creates the full HTML document structure
func buildHTML(title, entityType, bodyContent, jsonld string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s - Shared Events Library</title>
  <style>
    body { font-family: system-ui, -apple-system, sans-serif; max-width: 800px; margin: 2rem auto; padding: 0 1rem; line-height: 1.6; }
    h1 { color: #333; margin-bottom: 0.5rem; }
    .type { color: #666; font-size: 0.9rem; text-transform: uppercase; letter-spacing: 0.05em; }
    .content { margin-top: 2rem; }
    .field { margin-bottom: 1rem; }
    .label { font-weight: 600; color: #555; }
    .value { color: #333; }
    footer { margin-top: 3rem; padding-top: 2rem; border-top: 1px solid #ddd; color: #666; font-size: 0.9rem; }
  </style>
  <script type="application/ld+json">
%s
  </script>
</head>
<body>
  <div class="type">%s</div>
  <h1>%s</h1>
  <div class="content">
%s
  </div>
  <footer>
    <p>This page is part of the <a href="https://togather.foundation">Shared Events Library</a>.</p>
  </footer>
</body>
</html>`, html.EscapeString(title), jsonld, html.EscapeString(entityType), html.EscapeString(title), bodyContent)
}

// buildEventBody creates the body content for an event
func buildEventBody(name, description, startDate, location, organizer string) string {
	var parts []string

	if description != "" {
		parts = append(parts, fmt.Sprintf(`    <div class="field">
      <div class="label">Description</div>
      <div class="value">%s</div>
    </div>`, html.EscapeString(description)))
	}

	if startDate != "" {
		formatted := formatDateTime(startDate)
		parts = append(parts, fmt.Sprintf(`    <div class="field">
      <div class="label">Start Date</div>
      <div class="value">%s</div>
    </div>`, html.EscapeString(formatted)))
	}

	if location != "" {
		parts = append(parts, fmt.Sprintf(`    <div class="field">
      <div class="label">Location</div>
      <div class="value">%s</div>
    </div>`, html.EscapeString(location)))
	}

	if organizer != "" {
		parts = append(parts, fmt.Sprintf(`    <div class="field">
      <div class="label">Organizer</div>
      <div class="value">%s</div>
    </div>`, html.EscapeString(organizer)))
	}

	return strings.Join(parts, "\n")
}

// buildPlaceBody creates the body content for a place
func buildPlaceBody(name, address string) string {
	var parts []string

	if address != "" {
		parts = append(parts, fmt.Sprintf(`    <div class="field">
      <div class="label">Address</div>
      <div class="value">%s</div>
    </div>`, html.EscapeString(address)))
	}

	return strings.Join(parts, "\n")
}

// buildOrganizationBody creates the body content for an organization
func buildOrganizationBody(name, description, url string) string {
	var parts []string

	if description != "" {
		parts = append(parts, fmt.Sprintf(`    <div class="field">
      <div class="label">Description</div>
      <div class="value">%s</div>
    </div>`, html.EscapeString(description)))
	}

	if url != "" {
		parts = append(parts, fmt.Sprintf(`    <div class="field">
      <div class="label">Website</div>
      <div class="value"><a href="%s">%s</a></div>
    </div>`, html.EscapeString(url), html.EscapeString(url)))
	}

	return strings.Join(parts, "\n")
}

// extractString safely extracts a string value from a map
func extractString(data map[string]any, key string) string {
	if value, ok := data[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

// extractLocation extracts location information from event data
func extractLocation(data map[string]any) string {
	if location, ok := data["location"].(map[string]any); ok {
		name := extractString(location, "name")
		address := extractString(location, "address")
		if name != "" && address != "" {
			return fmt.Sprintf("%s (%s)", name, address)
		}
		if name != "" {
			return name
		}
		if address != "" {
			return address
		}
	}
	return ""
}

// extractOrganizer extracts organizer information from event data
func extractOrganizer(data map[string]any) string {
	if organizer, ok := data["organizer"].(map[string]any); ok {
		return extractString(organizer, "name")
	}
	return ""
}

// extractAddress extracts address from place data
func extractAddress(data map[string]any) string {
	locality := extractString(data, "addressLocality")
	region := extractString(data, "addressRegion")
	country := extractString(data, "addressCountry")

	var parts []string
	if locality != "" {
		parts = append(parts, locality)
	}
	if region != "" {
		parts = append(parts, region)
	}
	if country != "" {
		parts = append(parts, country)
	}

	return strings.Join(parts, ", ")
}

// formatDateTime formats an ISO 8601 datetime string for display
func formatDateTime(isoDate string) string {
	t, err := time.Parse(time.RFC3339, isoDate)
	if err != nil {
		return isoDate
	}
	return t.Format("Monday, January 2, 2006 at 3:04 PM MST")
}
