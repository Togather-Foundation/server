package postgres

import "strings"

// derefString safely dereferences a string pointer, returning empty string if nil
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// nullableString trims whitespace and returns nil for empty strings
func nullableString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
