package artsdata

import (
	"encoding/json"
	"strconv"
)

// resolveString resolves a JSON-LD value to a plain string, tolerating the
// compacted shapes Artsdata emits on dereference:
//
//   - a plain string                    → returned as-is
//   - {"@none": "..."}                  → the scalar (compacted language-map tail)
//   - {"@value": "...", "type": "..."}  → the typed/scalar value
//   - a language map {"fr": ..., "en": ..., "@none": ...} → "@none", then "en",
//     then the first non-reserved scalar key
//
// It returns "" for nil, non-scalar, or reserved-only objects (e.g. {"@id": ...}).
func resolveString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case map[string]interface{}:
		if s := scalarString(val["@none"]); s != "" {
			return s
		}
		if s := scalarString(val["@value"]); s != "" {
			return s
		}
		if s := scalarString(val["en"]); s != "" {
			return s
		}
		for k, sub := range val {
			if reservedJSONLDKey(k) {
				continue
			}
			if s := scalarString(sub); s != "" {
				return s
			}
		}
		return ""
	default:
		return ""
	}
}

// scalarString returns the string form of a JSON scalar (string or number), or ""
// for anything else (including nested objects/arrays).
func scalarString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
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

// firstPresent returns the first key present in m with a non-nil value.
func firstPresent(m map[string]interface{}, keys ...string) interface{} {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

// UnmarshalJSON decodes a (possibly compacted) Artsdata JSON-LD dereference
// response, accepting `id`/`@id` and `type`/`@type` aliases and normalising the
// address block. RawJSON is intentionally not populated here; Dereference sets it
// from the raw response bytes.
func (e *EntityData) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	e.ID = resolveString(firstPresent(raw, "id", "@id"))
	e.Type = firstPresent(raw, "type", "@type")
	e.Name = raw["name"]
	e.SameAs = raw["sameAs"]
	e.Description = raw["description"]
	e.URL = raw["url"]

	if addr, ok := raw["address"].(map[string]interface{}); ok {
		a := &Address{}
		a.decode(addr)
		e.Address = a
	}
	return nil
}

// decode populates the address scalar fields from a raw address object, resolving
// plain-string, {"@none": ...}, and {"@value": ...} wrappers. The address's own
// id/type/@id/@type keys are ignored.
func (a *Address) decode(raw map[string]interface{}) {
	a.StreetAddress = resolveString(raw["streetAddress"])
	a.AddressLocality = resolveString(raw["addressLocality"])
	a.AddressRegion = resolveString(raw["addressRegion"])
	a.PostalCode = resolveString(raw["postalCode"])
	a.AddressCountry = resolveString(raw["addressCountry"])
}

// UnmarshalJSON decodes a JSON address object, normalising the scalar wrappers.
func (a *Address) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.decode(raw)
	return nil
}
