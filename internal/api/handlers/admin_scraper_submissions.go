package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/scraper"
)

// AdminScraperSubmissionHandler handles admin management of URL submissions.
type AdminScraperSubmissionHandler struct {
	repo scraper.SubmissionRepository
	env  string
}

// NewAdminScraperSubmissionHandler creates a new AdminScraperSubmissionHandler.
func NewAdminScraperSubmissionHandler(repo scraper.SubmissionRepository, env string) *AdminScraperSubmissionHandler {
	return &AdminScraperSubmissionHandler{repo: repo, env: env}
}

// submissionResponse is the JSON representation of a submission for the admin API.
type submissionResponse struct {
	ID              int64      `json:"id"`
	URL             string     `json:"url"`
	URLNorm         string     `json:"url_norm"`
	SubmittedAt     time.Time  `json:"submitted_at"`
	SubmitterIP     string     `json:"submitter_ip"`
	Status          string     `json:"status"`
	RejectionReason *string    `json:"rejection_reason"`
	Notes           *string    `json:"notes"`
	ValidatedAt     *time.Time `json:"validated_at"`
}

func toSubmissionResponse(s *scraper.Submission) submissionResponse {
	return submissionResponse{
		ID:              s.ID,
		URL:             s.URL,
		URLNorm:         s.URLNorm,
		SubmittedAt:     s.SubmittedAt,
		SubmitterIP:     s.SubmitterIP,
		Status:          s.Status,
		RejectionReason: s.RejectionReason,
		Notes:           s.Notes,
		ValidatedAt:     s.ValidatedAt,
	}
}

// ListSubmissions handles GET /api/v1/admin/scraper/submissions.
// Query params: status (optional), limit (default 50, max 200), offset (default 0).
func (h *AdminScraperSubmissionHandler) ListSubmissions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Optional status filter.
	var statusFilter *string
	if s := q.Get("status"); s != "" {
		statusFilter = &s
	}

	// Parse limit (default 50, max 200).
	limit := 50
	if lStr := q.Get("limit"); lStr != "" {
		v, err := strconv.Atoi(lStr)
		if err != nil || v <= 0 || v > 200 {
			problem.Write(w, r, http.StatusBadRequest,
				"https://sel.events/problems/validation-error",
				"Invalid limit parameter: must be 1–200", nil, h.env)
			return
		}
		limit = v
	}

	// Parse offset (default 0).
	offset := 0
	if oStr := q.Get("offset"); oStr != "" {
		v, err := strconv.Atoi(oStr)
		if err != nil || v < 0 {
			problem.Write(w, r, http.StatusBadRequest,
				"https://sel.events/problems/validation-error",
				"Invalid offset parameter: must be 0 or greater", nil, h.env)
			return
		}
		offset = v
	}

	subs, err := h.repo.List(r.Context(), statusFilter, limit, offset)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError,
			"https://sel.events/problems/server-error",
			"Failed to list submissions", err, h.env)
		return
	}

	total, err := h.repo.Count(r.Context(), statusFilter)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError,
			"https://sel.events/problems/server-error",
			"Failed to count submissions", err, h.env)
		return
	}

	items := make([]submissionResponse, 0, len(subs))
	for _, s := range subs {
		items = append(items, toSubmissionResponse(s))
	}

	writeJSON(w, http.StatusOK, struct {
		Submissions []submissionResponse `json:"submissions"`
		Total       int64                `json:"total"`
		Limit       int                  `json:"limit"`
		Offset      int                  `json:"offset"`
	}{
		Submissions: items,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
	}, "application/json")
}

// UpdateSubmission handles PATCH /api/v1/admin/scraper/submissions/{id}.
// Accepts {"status": "processed"|"rejected", "notes": "..."}.
func (h *AdminScraperSubmissionHandler) UpdateSubmission(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Invalid submission ID", nil, h.env)
		return
	}

	var body struct {
		Status string  `json:"status"`
		Notes  *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Invalid request body", err, h.env)
		return
	}

	if body.Status != "processed" && body.Status != "rejected" {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"status must be 'processed' or 'rejected'", nil, h.env)
		return
	}

	updated, err := h.repo.UpdateAdminReview(r.Context(), id, body.Status, body.Notes)
	if err != nil {
		if errors.Is(err, scraper.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound,
				"https://sel.events/problems/not-found",
				"Submission not found", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError,
			"https://sel.events/problems/server-error",
			"Failed to update submission", err, h.env)
		return
	}

	writeJSON(w, http.StatusOK, toSubmissionResponse(updated), "application/json")
}
