package postgres

import "github.com/jackc/pgx/v5/pgtype"

// derefString safely dereferences a string pointer, returning empty string if nil
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// pgtextToString converts pgtype.Text to string, returning empty string if not valid
func pgtextToString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
