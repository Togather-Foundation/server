package email

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/rs/zerolog"
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

// === Service-Level Tests ===

// setupTestService creates a service with email disabled for most tests
func setupTestService(t *testing.T) *Service {
	t.Helper()

	cfg := config.EmailConfig{
		Enabled:      false, // Don't send real emails in tests
		From:         "test@example.com",
		SMTPHost:     "smtp.test.com",
		SMTPPort:     587,
		SMTPUser:     "testuser",
		SMTPPassword: "testpass",
	}

	// Use test template directory
	templateDir := "testdata/templates"

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	svc, err := NewService(cfg, templateDir, logger)
	if err != nil {
		t.Fatalf("Failed to create test service: %v", err)
	}

	return svc
}

// Test NewService with valid configuration
func TestNewService_Success(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:      false,
		From:         "test@example.com",
		SMTPHost:     "smtp.test.com",
		SMTPPort:     587,
		SMTPUser:     "testuser",
		SMTPPassword: "testpass",
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	svc, err := NewService(cfg, "testdata/templates", logger)

	if err != nil {
		t.Errorf("Expected successful service creation, got error: %v", err)
	}

	if svc == nil {
		t.Fatal("Expected non-nil service")
	}

	if svc.config.From != cfg.From {
		t.Errorf("Expected From=%s, got %s", cfg.From, svc.config.From)
	}
}

// Test NewService with invalid sender email when enabled
func TestNewService_InvalidSenderEmail(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:      true, // Validation only happens when enabled
		From:         "not-an-email",
		SMTPHost:     "smtp.test.com",
		SMTPPort:     587,
		SMTPUser:     "testuser",
		SMTPPassword: "testpass",
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	_, err := NewService(cfg, "testdata/templates", logger)

	if err == nil {
		t.Error("Expected error for invalid sender email, got nil")
	}

	if !strings.Contains(err.Error(), "invalid sender email") {
		t.Errorf("Expected 'invalid sender email' error, got: %v", err)
	}
}

