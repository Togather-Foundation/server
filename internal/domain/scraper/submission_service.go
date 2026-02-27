package scraper

import (
	"context"
	"fmt"
)

const (
	// rateLimitPerIP is the maximum number of URLs a single IP may submit in 24 hours.
	rateLimitPerIP = 5
)

// SubmissionService handles the synchronous validation and queuing of URL submissions.
type SubmissionService struct {
	repo SubmissionRepository
}

// NewSubmissionService creates a new SubmissionService backed by the given repository.
func NewSubmissionService(repo SubmissionRepository) *SubmissionService {
	return &SubmissionService{repo: repo}
}

// Submit processes a batch of URLs. It validates format, dedup, and rate limit,
// then inserts accepted URLs. Returns per-URL results.
// Returns ErrRateLimitExceeded if the IP has already reached 5 URLs in 24h.
//
// Per-batch quota accounting: a batch of N valid URLs uses N of the remaining
// slots.  Once the remaining capacity drops to zero, additional URLs in the
// same batch receive a "rate_limited" status rather than "accepted".
func (s *SubmissionService) Submit(ctx context.Context, urls []string, submitterIP string) ([]SubmissionResult, error) {
	// Check per-IP rate limit before processing any URL.
	count, err := s.repo.CountRecentByIP(ctx, submitterIP)
	if err != nil {
		return nil, fmt.Errorf("count recent submissions by IP: %w", err)
	}
	if count >= rateLimitPerIP {
		return nil, ErrRateLimitExceeded
	}

	// Track remaining capacity for this batch so that a single request cannot
	// exceed the per-IP quota across multiple URLs (spec §Rate Limiting).
	remaining := rateLimitPerIP - count

	results := make([]SubmissionResult, 0, len(urls))

	for _, rawURL := range urls {
		if remaining <= 0 {
			// Quota exhausted within this batch — report remaining URLs as rate-limited.
			results = append(results, SubmissionResult{
				URL:     rawURL,
				Status:  "rate_limited",
				Message: "Rate limit reached; remaining URLs in this batch were not accepted",
			})
			continue
		}

		result, err := s.processURL(ctx, rawURL, submitterIP)
		if err != nil {
			return nil, fmt.Errorf("process URL %q: %w", rawURL, err)
		}
		if result.Status == "accepted" {
			remaining--
		}
		results = append(results, result)
	}

	return results, nil
}

// processURL handles a single URL: normalise → dedup → insert.
func (s *SubmissionService) processURL(ctx context.Context, rawURL, submitterIP string) (SubmissionResult, error) {
	original, normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return SubmissionResult{
			URL:     rawURL,
			Status:  "rejected",
			Message: fmt.Sprintf("Invalid URL: %s", err.Error()),
		}, nil
	}

	// Dedup check.
	existing, err := s.repo.GetRecentByURLNorm(ctx, normalized)
	if err != nil {
		return SubmissionResult{}, fmt.Errorf("dedup check: %w", err)
	}
	if existing != nil {
		return SubmissionResult{
			URL:     original,
			Status:  "duplicate",
			Message: "Already submitted within 30 days",
		}, nil
	}

	// Insert accepted submission.
	sub := &Submission{
		URL:         original,
		URLNorm:     normalized,
		SubmitterIP: submitterIP,
		Status:      "pending_validation",
	}
	if _, err := s.repo.Insert(ctx, sub); err != nil {
		return SubmissionResult{}, fmt.Errorf("insert submission: %w", err)
	}

	return SubmissionResult{
		URL:     original,
		Status:  "accepted",
		Message: "URL queued for review",
	}, nil
}
