package jsonld

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SerializeToTurtle converts JSON-LD data to Turtle (text/turtle) format.
// It handles common Schema.org types (Event, Place, Organization) and
// provides a simplified RDF serialization suitable for content negotiation.
func SerializeToTurtle(jsonldData map[string]any) (string, error) {
	if jsonldData == nil {
		return "", fmt.Errorf("jsonldData cannot be nil")
	}

	var builder strings.Builder

	// Write common prefixes
	builder.WriteString("@prefix schema: <https://schema.org/> .\n")
	builder.WriteString("@prefix sel: <https://sharedevents.org/ns#> .\n")
	builder.WriteString("@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n")
	builder.WriteString("\n")

	// Extract subject URI (from @id or id)
	subjectURI, ok := jsonldData["@id"].(string)
	if !ok {
		if idVal, exists := jsonldData["id"].(string); exists {
			subjectURI = idVal
		} else {
			return "", fmt.Errorf("missing @id or id field")
		}
	}

	// Write subject with angle brackets
	builder.WriteString(fmt.Sprintf("<%s>\n", subjectURI))

	// Extract @type
	typeVal := extractType(jsonldData)
	if typeVal != "" {
		builder.WriteString(fmt.Sprintf("    a schema:%s ;\n", typeVal))
	}

	// Process properties in sorted order for consistent output
	properties := make([]string, 0, len(jsonldData))
	for key := range jsonldData {
		if key != "@context" && key != "@id" && key != "id" && key != "@type" && key != "type" {
			properties = append(properties, key)
		}
	}
	sort.Strings(properties)

	// Write each property as a triple
	for i, prop := range properties {
		value := jsonldData[prop]
		turtle := serializeProperty(prop, value)
		if turtle != "" {
			if i == len(properties)-1 {
				builder.WriteString(fmt.Sprintf("    %s .\n", turtle))
			} else {
				builder.WriteString(fmt.Sprintf("    %s ;\n", turtle))
			}
		}
	}

	return builder.String(), nil
}

// extractType extracts the @type or type field from JSON-LD
func extractType(data map[string]any) string {
	if typeVal, ok := data["@type"].(string); ok {
		return typeVal
	}
	if typeVal, ok := data["type"].(string); ok {
		return typeVal
	}
	// Handle array types (take first)
	if typeArr, ok := data["@type"].([]interface{}); ok && len(typeArr) > 0 {
		if typeStr, ok := typeArr[0].(string); ok {
			return typeStr
		}
	}
	if typeArr, ok := data["type"].([]interface{}); ok && len(typeArr) > 0 {
		if typeStr, ok := typeArr[0].(string); ok {
			return typeStr
		}
	}
	return ""
}

// serializeProperty serializes a single property to Turtle format
func serializeProperty(prop string, value any) string {
	// Skip null values
	if value == nil {
		return ""
	}

	predicate := fmt.Sprintf("schema:%s", prop)

	switch v := value.(type) {
	case string:
		// Check if it's a URI (starts with http:// or https://)
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return fmt.Sprintf("%s <%s>", predicate, v)
		}
		// Otherwise it's a literal string
		escaped := escapeLiteral(v)
		return fmt.Sprintf("%s \"%s\"", predicate, escaped)

	case float64, int, int64:
		return fmt.Sprintf("%s %v", predicate, v)

	case bool:
		return fmt.Sprintf("%s %t", predicate, v)

	case map[string]any:
		// Handle nested objects (like Place, Organization)
		if nestedID, ok := v["@id"].(string); ok {
			return fmt.Sprintf("%s <%s>", predicate, nestedID)
		}
		if nestedID, ok := v["id"].(string); ok {
			return fmt.Sprintf("%s <%s>", predicate, nestedID)
		}
		// If no ID, serialize as blank node (simplified)
		nestedJSON, _ := json.Marshal(v)
		escaped := escapeLiteral(string(nestedJSON))
		return fmt.Sprintf("%s \"%s\"", predicate, escaped)

	case []interface{}:
		// Handle arrays - serialize as multiple triples with same predicate
		// For now, just take the first item to keep it simple
		if len(v) > 0 {
			return serializeProperty(prop, v[0])
		}
		return ""

	default:
		// Fallback: serialize as JSON string
		jsonBytes, _ := json.Marshal(v)
		escaped := escapeLiteral(string(jsonBytes))
		return fmt.Sprintf("%s \"%s\"", predicate, escaped)
	}
}

// escapeLiteral escapes special characters in Turtle string literals
func escapeLiteral(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
