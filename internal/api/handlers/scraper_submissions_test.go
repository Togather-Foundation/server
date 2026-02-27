package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Togather-Foundation/server/internal/domain/scraper"
)

// ----------------------------------------------------------------------------
// Mock submission repository
// ----------------------------------------------------------------------------

type mockSubmissionRepository struct {
	listResult    []*scraper.Submission
	listErr       error
	countResult   int64
	countErr      error
	updateResult  *scraper.Submission
	updateErr     error
	insertResult  *scraper.Submission
	insertErr     error
	recentByNorm  map[string]*scraper.Submission
	recentByIP    int64
	recentByIPErr error
}

func newMockSubmissionRepo() *mockSubmissionRepository {
	return &mockSubmissionRepository{
		recentByNorm: make(map[string]*scraper.Submission),
	}
}

func (m *mockSubmissionRepository) GetRecentByURLNorm(_ context.Context, urlNorm string) (*scraper.Submission, error) {
	return m.recentByNorm[urlNorm], nil
}

func (m *mockSubmissionRepository) CountRecentByIP(_ context.Context, _ string) (int64, error) {
	return m.recentByIP, m.recentByIPErr
}

func (m *mockSubmissionRepository) Insert(_ context.Context, sub *scraper.Submission) (*scraper.Submission, error) {
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	if m.insertResult != nil {
		return m.insertResult, nil
	}
	cp := *sub
	cp.ID = 1
	cp.SubmittedAt = time.Now()
	return &cp, nil
}

func (m *mockSubmissionRepository) ListPendingValidation(_ context.Context, _ int) ([]*scraper.Submission, error) {
	return nil, nil
}

func (m *mockSubmissionRepository) CountPendingValidation(_ context.Context) (int64, error) {
	return 0, nil
}

func (m *mockSubmissionRepository) UpdateStatus(_ context.Context, _ int64, _ string, _ *string, _ *time.Time) error {
	return nil
}

func (m *mockSubmissionRepository) UpdateAdminReview(_ context.Context, id int64, status string, notes *string) (*scraper.Submission, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	if m.updateResult != nil {
		return m.updateResult, nil
	}
	return &scraper.Submission{ID: id, Status: status, Notes: notes}, nil
}

func (m *mockSubmissionRepository) List(_ context.Context, _ *string, _, _ int) ([]*scraper.Submission, error) {
	return m.listResult, m.listErr
}

func (m *mockSubmissionRepository) Count(_ context.Context, _ *string) (int64, error) {
	return m.countResult, m.countErr
}

// ----------------------------------------------------------------------------
// Public handler tests: POST /api/v1/scraper/submissions
// ----------------------------------------------------------------------------

func TestScraperSubmission_ValidBatch(t *testing.T) {
	repo := newMockSubmissionRepo()
	svc := scraper.NewSubmissionService(repo)
	h := NewScraperSubmissionHandler(svc, "test")

	body := `{"urls":["https://example.com/events","https://other.org/cal"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scraper/submissions", bytes.NewBufferString(body))
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.SubmitURLs(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Results []scraper.SubmissionResult `json:"results"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Results, 2)
	for _, r := range resp.Results {
		assert.Equal(t, "accepted", r.Status)
		assert.Equal(t, "URL queued for review", r.Message)
	}
}

func TestScraperSubmission_EmptyURLs_400(t *testing.T) {
	repo := newMockSubmissionRepo()
	svc := scraper.NewSubmissionService(repo)
	h := NewScraperSubmissionHandler(svc, "test")

	body := `{"urls":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scraper/submissions", bytes.NewBufferString(body))
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.SubmitURLs(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScraperSubmission_TooManyURLs_400(t *testing.T) {
	repo := newMockSubmissionRepo()
	svc := scraper.NewSubmissionService(repo)
	h := NewScraperSubmissionHandler(svc, "test")

	urls := make([]string, 11)
	for i := range urls {
		urls[i] = "https://example.com/events"
	}
	bodyBytes, _ := json.Marshal(map[string]any{"urls": urls})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scraper/submissions", bytes.NewBuffer(bodyBytes))
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.SubmitURLs(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var prob struct {
		Title string `json:"title"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&prob))
	assert.Equal(t, "Maximum 10 URLs per request", prob.Title)
}

