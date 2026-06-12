package validation

import (
	"net/url"
	"strings"
)

// IsSafeRelativeRedirect validates that a redirect target is a safe same-origin
// relative URL. Returns the cleaned path if safe, or fallback otherwise.
//
// Rejects: absolute URLs (scheme://host), protocol-relative URLs (//evil.com),
// path traversal (..), backslashes, and CR/LF injection.
func IsSafeRelativeRedirect(raw, fallback string) string {
	if raw == "" {
		return fallback
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fallback
	}

	if parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" {
		return fallback
	}

	path := parsed.Path

	if unescaped, err := url.PathUnescape(path); err == nil {
		if strings.Contains(unescaped, "..") {
			return fallback
		}
	} else {
		return fallback
	}

	if strings.ContainsAny(path, "\\\r\n") {
		return fallback
	}

	if strings.ContainsAny(parsed.RawQuery, "\r\n") || strings.ContainsAny(parsed.Fragment, "\r\n") {
		return fallback
	}
	if q, err := url.QueryUnescape(parsed.RawQuery); err == nil {
		if strings.ContainsAny(q, "\r\n") {
			return fallback
		}
	}
	if f, err := url.QueryUnescape(parsed.Fragment); err == nil {
		if strings.ContainsAny(f, "\r\n") {
			return fallback
		}
	}

	if !strings.HasPrefix(path, "/") {
		return fallback
	}

	result := path
	if parsed.RawQuery != "" {
		result += "?" + parsed.RawQuery
	}
	if parsed.Fragment != "" {
		result += "#" + parsed.Fragment
	}

	return result
}