// Test NewService with non-existent template directory
func TestNewService_InvalidTemplateDir(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:      false,
		From:         "test@example.com",
		SMTPHost:     "smtp.test.com",
		SMTPPort:     587,
		SMTPUser:     "testuser",
		SMTPPassword: "testpass",
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	_, err := NewService(cfg, "/nonexistent/path", logger)

	if err == nil {
		t.Error("Expected error for invalid template directory, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse email templates") {
		t.Errorf("Expected 'failed to parse email templates' error, got: %v", err)
	}
}

// Test NewService with malformed template
func TestNewService_TemplateParseError(t *testing.T) {
	// Create temporary directory with malformed template
	tmpDir := t.TempDir()
	malformedTemplate := filepath.Join(tmpDir, "bad.html")

	// Write malformed template
	err := os.WriteFile(malformedTemplate, []byte("{{.Field {{.Broken}}"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test template: %v", err)
	}

	cfg := config.EmailConfig{
		Enabled:  false,
		From:     "test@example.com",
		SMTPHost: "smtp.test.com",
		SMTPPort: 587,
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	_, err = NewService(cfg, tmpDir, logger)

	if err == nil {
		t.Error("Expected error for malformed template, got nil")
	}
}

// Test NewService with email disabled (should not validate SMTP config)
func TestNewService_EmailDisabled(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:      false,
		From:         "test@example.com",
		SMTPHost:     "", // Empty SMTP config
		SMTPPort:     0,  // Zero port
		SMTPUser:     "", // No user
		SMTPPassword: "", // No password
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	svc, err := NewService(cfg, "testdata/templates", logger)

	if err != nil {
		t.Errorf("Service creation should succeed when disabled, got error: %v", err)
	}

	if svc == nil {
		t.Error("Expected non-nil service")
	}
}

// Test SendInvitation when email is disabled
func TestSendInvitation_EmailDisabled(t *testing.T) {
	svc := setupTestService(t) // Enabled=false

	err := svc.SendInvitation("user@example.com", "https://example.com/invite?token=abc123", "Admin User")

	if err != nil {
		t.Errorf("SendInvitation should succeed (log only) when disabled, got error: %v", err)
	}
}

// Test SendInvitation with invalid recipient email addresses
func TestSendInvitation_InvalidRecipient(t *testing.T) {
	svc := setupTestService(t)

	testCases := []struct {
		name  string
		email string
	}{
		{"missing @", "not-an-email"},
		{"missing domain", "missing@"},
		{"missing local part", "@example.com"},
		{"spaces", "user name@example.com"},
		{"header injection CR", "user@example.com\r\nBcc: attacker@evil.com"},
		{"header injection LF", "test@example.com\nCc: hacker@evil.com"},
		{"empty string", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.SendInvitation(tc.email, "https://example.com/invite", "Admin")

			if err == nil {
				t.Errorf("Expected error for invalid email: %s", tc.email)
			}

			if !strings.Contains(err.Error(), "invalid recipient email") {
				t.Errorf("Expected 'invalid recipient email' error, got: %v", err)
			}
		})
	}
}

// Test SendInvitation with invalid invite URLs
func TestSendInvitation_InvalidURL(t *testing.T) {
	svc := setupTestService(t)

	testCases := []struct {
		name string
		url  string
	}{
		{"javascript scheme", "javascript:alert('xss')"},
		{"data scheme", "data:text/html,<script>alert('xss')</script>"},
		{"file scheme", "file:///etc/passwd"},
		{"no scheme", "example.com/invite"},
		{"empty string", ""},
		{"malformed URL", "ht!tp://bad url"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.SendInvitation("user@example.com", tc.url, "Admin")

			if err == nil {
				t.Errorf("Expected error for invalid URL: %s", tc.url)
			}

			if !strings.Contains(err.Error(), "invalid invite link") {
				t.Errorf("Expected 'invalid invite link' error, got: %v", err)
			}
		})
	}
}

// Test template rendering with valid data
func TestRenderTemplate_Success(t *testing.T) {
	svc := setupTestService(t)

	data := InvitationData{
		InviteLink:  "https://example.com/invite?token=abc123",
		InvitedBy:   "Admin User",
		CurrentYear: time.Now().Year(),
	}

	html, err := svc.renderTemplate("invitation.html", data)

	if err != nil {
		t.Errorf("Template render failed: %v", err)
	}

	// Verify key elements are present
	if !strings.Contains(html, data.InviteLink) {
		t.Error("Template missing invite link")
	}

	if !strings.Contains(html, data.InvitedBy) {
		t.Error("Template missing inviter name")
	}

	// Verify HTML structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Template missing DOCTYPE")
	}

	if !strings.Contains(html, "Accept Invitation") {
		t.Error("Template missing call to action")
	}

	// Verify copyright year is rendered dynamically
	expectedYear := fmt.Sprintf("&copy; %d", data.CurrentYear)
	if !strings.Contains(html, expectedYear) {
		// Try alternative formats (template might escape differently)
		altYear := fmt.Sprintf("© %d", data.CurrentYear)
		yearOnly := fmt.Sprintf("%d", data.CurrentYear)

		// Debug: print what we find around "Togather"
		idx := strings.Index(html, "Togather Shared Events Library")
		if idx >= 0 {
			endIdx := idx + 100
			if endIdx > len(html) {
				endIdx = len(html)
			}
			t.Logf("Found around 'Togather Shared Events Library': %q", html[idx:endIdx])
		}

		if !strings.Contains(html, altYear) && !strings.Contains(html, yearOnly) {
			t.Errorf("Template missing copyright year, expected to find year %d", data.CurrentYear)
		}
	}
}

// Test template rendering with missing template
func TestRenderTemplate_MissingTemplate(t *testing.T) {
	svc := setupTestService(t)

	_, err := svc.renderTemplate("nonexistent.html", nil)

	if err == nil {
		t.Error("Expected error for missing template, got nil")
	}

	if !strings.Contains(err.Error(), "failed to execute template") {
		t.Errorf("Expected 'failed to execute template' error, got: %v", err)
	}
}

