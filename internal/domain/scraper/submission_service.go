package scraper

import (
	"context"
	"fmt"
)

// SubmissionService handles the synchronous validation and queuing of URL submissions.
type SubmissionService struct {
	repo           SubmissionRepository
	rateLimitPerIP int
}

// NewSubmissionService creates a new SubmissionService backed by the given repository.
// rateLimitPerIP is the maximum number of URLs a single IP may submit in 24 hours;
// the value comes from config.RateLimit.SubmissionsPerIPPer24h.
func NewSubmissionService(repo SubmissionRepository, rateLimitPerIP int) *SubmissionService {
	return &SubmissionService{repo: repo, rateLimitPerIP: rateLimitPerIP}
}

// Submit processes a batch of URLs. It validates format, dedup, and rate limit,
// then inserts accepted URLs. Returns per-URL results.
//
// Rate limit is a pre-check only: if the IP already has ≥rateLimitPerIP accepted
// submissions in the last 24h, the entire request is rejected with
// ErrRateLimitExceeded (429). If the IP has at least one slot remaining when the
// request arrives, the whole batch goes through regardless of how many URLs are in
// it. Attack surface is bounded to 2N-1 accepted URLs across two back-to-back
// requests, which is acceptable for MVP.
func (s *SubmissionService) Submit(ctx context.Context, urls []string, submitterIP string) ([]SubmissionResult, error) {
	// Pre-check: reject the entire request if the IP is already at quota.
	count, err := s.repo.CountRecentByIP(ctx, submitterIP)
	if err != nil {
		return nil, fmt.Errorf("count recent submissions by IP: %w", err)
	}
	if int(count) >= s.rateLimitPerIP {
		return nil, ErrRateLimitExceeded
	}

	results := make([]SubmissionResult, 0, len(urls))

	for _, rawURL := range urls {
		result, err := s.processURL(ctx, rawURL, submitterIP)
		if err != nil {
			return nil, fmt.Errorf("process URL %q: %w", rawURL, err)
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
