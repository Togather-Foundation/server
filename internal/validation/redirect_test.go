package validation

import (
	"testing"
)

func TestIsSafeRelativeRedirect(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "valid dashboard path", input: "/admin/dashboard", expected: "/admin/dashboard"},
		{name: "valid events path", input: "/admin/events", expected: "/admin/events"},
		{name: "path with query params", input: "/admin/events?foo=bar", expected: "/admin/events?foo=bar"},
		{name: "path with fragment", input: "/admin/dashboard#section", expected: "/admin/dashboard#section"},
		{name: "path with query and fragment", input: "/admin/events?foo=bar#baz", expected: "/admin/events?foo=bar#baz"},
		{name: "protocol-relative URL", input: "//evil.com", expected: "/admin/dashboard"},
		{name: "protocol-relative URL with path", input: "//evil.com/phish", expected: "/admin/dashboard"},
		{name: "absolute HTTPS URL", input: "https://evil.com", expected: "/admin/dashboard"},
		{name: "absolute HTTP URL", input: "http://evil.com", expected: "/admin/dashboard"},
		{name: "javascript scheme", input: "javascript:alert(1)", expected: "/admin/dashboard"},
		{name: "path traversal", input: "/admin/../../../etc/passwd", expected: "/admin/dashboard"},
		{name: "percent-encoded traversal", input: "/admin/..%2f..%2f", expected: "/admin/dashboard"},
		{name: "backslash traversal", input: "/admin/..\\..\\", expected: "/admin/dashboard"},
		{name: "empty string", input: "", expected: "/admin/dashboard"},
		{name: "backslash path", input: "\\evil.com", expected: "/admin/dashboard"},
		{name: "backslash in path", input: "/\\evil.com", expected: "/admin/dashboard"},
		{name: "CRLF injection", input: "/admin\r\nSet-Cookie: evil=true", expected: "/admin/dashboard"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSafeRelativeRedirect(tt.input)
			if got != tt.expected {
				t.Errorf("IsSafeRelativeRedirect(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
