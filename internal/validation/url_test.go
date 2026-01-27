package validation

import (
	"strings"
	"testing"
)

func TestValidateURL_ValidURLs(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		requireHTTPS bool
	}{
		{"HTTP URL", "http://example.com", false},
		{"HTTPS URL", "https://example.com", false},
		{"HTTPS URL with requireHTTPS", "https://example.com", true},
		{"URL with path", "https://example.com/path/to/resource", false},
		{"URL with query", "https://example.com?foo=bar", false},
		{"URL with fragment", "https://example.com#section", false},
		{"URL with port", "https://example.com:8080/path", false},
		{"Empty URL (allowed)", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url, "test_field", tt.requireHTTPS)
			if err != nil {
				t.Errorf("ValidateURL(%q, requireHTTPS=%v) returned error: %v", tt.url, tt.requireHTTPS, err)
			}
		})
	}
}

func TestValidateURL_InvalidURLs(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		requireHTTPS  bool
		expectedError string
	}{
		{"No scheme", "example.com", false, "must include a scheme"},
		{"HTTP when HTTPS required", "http://example.com", true, "must use HTTPS"},
		{"Invalid scheme", "ftp://example.com", false, "scheme must be http or https"},
		{"No host", "https://", false, "must include a host"},
		{"Malformed URL", "ht!tp://example.com", false, "invalid URL format"},
		{"Just scheme", "https", false, "must include a scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url, "test_field", tt.requireHTTPS)
			if err == nil {
				t.Errorf("ValidateURL(%q, requireHTTPS=%v) should return error", tt.url, tt.requireHTTPS)
				return
			}

			errMsg := err.Error()
			if !strings.Contains(errMsg, tt.expectedError) {
				t.Errorf("Error message %q should contain %q", errMsg, tt.expectedError)
			}
		})
	}
}

func TestValidateBaseURL_ValidBaseURLs(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		requireHTTPS bool
	}{
		{"HTTPS base URL", "https://example.com", false},
		{"HTTPS with trailing slash", "https://example.com/", false},
		{"HTTP base URL", "http://example.com", false},
		{"With port", "https://example.com:8080", false},
		{"HTTPS base URL with requireHTTPS", "https://example.com", true},
		{"Empty URL (allowed)", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBaseURL(tt.url, "base_url", tt.requireHTTPS)
			if err != nil {
				t.Errorf("ValidateBaseURL(%q, requireHTTPS=%v) returned error: %v", tt.url, tt.requireHTTPS, err)
			}
		})
	}
}

func TestValidateBaseURL_InvalidBaseURLs(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		requireHTTPS  bool
		expectedError string
	}{
		{"With path", "https://example.com/path", false, "must not contain a path"},
		{"With query", "https://example.com?foo=bar", false, "must not contain query parameters"},
		{"With fragment", "https://example.com#section", false, "must not contain a fragment"},
		{"HTTP when HTTPS required", "http://example.com", true, "must use HTTPS"},
		{"No scheme", "example.com", false, "must include a scheme"},
		{"With path and query", "https://example.com/path?foo=bar", false, "must not contain a path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBaseURL(tt.url, "base_url", tt.requireHTTPS)
			if err == nil {
				t.Errorf("ValidateBaseURL(%q, requireHTTPS=%v) should return error", tt.url, tt.requireHTTPS)
				return
			}

			errMsg := err.Error()
			if !strings.Contains(errMsg, tt.expectedError) {
				t.Errorf("Error message %q should contain %q", errMsg, tt.expectedError)
			}
		})
	}
}

func TestURLValidationError_ErrorMessage(t *testing.T) {
	err := URLValidationError{
		Field:   "base_url",
		Message: "must use HTTPS in production",
		URL:     "http://example.com",
	}

	expected := "base_url: must use HTTPS in production (url: http://example.com)"
	if err.Error() != expected {
		t.Errorf("Error message mismatch:\ngot:  %s\nwant: %s", err.Error(), expected)
	}
}

func TestValidateURL_EmptyURL(t *testing.T) {
	// Empty URLs should be allowed (field might be optional)
	err := ValidateURL("", "optional_url", true)
	if err != nil {
		t.Errorf("Empty URL should be allowed, got error: %v", err)
	}
}

func TestValidateBaseURL_EmptyURL(t *testing.T) {
	// Empty URLs should be allowed (field might be optional)
	err := ValidateBaseURL("", "optional_base_url", true)
	if err != nil {
		t.Errorf("Empty URL should be allowed, got error: %v", err)
	}
}

// Test various edge cases
func TestValidateURL_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		shouldErr bool
	}{
		{"Localhost", "http://localhost:3000", false},
		{"IP address", "https://192.168.1.1", false},
		{"IPv6", "https://[::1]:8080", false},
		{"Subdomain", "https://api.example.com", false},
		{"Very long domain", "https://very.long.subdomain.example.com/path", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url, "test_field", false)
			if (err != nil) != tt.shouldErr {
				t.Errorf("ValidateURL(%q) error = %v, shouldErr = %v", tt.url, err, tt.shouldErr)
			}
		})
	}
}

// Benchmark URL validation
func BenchmarkValidateURL(b *testing.B) {
	url := "https://example.com/path/to/resource"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateURL(url, "test_field", false)
	}
}

func BenchmarkValidateBaseURL(b *testing.B) {
	url := "https://example.com"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateBaseURL(url, "base_url", false)
	}
}
