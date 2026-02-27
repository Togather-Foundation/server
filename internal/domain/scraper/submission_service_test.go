package scraper_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strings"

	"github.com/Togather-Foundation/server/internal/domain/scraper"
)

// --- inline mock repository ---

type mockSubmissionRepo struct {
	// recentByURLNorm: keyed by urlNorm, returns the Submission (or nil) + error.
	recentByURLNorm map[string]*scraper.Submission
	recentByIPCount int64
	recentByIPErr   error
	insertErr       error
	inserted        []*scraper.Submission
}

func newMockRepo() *mockSubmissionRepo {
	return &mockSubmissionRepo{
		recentByURLNorm: make(map[string]*scraper.Submission),
	}
}

func (m *mockSubmissionRepo) GetRecentByURLNorm(_ context.Context, urlNorm string) (*scraper.Submission, error) {
	return m.recentByURLNorm[urlNorm], nil
}

func (m *mockSubmissionRepo) CountRecentByIP(_ context.Context, _ string) (int64, error) {
	return m.recentByIPCount, m.recentByIPErr
}

func (m *mockSubmissionRepo) Insert(_ context.Context, sub *scraper.Submission) (*scraper.Submission, error) {
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	cp := *sub
	cp.ID = int64(len(m.inserted) + 1)
	now := time.Now()
	cp.SubmittedAt = now
	m.inserted = append(m.inserted, &cp)
	return &cp, nil
}

func (m *mockSubmissionRepo) ListPendingValidation(_ context.Context, _ int) ([]*scraper.Submission, error) {
	return nil, nil
}

func (m *mockSubmissionRepo) CountPendingValidation(_ context.Context) (int64, error) {
	return 0, nil
}

func (m *mockSubmissionRepo) UpdateStatus(_ context.Context, _ int64, _ string, _ *string, _ *time.Time) error {
	return nil
}

func (m *mockSubmissionRepo) UpdateAdminReview(_ context.Context, _ int64, _ string, _ *string) (*scraper.Submission, error) {
	return nil, nil
}

func (m *mockSubmissionRepo) List(_ context.Context, _ *string, _, _ int) ([]*scraper.Submission, error) {
	return nil, nil
}

func (m *mockSubmissionRepo) Count(_ context.Context, _ *string) (int64, error) {
	return 0, nil
}

// --- URL normalisation tests ---

func TestNormalizeURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantNorm    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "lowercase scheme",
			input:    "HTTPS://Example.COM/events",
			wantNorm: "https://example.com/events",
		},
		{
			name:     "lowercase host",
			input:    "https://EXAMPLE.COM/events",
			wantNorm: "https://example.com/events",
		},
		{
			name:     "strip fragment",
			input:    "https://example.com/events#section",
			wantNorm: "https://example.com/events",
		},
		{
			name:     "strip trailing slash on path",
			input:    "https://example.com/events/",
			wantNorm: "https://example.com/events",
		},
		{
			name:     "keep root slash",
			input:    "https://example.com/",
			wantNorm: "https://example.com/",
		},
		{
			name:     "sort query parameters",
			input:    "https://example.com/events?z=last&a=first&m=mid",
			wantNorm: "https://example.com/events?a=first&m=mid&z=last",
		},
		{
			name:     "stores original url",
			input:    "HTTPS://Example.COM/Events#frag",
			wantNorm: "https://example.com/Events",
		},
		{
			name:     "http scheme allowed",
			input:    "http://example.com/events",
			wantNorm: "http://example.com/events",
		},
		{
			name:        "empty string rejected",
			input:       "",
			wantErr:     true,
			errContains: "empty URL",
		},
		{
			name:        "missing host rejected",
			input:       "https:///path",
			wantErr:     true,
			errContains: "missing host",
		},
		{
			name:        "non-http scheme rejected",
			input:       "ftp://example.com/file",
			wantErr:     true,
			errContains: "unsupported scheme",
		},
		{
			name:    "unparseable URL rejected",
			input:   "://bad url\x00",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			original, norm, err := scraper.NormalizeURL(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (norm=%q)", norm)
				}
				if tc.errContains != "" && !containsStr(err.Error(), tc.errContains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.input != "" && original != tc.input {
				t.Errorf("original: got %q, want %q", original, tc.input)
			}
			if norm != tc.wantNorm {
				t.Errorf("normalized: got %q, want %q", norm, tc.wantNorm)
			}
		})
	}
}

// --- SubmissionService tests ---

func TestSubmissionService_FormatValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantMsg string // expected substring in Message
	}{
		{
			name:    "empty string rejected",
			url:     "",
			wantMsg: "Invalid URL",
		},
		{
			name:    "missing host rejected",
			url:     "https:///nohost",
			wantMsg: "Invalid URL",
		},
		{
			name:    "ftp scheme rejected",
			url:     "ftp://example.com",
			wantMsg: "Invalid URL",
		},
		{
			name:    "unparseable url rejected",
			url:     "://\x00bad",
			wantMsg: "Invalid URL",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newMockRepo()
			svc := scraper.NewSubmissionService(repo)

			results, err := svc.Submit(context.Background(), []string{tc.url}, "1.2.3.4")
			if err != nil {
				t.Fatalf("unexpected service error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			r := results[0]
			if r.Status != "rejected" {
				t.Errorf("status: got %q, want %q", r.Status, "rejected")
			}
			if !containsStr(r.Message, tc.wantMsg) {
				t.Errorf("message %q does not contain %q", r.Message, tc.wantMsg)
			}
		})
	}
}

