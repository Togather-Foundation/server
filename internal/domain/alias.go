package domain

import (
	"fmt"
	"net/url"
)

func ResolveAlias(values url.Values, canonical, alias string, warnings *[]string) string {
	if values.Get(canonical) != "" {
		return values.Get(canonical)
	}
	if v := values.Get(alias); v != "" {
		*warnings = append(*warnings, fmt.Sprintf("Unrecognised parameter alias %q — use %q instead", alias, canonical))
		return v
	}
	return ""
}
