package email

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/mail"
	"net/smtp"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/rs/zerolog"
)

// Service handles email sending with SMTP
type Service struct {
	config    config.EmailConfig
	templates *template.Template
	logger    zerolog.Logger
}

// InvitationData holds data for rendering the invitation email template
type InvitationData struct {
	InvitedBy   string // Name/email of the admin who sent the invitation
	InviteLink  string // Full URL to accept the invitation
	CurrentYear int    // Current year for copyright notice
}

// NewService creates a new email service instance
// templatesDir should point to the directory containing HTML email templates (e.g., "web/email/templates")
func NewService(cfg config.EmailConfig, templatesDir string, logger zerolog.Logger) (*Service, error) {
	// Validate sender email address if email is enabled
	if cfg.Enabled {
		if err := validateEmailAddress(cfg.From); err != nil {
			return nil, fmt.Errorf("invalid sender email in config: %w", err)
		}
	}

	// Parse all HTML templates in the directory
	pattern := filepath.Join(templatesDir, "*.html")
	templates, err := template.ParseGlob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to parse email templates: %w", err)
	}

	return &Service{
		config:    cfg,
		templates: templates,
		logger:    logger.With().Str("component", "email").Logger(),
	}, nil
}

// SendInvitation sends an invitation email to a new user
func (s *Service) SendInvitation(to, inviteLink, invitedBy string) error {
	// Validate recipient email address early
	if err := validateEmailAddress(to); err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}

	// Validate invite link URL to prevent XSS attacks (javascript:, data:, etc.)
	if err := validateInviteURL(inviteLink); err != nil {
		return fmt.Errorf("invalid invite link: %w", err)
	}

	// If email is disabled, just log and return
	if !s.config.Enabled {
		s.logger.Info().
			Str("to", to).
			Str("invited_by", invitedBy).
			Str("link", inviteLink).
			Msg("email service disabled, skipping invitation email")
		return nil
	}

	// Render the invitation template
	data := InvitationData{
		InvitedBy:   invitedBy,
		InviteLink:  inviteLink,
		CurrentYear: time.Now().Year(),
	}
	htmlBody, err := s.renderTemplate("invitation.html", data)
	if err != nil {
		return fmt.Errorf("failed to render invitation template: %w", err)
	}

	subject := "Welcome to Togather - Set Up Your Account"
	if err := s.send(to, subject, htmlBody); err != nil {
		return fmt.Errorf("failed to send invitation email: %w", err)
	}

	s.logger.Info().
		Str("to", to).
		Str("invited_by", invitedBy).
		Msg("invitation email sent successfully")
	return nil
}

// validateEmailAddress validates an email address for format and header injection attempts
func validateEmailAddress(email string) error {
	// Parse email address format
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return fmt.Errorf("invalid email format: %w", err)
	}

	// Check for header injection attempts (newlines)
	if strings.ContainsAny(addr.Address, "\r\n") {
		return fmt.Errorf("invalid email address: contains newline characters")
	}

	return nil
}

// validateInviteURL validates that the invite link is a safe HTTP(S) URL
// This prevents XSS attacks via javascript:, data:, or other dangerous URL schemes
func validateInviteURL(link string) error {
	// Parse the URL
	u, err := url.Parse(link)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Ensure it's an http or https URL, not javascript:, data:, etc.
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (must be http or https)", u.Scheme)
	}

	// Ensure there's a host component
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

// send sends an email with the given subject and HTML body via Gmail SMTP
func (s *Service) send(to, subject, htmlBody string) error {
	// Validate recipient email address to prevent header injection
	if err := validateEmailAddress(to); err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}

	// Build email message with MIME headers
	from := s.config.From
	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var msg bytes.Buffer
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	// Connect to Gmail SMTP server
	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)
	auth := smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPassword, s.config.SMTPHost)

	// Use STARTTLS for secure connection (port 587)
	// Note: Gmail requires TLS, so we need to manually handle the connection
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer func() { _ = client.Close() }()

	// Start TLS handshake with explicit security settings
	tlsConfig := &tls.Config{
		ServerName:         s.config.SMTPHost,
		InsecureSkipVerify: false,            // Explicit: always verify certificates
		MinVersion:         tls.VersionTLS12, // Require TLS 1.2 or higher
	}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("failed to start TLS: %w", err)
	}

	// Authenticate
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	// Set sender
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipient
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}

	// Send email body
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open data writer: %w", err)
	}
	if _, err := w.Write(msg.Bytes()); err != nil {
		return fmt.Errorf("failed to write email body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	// Send QUIT command
	if err := client.Quit(); err != nil {
		return fmt.Errorf("failed to quit SMTP connection: %w", err)
	}

	return nil
}

// renderTemplate renders an email template with the given data
func (s *Service) renderTemplate(name string, data interface{}) (string, error) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", name, err)
	}
	return buf.String(), nil
}
