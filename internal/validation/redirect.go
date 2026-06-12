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

	if unescaped, err := url.PathUnescape(path); err == nil {
		if strings.Contains(unescaped, "..") {
			return "/admin/dashboard"
		}
	} else {
		return "/admin/dashboard"
	}

	if strings.ContainsAny(path, "\\\r\n") {
		return "/admin/dashboard"
	}

	if strings.ContainsAny(parsed.RawQuery, "\r\n") || strings.ContainsAny(parsed.Fragment, "\r\n") {
		return "/admin/dashboard"
	}
	if q, err := url.QueryUnescape(parsed.RawQuery); err == nil {
		if strings.ContainsAny(q, "\r\n") {
			return "/admin/dashboard"
		}
	}
	if f, err := url.QueryUnescape(parsed.Fragment); err == nil {
		if strings.ContainsAny(f, "\r\n") {
			return "/admin/dashboard"
		}
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