func TestSubmissionService_DedupCheck(t *testing.T) {
	t.Parallel()

	repo := newMockRepo()
	// Pre-seed the mock: the normalised form of this URL exists in recent submissions.
	normalURL := "https://example.com/events"
	repo.recentByURLNorm[normalURL] = &scraper.Submission{
		ID:      99,
		URLNorm: normalURL,
		Status:  "pending_validation",
	}

	svc := scraper.NewSubmissionService(repo)
	results, err := svc.Submit(context.Background(), []string{"https://example.com/events"}, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Status != "duplicate" {
		t.Errorf("status: got %q, want %q", r.Status, "duplicate")
	}
	if r.Message != "Already submitted within 30 days" {
		t.Errorf("message: got %q, want %q", r.Message, "Already submitted within 30 days")
	}
	if len(repo.inserted) != 0 {
		t.Error("duplicate URL should not have been inserted")
	}
}

func TestSubmissionService_RateLimit(t *testing.T) {
	t.Parallel()

	repo := newMockRepo()
	repo.recentByIPCount = 5 // at or above the limit

	svc := scraper.NewSubmissionService(repo)
	_, err := svc.Submit(context.Background(), []string{"https://example.com/events"}, "1.2.3.4")
	if !errors.Is(err, scraper.ErrRateLimitExceeded) {
		t.Errorf("expected ErrRateLimitExceeded, got %v", err)
	}
}

func TestSubmissionService_AcceptedURL(t *testing.T) {
	t.Parallel()

	repo := newMockRepo()
	repo.recentByIPCount = 0

	svc := scraper.NewSubmissionService(repo)
	results, err := svc.Submit(context.Background(), []string{"https://example.com/events"}, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Status != "accepted" {
		t.Errorf("status: got %q, want %q", r.Status, "accepted")
	}
	if r.Message != "URL queued for review" {
		t.Errorf("message: got %q, want %q", r.Message, "URL queued for review")
	}
	if len(repo.inserted) != 1 {
		t.Errorf("expected 1 inserted row, got %d", len(repo.inserted))
	}
}

func TestSubmissionService_MixedBatch(t *testing.T) {
	t.Parallel()

	repo := newMockRepo()
	// Seed a duplicate.
	repo.recentByURLNorm["https://dup.example.com/events"] = &scraper.Submission{
		ID:      42,
		URLNorm: "https://dup.example.com/events",
	}

	svc := scraper.NewSubmissionService(repo)
	batch := []string{
		"https://new.example.com/events", // accepted
		"ftp://bad.example.com",          // rejected (bad scheme)
		"https://dup.example.com/events", // duplicate
		"",                               // rejected (empty)
	}

	results, err := svc.Submit(context.Background(), batch, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	wantStatuses := []string{"accepted", "rejected", "duplicate", "rejected"}
	for i, want := range wantStatuses {
		if results[i].Status != want {
			t.Errorf("result[%d] status: got %q, want %q (url=%q, msg=%q)",
				i, results[i].Status, want, results[i].URL, results[i].Message)
		}
	}
	// Only the first URL should have been inserted.
	if len(repo.inserted) != 1 {
		t.Errorf("expected 1 inserted row, got %d", len(repo.inserted))
	}
}

// --- helpers ---

// containsStr reports whether substr is within s (wraps strings.Contains for
// readability in test assertions).
func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TestSubmissionService_RateLimit_BatchBoundary verifies that a batch cannot
// exceed the per-IP quota even when count < rateLimitPerIP at batch start.
// Spec §Rate Limiting: "a batch of 3 uses 3 of the 5 slots".
func TestSubmissionService_RateLimit_BatchBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		existingCount   int64
		urls            []string
		wantAccepted    int
		wantRateLimited int
	}{
		{
			name:          "count=2, batch=5: accept 3, rate-limit 2",
			existingCount: 2,
			urls: []string{
				"https://a.example.com",
				"https://b.example.com",
				"https://c.example.com",
				"https://d.example.com",
				"https://e.example.com",
			},
			wantAccepted:    3,
			wantRateLimited: 2,
		},
		{
			name:          "count=4, batch=3: accept 1, rate-limit 2",
			existingCount: 4,
			urls: []string{
				"https://x.example.com",
				"https://y.example.com",
				"https://z.example.com",
			},
			wantAccepted:    1,
			wantRateLimited: 2,
		},
		{
			name:          "count=0, batch=5: accept all 5",
			existingCount: 0,
			urls: []string{
				"https://a.example.com",
				"https://b.example.com",
				"https://c.example.com",
				"https://d.example.com",
				"https://e.example.com",
			},
			wantAccepted:    5,
			wantRateLimited: 0,
		},
		{
			name:          "count=3, batch=3: accept 2, rate-limit 1",
			existingCount: 3,
			urls: []string{
				"https://p.example.com",
				"https://q.example.com",
				"https://r.example.com",
			},
			wantAccepted:    2,
			wantRateLimited: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newMockRepo()
			repo.recentByIPCount = tc.existingCount
			svc := scraper.NewSubmissionService(repo)

			results, err := svc.Submit(context.Background(), tc.urls, "1.2.3.4")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var accepted, rateLimited int
			for _, r := range results {
				switch r.Status {
				case "accepted":
					accepted++
				case "rate_limited":
					rateLimited++
				}
			}

			if accepted != tc.wantAccepted {
				t.Errorf("accepted: got %d, want %d", accepted, tc.wantAccepted)
			}
			if rateLimited != tc.wantRateLimited {
				t.Errorf("rate_limited: got %d, want %d", rateLimited, tc.wantRateLimited)
			}
			// Total inserted must not exceed quota.
			totalInserted := int64(len(repo.inserted))
			if tc.existingCount+totalInserted > 5 {
				t.Errorf("total submissions would exceed quota: existing=%d inserted=%d (max 5)",
					tc.existingCount, totalInserted)
			}
		})
	}
}
