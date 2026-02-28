package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/scraper"
)

// submissionMaxBodyBytes is the maximum accepted request body size for
// POST /api/v1/scraper/submissions. 1 MiB is generous for a payload of up to
// 10 URLs; anything larger is almost certainly an attack or misconfigured client.
const submissionMaxBodyBytes = 1 << 20 // 1 MiB

// ScraperSubmissionHandler handles the public URL submission endpoint.
type ScraperSubmissionHandler struct {
	service *scraper.SubmissionService
	env     string
}

// NewScraperSubmissionHandler creates a new ScraperSubmissionHandler.
func NewScraperSubmissionHandler(service *scraper.SubmissionService, env string) *ScraperSubmissionHandler {
	return &ScraperSubmissionHandler{service: service, env: env}
}

// SubmitURLs handles POST /api/v1/scraper/submissions.
// No authentication required — uses public rate limit middleware.
func (h *ScraperSubmissionHandler) SubmitURLs(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to prevent DoS via oversized payloads (srv-puwjd).
	r.Body = http.MaxBytesReader(w, r.Body, submissionMaxBodyBytes)

	var body struct {
		URLs []string `json:"urls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// Distinguish body-too-large from generic decode errors.
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			problem.Write(w, r, http.StatusRequestEntityTooLarge,
				"https://sel.events/problems/request-too-large",
				"Request body too large", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Invalid request body", err, h.env)
		return
	}

	if len(body.URLs) == 0 {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"urls is required and must be non-empty", nil, h.env)
		return
	}

	if len(body.URLs) > 10 {
		problem.Write(w, r, http.StatusBadRequest,
			"https://sel.events/problems/validation-error",
			"Maximum 10 URLs per request", nil, h.env)
		return
	}

	ip := extractClientIP(r)

	results, err := h.service.Submit(r.Context(), body.URLs, ip)
	if err != nil {
		if errors.Is(err, scraper.ErrRateLimitExceeded) {
			w.Header().Set("Retry-After", "86400")
			problem.Write(w, r, http.StatusTooManyRequests,
				"https://sel.events/problems/rate-limit-exceeded",
				"Rate limit exceeded", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError,
			"https://sel.events/problems/server-error",
			"Failed to process URL submissions", err, h.env)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Results []scraper.SubmissionResult `json:"results"`
	}{Results: results}, "application/json")
}
