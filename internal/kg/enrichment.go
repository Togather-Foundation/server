package kg

// ExtractStringValue extracts a plain string from a JSON-LD interface{} value.
// Handles the following variants:
//   - plain string → returned as-is
//   - map[string]interface{} with "@value" key → the value string is returned
//   - nil or other types → empty string
func ExtractStringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case map[string]interface{}:
		if s, ok := val["@value"].(string); ok {
			return s
		}
		return ""
	default:
		return ""
	}
}
