package email

import (
	"testing"
)

func TestValidateEmailAddress_Valid(t *testing.T) {
	tests := []string{
		"user@example.com",
		"test.user@example.com",
		"user+tag@example.co.uk",
		"firstname.lastname@company.org",
		"User Name <user@example.com>", // RFC 5322 format with display name
	}

	for _, email := range tests {
		t.Run(email, func(t *testing.T) {
			err := validateEmailAddress(email)
			if err != nil {
				t.Errorf("Expected valid email %q to pass validation, got error: %v", email, err)
			}
		})
	}
}

func TestValidateEmailAddress_InvalidFormat(t *testing.T) {
	tests := []struct {
		email       string
		description string
	}{
		{"", "empty string"},
		{"notanemail", "no @ symbol"},
		{"@example.com", "missing local part"},
		{"user@", "missing domain"},
		{"user @example.com", "space before @"},
		{"user@exam ple.com", "space in domain"},
		{"user@@example.com", "double @"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			err := validateEmailAddress(tt.email)
			if err == nil {
				t.Errorf("Expected error for invalid email %q (%s), but got none", tt.email, tt.description)
			}
		})
	}
}

func TestValidateEmailAddress_HeaderInjection(t *testing.T) {
	tests := []struct {
		email       string
		description string
	}{
		{
			"victim@example.com\r\nBcc: attacker@evil.com",
			"CRLF with Bcc injection",
		},
		{
			"test@example.com\nCc: hacker@evil.com",
			"LF with Cc injection",
		},
		{
			"user@domain.com\r\nSubject: Phishing",
			"CRLF with Subject injection",
		},
		{
			"user@domain.com\rX-Mailer: Evil",
			"CR with custom header injection",
		},
		{
			"user@domain.com\nContent-Type: text/plain",
			"LF with Content-Type injection",
		},
		{
			"attacker@evil.com\r\n\r\n<html><body>Phishing content</body></html>",
			"double CRLF to inject body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			err := validateEmailAddress(tt.email)
			if err == nil {
				t.Errorf("Expected error for email with header injection %q (%s), but got none", tt.email, tt.description)
			}
		})
	}
}

func TestValidateEmailAddress_EdgeCases(t *testing.T) {
	tests := []struct {
		email       string
		shouldPass  bool
		description string
	}{
		{"user+tag@example.com", true, "plus addressing"},
		{"user.name@example.com", true, "dots in local part"},
		{"123@example.com", true, "numeric local part"},
		{"a@b.c", true, "minimal valid email"},
		{"user@subdomain.example.com", true, "subdomain"},
		{"user@example.co.uk", true, "multi-level TLD"},
		{"User <user@example.com>", true, "display name with angle brackets"},
		{"user@[192.168.1.1]", true, "IP address domain (RFC 5321)"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			err := validateEmailAddress(tt.email)
			if tt.shouldPass && err != nil {
				t.Errorf("Expected email %q to pass validation (%s), got error: %v", tt.email, tt.description, err)
			}
			if !tt.shouldPass && err == nil {
				t.Errorf("Expected email %q to fail validation (%s), but got none", tt.email, tt.description)
			}
		})
	}
}

func TestValidateInviteURL_Valid(t *testing.T) {
	tests := []string{
		"https://example.com/invite",
		"http://example.com/invite",
		"https://subdomain.example.com/path/to/invite",
		"https://example.com:8080/invite",
		"https://example.com/invite?token=abc123",
		"https://example.com/invite#section",
		"https://192.168.1.1/invite",
		"https://[::1]/invite", // IPv6
	}

	for _, link := range tests {
		t.Run(link, func(t *testing.T) {
			err := validateInviteURL(link)
			if err != nil {
				t.Errorf("Expected valid URL %q to pass validation, got error: %v", link, err)
			}
		})
	}
}

func TestValidateInviteURL_InvalidScheme(t *testing.T) {
	tests := []struct {
		url         string
		description string
	}{
		{"javascript:alert('xss')", "javascript protocol"},
		{"data:text/html,<script>alert('xss')</script>", "data protocol"},
		{"vbscript:msgbox('xss')", "vbscript protocol"},
		{"file:///etc/passwd", "file protocol"},
		{"ftp://example.com/file", "ftp protocol"},
		{"tel:+1234567890", "tel protocol"},
		{"mailto:user@example.com", "mailto protocol"},
		{"ssh://user@host", "ssh protocol"},
		{"//example.com/invite", "protocol-relative URL"},
		{"/path/to/invite", "relative path"},
		{"invite", "bare path"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			err := validateInviteURL(tt.url)
			if err == nil {
				t.Errorf("Expected error for URL with invalid scheme %q (%s), but got none", tt.url, tt.description)
			}
		})
	}
}

func TestValidateInviteURL_MalformedURL(t *testing.T) {
	tests := []struct {
		url         string
		description string
	}{
		{"", "empty string"},
		{"not a url", "not a URL"},
		{"http://", "missing host"},
		{"https://", "missing host"},
		{"ht!tp://example.com", "invalid characters"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			err := validateInviteURL(tt.url)
			if err == nil {
				t.Errorf("Expected error for malformed URL %q (%s), but got none", tt.url, tt.description)
			}
		})
	}
}

func TestValidateInviteURL_XSSVectors(t *testing.T) {
	// Common XSS attack vectors that should be blocked
	tests := []string{
		"javascript:alert('XSS')",
		"javascript:alert(document.cookie)",
		"javascript:void(0)",
		"javascript://example.com%0Aalert('XSS')",
		"data:text/html,<script>alert('XSS')</script>",
		"data:text/html;base64,PHNjcmlwdD5hbGVydCgnWFNTJyk8L3NjcmlwdD4=",
		"vbscript:msgbox('XSS')",
	}

	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			err := validateInviteURL(url)
			if err == nil {
				t.Errorf("Expected XSS vector %q to be blocked, but validation passed", url)
			}
		})
	}
}
