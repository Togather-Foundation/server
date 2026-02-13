package postgres

// derefString safely dereferences a string pointer, returning empty string if nil
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
