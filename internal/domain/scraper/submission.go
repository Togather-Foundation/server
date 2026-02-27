package scraper

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ErrRateLimitExceeded is returned when the per-IP rate limit is exceeded.
var ErrRateLimitExceeded = errors.New("rate limit exceeded")

// Submission represents a submitted URL awaiting validation.
type Submission struct {
	ID              int64
	URL             string
	URLNorm         string
	SubmittedAt     time.Time
	SubmitterIP     string
	Status          string
	RejectionReason *string
	Notes           *string
	ValidatedAt     *time.Time
}

// SubmissionResult is the per-URL response to the public endpoint.
type SubmissionResult struct {
	URL     string `json:"url"`
	Status  string `json:"status"` // "accepted" | "duplicate" | "rejected"
	Message string `json:"message"`
}

// SubmissionRepository defines storage operations for submissions.
type SubmissionRepository interface {
	// GetRecentByURLNorm returns a submission for the given url_norm within the dedup window (if any).
	GetRecentByURLNorm(ctx context.Context, urlNorm string) (*Submission, error)
	// CountRecentByIP returns the number of URLs submitted from the given IP in the rate limit window.
	CountRecentByIP(ctx context.Context, ip string) (int64, error)
	// Insert stores a new submission and returns the stored row.
	Insert(ctx context.Context, sub *Submission) (*Submission, error)
	// ListPendingValidation returns up to limit rows with status pending_validation (oldest first).
	ListPendingValidation(ctx context.Context, limit int) ([]*Submission, error)
	// CountPendingValidation returns the count of pending_validation rows.
	CountPendingValidation(ctx context.Context) (int64, error)
	// UpdateStatus updates status, rejection_reason, and validated_at for a submission.
	UpdateStatus(ctx context.Context, id int64, status string, rejectionReason *string, validatedAt *time.Time) error
	// UpdateAdminReview updates status and notes for a submission (admin PATCH), returns updated row.
	UpdateAdminReview(ctx context.Context, id int64, status string, notes *string) (*Submission, error)
	// List returns a paginated list of submissions with optional status filter.
	List(ctx context.Context, status *string, limit, offset int) ([]*Submission, error)
	// Count returns the total count of submissions with optional status filter.
	Count(ctx context.Context, status *string) (int64, error)
}

// NormalizeURL parses and normalises a raw URL, returning the original raw string
// and the normalised form. Returns an error if the URL is invalid.
//
// Normalisation rules:
//  1. Parse with net/url.Parse()
//  2. Validate: must have non-empty host, scheme must be "http" or "https"
//  3. Lowercase scheme and host
//  4. Strip fragment
//  5. Strip trailing slash (unless path is "" or "/")
//  6. Sort query parameters alphabetically
func NormalizeURL(raw string) (original, normalized string, err error) {
	if raw == "" {
		return "", "", errors.New("empty URL")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("unparseable URL: %w", err)
	}

	if u.Host == "" {
		return "", "", errors.New("missing host")
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", "", fmt.Errorf("unsupported scheme %q: must be http or https", u.Scheme)
	}

	// Normalise: lowercase scheme and host.
	u.Scheme = scheme
	u.Host = strings.ToLower(u.Host)

	// Strip fragment.
	u.Fragment = ""
	u.RawFragment = ""

	// Strip trailing slash unless path is exactly "/" or empty.
	if u.Path != "" && u.Path != "/" {
		u.Path = strings.TrimRight(u.Path, "/")
	}

	// Sort query parameters alphabetically.
	if u.RawQuery != "" {
		q := u.Query()
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		vals := url.Values{}
		for _, k := range keys {
			vals[k] = q[k]
		}
		u.RawQuery = vals.Encode()
	}

	return raw, u.String(), nil
}
