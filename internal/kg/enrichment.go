package kg

import "strconv"

// ExtractStringValue extracts a plain string from a JSON-LD interface{} value.
// Handles the following variants:
//   - plain string → returned as-is
//   - {"@value": ...} (plain or typed value object) → the scalar value string
//   - {"@none": ...} → the scalar (compacted language-map tail)
//   - a language map {"fr": ..., "en": ..., "@none": ...} → "@none", then "en",
//     then the first non-reserved scalar key
//   - nil or other types → empty string
func ExtractStringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case map[string]interface{}:
		if s := scalarStringValue(val["@none"]); s != "" {
			return s
		}
		if s := scalarStringValue(val["@value"]); s != "" {
			return s
		}
		if s := scalarStringValue(val["en"]); s != "" {
			return s
		}
		for k, sub := range val {
			if reservedJSONLDKey(k) {
				continue
			}
			if s := scalarStringValue(sub); s != "" {
				return s
			}
		}
		return ""
	default:
		return ""
	}
}

// scalarStringValue returns the string form of a JSON scalar (string or number),
// or "" for anything else.
func scalarStringValue(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

// reservedJSONLDKey reports whether k is a JSON-LD keyword/alias that must not be
// treated as a language tag when falling back through a language map.
func reservedJSONLDKey(k string) bool {
	switch k {
	case "@id", "@type", "@value", "@none", "@language", "@context", "id", "type":
		return true
	default:
		return false
	}
}
