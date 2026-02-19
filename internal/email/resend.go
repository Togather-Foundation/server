package email

import (
	"context"
	"errors"
	"fmt"

	"github.com/resend/resend-go/v2"
)

// sendViaResend sends an email using the Resend API.
// It handles rate limit errors gracefully without retrying.
func (s *Service) sendViaResend(ctx context.Context, to, subject, htmlBody string) error {
	if s.resendClient == nil {
		return fmt.Errorf("resend client not initialized")
	}

	params := &resend.SendEmailRequest{
		From:    s.config.From,
		To:      []string{to},
		Subject: subject,
		Html:    htmlBody,
	}

	sent, err := s.resendClient.Emails.SendWithContext(ctx, params)
	if err != nil {
		var rateLimitErr *resend.RateLimitError
		if errors.As(err, &rateLimitErr) {
			s.logger.Warn().
				Str("limit", rateLimitErr.Limit).
				Str("remaining", rateLimitErr.Remaining).
				Str("reset", rateLimitErr.Reset).
				Msg("resend rate limit exceeded")
			return fmt.Errorf("email rate limit exceeded (limit: %s, resets in: %s seconds): %w",
				rateLimitErr.Limit, rateLimitErr.Reset, err)
		}
		return fmt.Errorf("resend API error: %w", err)
	}

	s.logger.Info().
		Str("email_id", sent.Id).
		Str("to", to).
		Msg("email sent via Resend")
	return nil
}