// Test template rendering with nil data (should still work)
func TestRenderTemplate_NilData(t *testing.T) {
	svc := setupTestService(t)

	// Render with nil data - template fields will be empty
	html, err := svc.renderTemplate("invitation.html", nil)

	if err != nil {
		t.Errorf("Template render with nil data should succeed, got error: %v", err)
	}

	// Should still have HTML structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Template missing DOCTYPE")
	}
}

// Test XSS protection in template rendering
func TestRenderTemplate_XSSEscaping(t *testing.T) {
	svc := setupTestService(t)

	xssAttempts := []struct {
		input       string
		unsafeTags  []string // Tags that must not appear verbatim
		description string
	}{
		{
			input:       "<script>alert('xss')</script>",
			unsafeTags:  []string{"<script>"},
			description: "script tag injection",
		},
		{
			input:       "<img src=x onerror=alert('xss')>",
			unsafeTags:  []string{"<img"},
			description: "img tag with onerror",
		},
		{
			input:       "<svg onload=alert('xss')>",
			unsafeTags:  []string{"<svg"},
			description: "svg tag with onload",
		},
	}

	for _, tc := range xssAttempts {
		t.Run(tc.description, func(t *testing.T) {
			data := InvitationData{
				InviteLink:  "https://example.com/invite",
				InvitedBy:   tc.input,
				CurrentYear: time.Now().Year(),
			}

			html, err := svc.renderTemplate("invitation.html", data)

			if err != nil {
				t.Fatalf("Template render failed: %v", err)
			}

			// Verify dangerous tags are escaped (Go templates auto-escape HTML)
			for _, tag := range tc.unsafeTags {
				if strings.Contains(html, tag) {
					t.Errorf("XSS: dangerous tag '%s' not escaped in output", tag)
				}
			}

			// Verify escaped version is present (< becomes &lt;)
			if !strings.Contains(html, "&lt;") {
				t.Error("Expected HTML-escaped content for XSS attempt")
			}
		})
	}
}

// Test that send() validates recipient email
func TestSend_ValidatesRecipient(t *testing.T) {
	svc := setupTestService(t)

	// Test with header injection in recipient
	err := svc.send("user@example.com\r\nBcc: attacker@evil.com", "Test Subject", "<p>Test Body</p>")

	if err == nil {
		t.Error("Expected error for invalid recipient in send()")
	}

	if !strings.Contains(err.Error(), "invalid recipient email") {
		t.Errorf("Expected 'invalid recipient email' error, got: %v", err)
	}
}

// Test SendInvitation with all valid inputs (email disabled)
func TestSendInvitation_ValidInputs_Disabled(t *testing.T) {
	svc := setupTestService(t)

	err := svc.SendInvitation(
		"newuser@example.com",
		"https://togather.foundation/invite?token=abc123def456",
		"Admin User",
	)

	if err != nil {
		t.Errorf("SendInvitation with valid inputs should succeed, got error: %v", err)
	}
}

// Integration test with MailHog (requires external SMTP server)
// Run with: go test -v ./internal/email/ -run TestSendInvitation_E2E
func TestSendInvitation_E2E_WithMailHog(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E email test in short mode (use -short=false to enable)")
	}

	// Check if MailHog is available
	// docker run -d -p 1025:1025 -p 8025:8025 mailhog/mailhog
	cfg := config.EmailConfig{
		Enabled:      true,
		From:         "test@example.com",
		SMTPHost:     "localhost",
		SMTPPort:     1025,
		SMTPUser:     "", // MailHog doesn't require auth
		SMTPPassword: "",
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	svc, err := NewService(cfg, "testdata/templates", logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	err = svc.SendInvitation(
		"recipient@example.com",
		"https://togather.foundation/invite?token=test123",
		"Admin User",
	)

	if err != nil {
		// If connection refused, MailHog is not running - skip test
		if strings.Contains(err.Error(), "connection refused") {
			t.Skip("MailHog not running on localhost:1025 - skipping E2E test")
		}
		t.Errorf("Email send failed: %v", err)
	}

	t.Log("✓ Email sent successfully to MailHog")
	t.Log("Check http://localhost:8025 to view the email")
}
