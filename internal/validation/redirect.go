package validation

import (
	"net/url"
	"strings"
)

// IsSafeRelativeRedirect validates that a redirect target is a safe same-origin
// relative URL. Returns the cleaned path if safe, or "/admin/dashboard" otherwise.
//
// Rejects: absolute URLs (scheme://host), protocol-relative URLs (//evil.com),
// path traversal (..), backslashes, and CR/LF injection.
func IsSafeRelativeRedirect(raw string) string {
	if raw == "" {
		return "/admin/dashboard"
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "/admin/dashboard"
	}

	if parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" {
		return "/admin/dashboard"
	}

	path := parsed.Path

	if strings.Contains(path, "..") {
		return "/admin/dashboard"
	}

	if strings.ContainsAny(path, "\\\r\n") {
		return "/admin/dashboard"
	}

	if !strings.HasPrefix(path, "/") {
		return "/admin/dashboard"
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