func TestScraperSubmission_RateLimitExceeded_429(t *testing.T) {
	repo := newMockSubmissionRepo()
	repo.recentByIP = 5 // at limit
	svc := scraper.NewSubmissionService(repo)
	h := NewScraperSubmissionHandler(svc, "test")

	body := `{"urls":["https://example.com/events"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scraper/submissions", bytes.NewBufferString(body))
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.SubmitURLs(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "86400", w.Header().Get("Retry-After"))
}

func TestScraperSubmission_MixedBatch_200(t *testing.T) {
	repo := newMockSubmissionRepo()
	// Pre-seed duplicate.
	repo.recentByNorm["https://dup.example.com/events"] = &scraper.Submission{ID: 99}
	svc := scraper.NewSubmissionService(repo)
	h := NewScraperSubmissionHandler(svc, "test")

	bodyPayload := map[string]any{
		"urls": []string{
			"https://new.example.com/events",
			"https://dup.example.com/events",
			"ftp://bad.example.com",
		},
	}
	bodyBytes, _ := json.Marshal(bodyPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scraper/submissions", bytes.NewBuffer(bodyBytes))
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.SubmitURLs(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Results []scraper.SubmissionResult `json:"results"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Results, 3)

	assert.Equal(t, "accepted", resp.Results[0].Status)
	assert.Equal(t, "duplicate", resp.Results[1].Status)
	assert.Equal(t, "rejected", resp.Results[2].Status)
}

func TestScraperSubmission_MalformedBody_400(t *testing.T) {
	repo := newMockSubmissionRepo()
	svc := scraper.NewSubmissionService(repo)
	h := NewScraperSubmissionHandler(svc, "test")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scraper/submissions", bytes.NewBufferString(`{not json`))
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.SubmitURLs(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ----------------------------------------------------------------------------
// Admin handler tests
// ----------------------------------------------------------------------------

func TestAdminScraperSubmission_List(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := newMockSubmissionRepo()
	repo.listResult = []*scraper.Submission{
		{ID: 1, URL: "https://example.com/events", URLNorm: "https://example.com/events", SubmittedAt: now, SubmitterIP: "1.2.3.4", Status: "pending"},
	}
	repo.countResult = 1

	h := NewAdminScraperSubmissionHandler(repo, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scraper/submissions", nil)
	w := httptest.NewRecorder()

	h.ListSubmissions(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Submissions []submissionResponse `json:"submissions"`
		Total       int64                `json:"total"`
		Limit       int                  `json:"limit"`
		Offset      int                  `json:"offset"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Submissions, 1)
	assert.Equal(t, int64(1), resp.Total)
	assert.Equal(t, 50, resp.Limit)
	assert.Equal(t, 0, resp.Offset)
	assert.Equal(t, "https://example.com/events", resp.Submissions[0].URL)
}

func TestAdminScraperSubmission_List_StatusFilter(t *testing.T) {
	repo := newMockSubmissionRepo()
	repo.listResult = []*scraper.Submission{}
	repo.countResult = 0

	h := NewAdminScraperSubmissionHandler(repo, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scraper/submissions?status=pending&limit=10&offset=20", nil)
	w := httptest.NewRecorder()

	h.ListSubmissions(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 10, resp.Limit)
	assert.Equal(t, 20, resp.Offset)
}

func TestAdminScraperSubmission_List_InvalidLimit_400(t *testing.T) {
	repo := newMockSubmissionRepo()
	h := NewAdminScraperSubmissionHandler(repo, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scraper/submissions?limit=999", nil)
	w := httptest.NewRecorder()

	h.ListSubmissions(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminScraperSubmission_Update_OK(t *testing.T) {
	notes := "Created source config"
	repo := newMockSubmissionRepo()
	repo.updateResult = &scraper.Submission{
		ID:     42,
		Status: "processed",
		Notes:  &notes,
	}

	h := NewAdminScraperSubmissionHandler(repo, "test")

	body := `{"status":"processed","notes":"Created source config"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/scraper/submissions/42", bytes.NewBufferString(body))
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	h.UpdateSubmission(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp submissionResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, int64(42), resp.ID)
	assert.Equal(t, "processed", resp.Status)
}

func TestAdminScraperSubmission_Update_InvalidStatus_400(t *testing.T) {
	repo := newMockSubmissionRepo()
	h := NewAdminScraperSubmissionHandler(repo, "test")

	body := `{"status":"approved"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/scraper/submissions/1", bytes.NewBufferString(body))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.UpdateSubmission(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminScraperSubmission_Update_InvalidID_400(t *testing.T) {
	repo := newMockSubmissionRepo()
	h := NewAdminScraperSubmissionHandler(repo, "test")

	body := `{"status":"rejected"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/scraper/submissions/abc", bytes.NewBufferString(body))
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()

	h.UpdateSubmission(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminScraperSubmission_Update_RepoError_500(t *testing.T) {
	repo := newMockSubmissionRepo()
	repo.updateErr = errors.New("db error")
	h := NewAdminScraperSubmissionHandler(repo, "test")

	body := `{"status":"rejected"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/scraper/submissions/1", bytes.NewBufferString(body))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.UpdateSubmission(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
