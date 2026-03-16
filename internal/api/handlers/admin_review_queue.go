package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/jackc/pgx/v5"
)

// AdminReviewQueueHandler handles admin review queue operations
type AdminReviewQueueHandler struct {
	Repository   events.Repository
	AdminService *events.AdminService
	AuditLogger  *audit.Logger
	Env          string
	BaseURL      string
}

// NewAdminReviewQueueHandler creates a new review queue handler
func NewAdminReviewQueueHandler(repo events.Repository, adminService *events.AdminService, auditLogger *audit.Logger, env string, baseURL string) *AdminReviewQueueHandler {
	return &AdminReviewQueueHandler{
		Repository:   repo,
		AdminService: adminService,
		AuditLogger:  auditLogger,
		Env:          env,
		BaseURL:      baseURL,
	}
}

// reviewQueueBase contains fields shared by both list (reviewQueueItem) and detail
// (reviewQueueDetail) API responses. Embedded via struct embedding — do not add fields
// here that duplicate field names in either outer struct, as this causes silent JSON
// serialization issues (both fields get dropped by encoding/json).
type reviewQueueBase struct {
	ID                 int                        `json:"id"`
	EventID            string                     `json:"eventId"`
	Status             string                     `json:"status"`
	Warnings           []events.ValidationWarning `json:"warnings"`
	CreatedAt          time.Time                  `json:"createdAt"`
	ReviewedBy         *string                    `json:"reviewedBy,omitempty"`
	ReviewedAt         *time.Time                 `json:"reviewedAt,omitempty"`
	RejectionReason    *string                    `json:"rejectionReason,omitempty"`
	DuplicateOfEventID *string                    `json:"duplicateOfEventUlid,omitempty"`
}

// reviewQueueItem represents a single item in the review queue list.
// Embeds reviewQueueBase for shared fields.
type reviewQueueItem struct {
	reviewQueueBase
	EventName      string     `json:"eventName,omitempty"`
	EventStartTime *time.Time `json:"eventStartTime,omitempty"`
	EventEndTime   *time.Time `json:"eventEndTime,omitempty"`
}

// reviewQueueDetail represents detailed review information.
// Embeds reviewQueueBase for shared fields.
type reviewQueueDetail struct {
	reviewQueueBase
	Original    map[string]any `json:"original"`
	Normalized  map[string]any `json:"normalized"`
	Changes     []changeDetail `json:"changes"`
	ReviewNotes *string        `json:"reviewNotes,omitempty"`
}

// changeDetail describes a specific change made during normalization
type changeDetail struct {
	Field     string `json:"field"`
	Original  any    `json:"original"`
	Corrected any    `json:"corrected"`
	Reason    string `json:"reason"`
}

// ListReviewQueue returns a paginated list of events pending review with quality warnings.
// It handles GET /api/v1/admin/review-queue and supports filtering by status (pending, approved, rejected, merged).
// Query parameters: status (default: pending), limit (1-100, default: 50), cursor (pagination).
func (h *AdminReviewQueueHandler) ListReviewQueue(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Repository == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Parse query parameters
	statusParam := r.URL.Query().Get("status")
	var status *string
	if statusParam != "" {
		status = &statusParam
	} else {
		defaultStatus := "pending"
		status = &defaultStatus
	}

	// Parse and validate limit parameter (default: 50, min: 1, max: 100)
	limit := 50
	limitStr := r.URL.Query().Get("limit")
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit < 1 || parsedLimit > 100 {
			// Invalid limit: use default
			limit = 50
		} else {
			limit = parsedLimit
		}
	}

	cursor := r.URL.Query().Get("cursor")
	var cursorID *int
	if cursor != "" {
		if parsedCursor, err := strconv.Atoi(cursor); err == nil && parsedCursor > 0 {
			cursorID = &parsedCursor
		}
	}

	// Build filters
	filters := events.ReviewQueueFilters{
		Status:     status,
		Limit:      limit,
		NextCursor: cursorID,
	}

	// Fetch review queue entries
	result, err := h.Repository.ListReviewQueue(r.Context(), filters)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to list review queue", fmt.Errorf("list review queue with status=%v limit=%d: %w", status, limit, err), h.Env)
		return
	}

	// Build response items
	items := make([]reviewQueueItem, 0, len(result.Entries))
	for _, review := range result.Entries {
		item, err := buildReviewQueueItem(review)
		if err != nil {
			slog.ErrorContext(r.Context(), "ListReviewQueue: skipping review item due to build error",
				slog.Int("review_id", review.ID),
				slog.String("error", err.Error()))
			continue
		}
		items = append(items, item)
	}

	// Calculate next cursor
	nextCursor := ""
	if result.NextCursor != nil {
		nextCursor = strconv.Itoa(*result.NextCursor)
	}

	writeJSON(w, http.StatusOK, listResponse{
		Items:      items,
		NextCursor: nextCursor,
		Total:      result.TotalCount,
	}, "application/json")
}

// GetReviewQueueEntry returns detailed review information for a specific queue entry.
// It handles GET /api/v1/admin/review-queue/:id and includes original payload, normalized payload,
// validation warnings, and a diff of changes made during normalization.
func (h *AdminReviewQueueHandler) GetReviewQueueEntry(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Repository == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate ID
	idStr := r.PathValue("id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing review ID", nil, h.Env)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid review ID", nil, h.Env)
		return
	}

	// Fetch review entry
	review, err := h.Repository.GetReviewQueueEntry(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", fmt.Errorf("get review queue entry id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", fmt.Errorf("get review queue entry id=%d: %w", id, err), h.Env)
		return
	}

	// Build detailed response
	detail, err := buildReviewQueueDetail(*review)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", fmt.Errorf("build review queue detail for id=%d: %w", id, err), h.Env)
		return
	}

	writeJSON(w, http.StatusOK, detail, "application/json")
}

// ApproveReview marks a review entry as approved and publishes the associated event.
// It handles POST /api/v1/admin/review-queue/:id/approve and transitions the event lifecycle
// state to published. Accepts optional review notes in the request body.
func (h *AdminReviewQueueHandler) ApproveReview(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Repository == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate ID
	idStr := r.PathValue("id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing review ID", nil, h.Env)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid review ID", nil, h.Env)
		return
	}

	// Parse request body
	var req struct {
		Notes               string `json:"notes,omitempty"`
		RecordNotDuplicates bool   `json:"record_not_duplicates,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.Env)
		return
	}

	// Get reviewer identity from context (set by auth middleware)
	reviewedBy := getUserFromContext(r)

	// Fetch the review entry to get event ID
	review, err := h.Repository.GetReviewQueueEntry(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", fmt.Errorf("approve review: get review queue entry id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", fmt.Errorf("approve review: get review queue entry id=%d: %w", id, err), h.Env)
		return
	}

	// Only pending reviews can be approved
	if review.Status != "pending" {
		problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/conflict", fmt.Sprintf("Review entry has already been %s", review.Status), nil, h.Env)
		return
	}

	// Update event lifecycle state to published
	eventULID := review.EventULID

	// Mark review as approved
	notes := &req.Notes
	if req.Notes == "" {
		notes = nil
	}

	// Atomically publish event AND approve review in a single transaction
	updatedReview, err := h.AdminService.ApproveEventWithReview(r.Context(), eventULID, id, reviewedBy, notes)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.approve", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":    err.Error(),
				"event_id": eventULID,
			})
		}

		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", fmt.Errorf("approve review id=%d: %w", id, err), h.Env)
			return
		}
		if errors.Is(err, events.ErrConflict) {
			problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/conflict", "Review entry has already been processed", fmt.Errorf("approve review id=%d: %w", id, err), h.Env)
			return
		}
		if errors.Is(err, events.ErrEventDeleted) {
			problem.Write(w, r, http.StatusGone, "https://sel.events/problems/event-deleted", "Event has been deleted", fmt.Errorf("approve review id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to approve review", fmt.Errorf("approve review id=%d event=%s: %w", id, eventULID, err), h.Env)
		return
	}

	// If requested, record duplicate warning pairs as not-duplicates so future
	// ingestion won't re-flag them. This is used by the "Not a Duplicate" action
	// which approves/publishes the event while acknowledging the duplicate warning.
	if req.RecordNotDuplicates {
		h.recordNotDuplicatesFromWarnings(r.Context(), review, reviewedBy)
	}

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.review.approve", "review", strconv.Itoa(id), "success", map[string]string{
			"event_id":                eventULID,
			"reviewed_by":             reviewedBy,
			"not_duplicates_recorded": strconv.FormatBool(req.RecordNotDuplicates),
		})
	}

	// Build response
	detail, err := buildReviewQueueDetail(*updatedReview)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", fmt.Errorf("approve review id=%d: build detail: %w", id, err), h.Env)
		return
	}

	writeJSON(w, http.StatusOK, detail, "application/json")
}

// RejectReview marks a review entry as rejected and deletes the associated event.
// It handles POST /api/v1/admin/review-queue/:id/reject and transitions the event lifecycle
// state to deleted. Requires a rejection reason in the request body.
func (h *AdminReviewQueueHandler) RejectReview(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Repository == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate ID
	idStr := r.PathValue("id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing review ID", nil, h.Env)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid review ID", nil, h.Env)
		return
	}

	// Parse request body
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.Env)
		return
	}

	if req.Reason == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Rejection reason is required", nil, h.Env)
		return
	}

	// Get reviewer identity from context
	reviewedBy := getUserFromContext(r)

	// Fetch the review entry to get event ID
	review, err := h.Repository.GetReviewQueueEntry(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", fmt.Errorf("reject review: get review queue entry id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", fmt.Errorf("reject review: get review queue entry id=%d: %w", id, err), h.Env)
		return
	}

	// Only pending reviews can be rejected
	if review.Status != "pending" {
		problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/conflict", fmt.Sprintf("Review entry has already been %s", review.Status), nil, h.Env)
		return
	}

	// Update event lifecycle state to deleted
	eventULID := review.EventULID

	// Atomically delete event AND reject review in a single transaction
	updatedReview, err := h.AdminService.RejectEventWithReview(r.Context(), eventULID, id, reviewedBy, req.Reason)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.reject", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":    err.Error(),
				"event_id": eventULID,
				"reason":   req.Reason,
			})
		}

		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", fmt.Errorf("reject review id=%d: %w", id, err), h.Env)
			return
		}
		if errors.Is(err, events.ErrConflict) {
			problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/conflict", "Review entry has already been processed", fmt.Errorf("reject review id=%d: %w", id, err), h.Env)
			return
		}
		if errors.Is(err, events.ErrEventDeleted) {
			problem.Write(w, r, http.StatusGone, "https://sel.events/problems/event-deleted", "Event has been deleted", fmt.Errorf("reject review id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to reject review", fmt.Errorf("reject review id=%d event=%s: %w", id, eventULID, err), h.Env)
		return
	}

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.review.reject", "review", strconv.Itoa(id), "success", map[string]string{
			"event_id":    eventULID,
			"reviewed_by": reviewedBy,
			"reason":      req.Reason,
		})
	}

	// Record not-duplicate decisions: if the review had potential_duplicate warnings,
	// extract the matched event ULIDs and record them so future ingestion won't re-flag.
	h.recordNotDuplicatesFromWarnings(r.Context(), review, reviewedBy)

	// Build response
	detail, err := buildReviewQueueDetail(*updatedReview)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", fmt.Errorf("reject review id=%d: build detail: %w", id, err), h.Env)
		return
	}

	writeJSON(w, http.StatusOK, detail, "application/json")
}

// FixReview applies manual corrections to a review entry and publishes the event.
// It handles POST /api/v1/admin/review-queue/:id/fix and accepts date corrections in the
// request body (startDate, endDate). After applying fixes, it approves the review and
// publishes the event. Note: Full correction workflow is planned for future implementation.
func (h *AdminReviewQueueHandler) FixReview(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Repository == nil || h.AdminService == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate ID
	idStr := r.PathValue("id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing review ID", nil, h.Env)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid review ID", nil, h.Env)
		return
	}

	// Parse request body
	var req struct {
		Corrections struct {
			StartDate *time.Time `json:"startDate,omitempty"`
			EndDate   *time.Time `json:"endDate,omitempty"`
		} `json:"corrections"`
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.Env)
		return
	}

	if req.Corrections.StartDate == nil && req.Corrections.EndDate == nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "At least one correction is required", nil, h.Env)
		return
	}

	// Get reviewer identity from context
	reviewedBy := getUserFromContext(r)

	// Fetch the review entry to get event ID
	review, err := h.Repository.GetReviewQueueEntry(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", fmt.Errorf("fix review: get review queue entry id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", fmt.Errorf("fix review: get review queue entry id=%d: %w", id, err), h.Env)
		return
	}

	// Only pending reviews can be fixed
	if review.Status != "pending" {
		problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/conflict", fmt.Sprintf("Review entry has already been %s", review.Status), nil, h.Env)
		return
	}

	// Update event with corrected dates
	eventULID := review.EventULID

	// Build notes for the correction
	notes := req.Notes
	if notes == "" {
		notes = "Manually corrected dates"
	}
	if req.Corrections.StartDate != nil {
		notes += fmt.Sprintf(" (startDate: %s)", req.Corrections.StartDate.Format(time.RFC3339))
	}
	if req.Corrections.EndDate != nil {
		notes += fmt.Sprintf(" (endDate: %s)", req.Corrections.EndDate.Format(time.RFC3339))
	}
	notesPtr := &notes

	// Atomically fix dates, publish event, AND approve review in a single transaction
	updatedReview, err := h.AdminService.FixAndApproveEventWithReview(r.Context(), eventULID, id, reviewedBy, notesPtr, req.Corrections.StartDate, req.Corrections.EndDate)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.fix", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":    err.Error(),
				"event_id": eventULID,
			})
		}

		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", fmt.Errorf("fix review id=%d: %w", id, err), h.Env)
			return
		}
		if errors.Is(err, events.ErrConflict) {
			problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/conflict", "Review entry has already been processed", fmt.Errorf("fix review id=%d: %w", id, err), h.Env)
			return
		}
		if errors.Is(err, events.ErrEventDeleted) {
			problem.Write(w, r, http.StatusGone, "https://sel.events/problems/event-deleted", "Event has been deleted", fmt.Errorf("fix review id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fix and approve review", fmt.Errorf("fix review id=%d event=%s: %w", id, eventULID, err), h.Env)
		return
	}

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.review.fix", "review", strconv.Itoa(id), "success", map[string]string{
			"event_id":    eventULID,
			"reviewed_by": reviewedBy,
			"notes":       notes,
		})
	}

	// Build response
	detail, err := buildReviewQueueDetail(*updatedReview)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", fmt.Errorf("fix review id=%d: build detail: %w", id, err), h.Env)
		return
	}

	writeJSON(w, http.StatusOK, detail, "application/json")
}

// MergeReview marks a review entry as merged, merging the duplicate event into a primary event.
// It handles POST /api/v1/admin/review-queue/:id/merge and uses AdminService.MergeEvents
// to perform the actual merge. Requires primary_event_ulid in the request body.
func (h *AdminReviewQueueHandler) MergeReview(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Repository == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	// Extract and validate ID
	idStr := r.PathValue("id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing review ID", nil, h.Env)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid review ID", nil, h.Env)
		return
	}

	// Parse request body — requires the ULID of the primary event to merge into
	var req struct {
		PrimaryEventULID string `json:"primary_event_ulid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.Env)
		return
	}
	if req.PrimaryEventULID == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "primary_event_ulid is required", nil, h.Env)
		return
	}

	// Get reviewer identity from context
	reviewedBy := getUserFromContext(r)

	// Fetch the review entry to get the duplicate event's ULID
	review, err := h.Repository.GetReviewQueueEntry(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", fmt.Errorf("merge review: get review queue entry id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", fmt.Errorf("merge review: get review queue entry id=%d: %w", id, err), h.Env)
		return
	}

	duplicateULID := review.EventULID

	// Prevent merging an event into itself
	if duplicateULID == req.PrimaryEventULID {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Cannot merge event into itself", nil, h.Env)
		return
	}

	// Perform event merge AND review status update atomically in a single transaction
	updatedReview, err := h.AdminService.MergeEventsWithReview(r.Context(), events.MergeEventsParams{
		PrimaryULID:   req.PrimaryEventULID,
		DuplicateULID: duplicateULID,
	}, id, reviewedBy)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.merge", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":           err.Error(),
				"duplicate_event": duplicateULID,
				"primary_event":   req.PrimaryEventULID,
			})
		}

		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event or review entry not found", fmt.Errorf("merge review id=%d: merge events %s->%s: %w", id, duplicateULID, req.PrimaryEventULID, err), h.Env)
			return
		}
		if errors.Is(err, events.ErrCannotMergeSameEvent) {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Cannot merge event into itself", err, h.Env)
			return
		}
		if errors.Is(err, events.ErrConflict) {
			problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/conflict", "Review entry has already been processed", fmt.Errorf("merge review id=%d: %w", id, err), h.Env)
			return
		}
		if errors.Is(err, events.ErrEventDeleted) {
			problem.Write(w, r, http.StatusGone, "https://sel.events/problems/event-deleted", "One or more events have been deleted", fmt.Errorf("merge review id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to merge events", fmt.Errorf("merge review id=%d: merge events %s->%s: %w", id, duplicateULID, req.PrimaryEventULID, err), h.Env)
		return
	}

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.review.merge", "review", strconv.Itoa(id), "success", map[string]string{
			"duplicate_event": duplicateULID,
			"primary_event":   req.PrimaryEventULID,
			"reviewed_by":     reviewedBy,
		})
	}

	// Build response
	detail, err := buildReviewQueueDetail(*updatedReview)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", fmt.Errorf("merge review id=%d: build detail: %w", id, err), h.Env)
		return
	}

	writeJSON(w, http.StatusOK, detail, "application/json")
}

// AddOccurrenceReview adds the review entry's event as a new occurrence on a target
// recurring-series event, soft-deletes the absorbed event, and marks the review
// as merged — all atomically.
// It handles POST /api/v1/admin/review-queue/:id/add-occurrence.
//
// Two flavours are dispatched automatically based on the warning type in the review:
//
//   - potential_duplicate (forward path): the review's own event is the source to be
//     absorbed into the target series supplied in target_event_ulid.
//
//   - near_duplicate_of_new_event (inverted path): the review entry sits on the
//     existing series event; the newly-ingested counterpart (stored in the review's
//     duplicate_of_event_id column) is the source to be absorbed into the existing
//     series.  target_event_ulid is NOT required for this path — the target is
//     derived from the review entry itself.  Any provided target_event_ulid is ignored.
func (h *AdminReviewQueueHandler) AddOccurrenceReview(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Repository == nil || h.AdminService == nil {
		env := ""
		if h != nil {
			env = h.Env
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, env)
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing review ID", nil, h.Env)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid review ID", nil, h.Env)
		return
	}

	var req struct {
		TargetEventULID string `json:"target_event_ulid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.Env)
		return
	}

	reviewedBy := getUserFromContext(r)

	// Peek at the review entry to determine which dispatch path to take.
	// We need to know whether this is a near_duplicate_of_new_event before
	// deciding whether target_event_ulid is required.
	review, err := h.Repository.GetReviewQueueEntry(r.Context(), id)
	if err != nil {
		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", fmt.Errorf("add-occurrence review: get entry id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", fmt.Errorf("add-occurrence review: get entry id=%d: %w", id, err), h.Env)
		return
	}

	isNearDupPath := reviewHasNearDupNewEventWarning(review)

	// Reject when the review has BOTH warning types.  The two dispatch paths have
	// inverted source/target semantics; silently picking one would either absorb the
	// wrong event or ignore the supplied target_event_ulid.  Require the caller to
	// resolve which path applies (e.g. by splitting the review or clearing warnings).
	if isNearDupPath && reviewHasPotentialDuplicateWarning(review) {
		problem.Write(w, r, http.StatusUnprocessableEntity,
			"https://sel.events/problems/ambiguous-occurrence-dispatch",
			"Review entry has both potential_duplicate and near_duplicate_of_new_event warnings; add-occurrence path is ambiguous",
			events.ErrAmbiguousOccurrenceDispatch, h.Env)
		return
	}

	if isNearDupPath {
		// Inverted path: absorb the newly-ingested event (DuplicateOfEventULID) into
		// the existing series (EventULID).  target_event_ulid is not needed.
		h.addOccurrenceNearDupPath(w, r, id, review, reviewedBy)
		return
	}

	// Forward path (potential_duplicate): target_event_ulid is required.
	if req.TargetEventULID == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "target_event_ulid is required", nil, h.Env)
		return
	}
	if err := ids.ValidateULID(req.TargetEventULID); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "target_event_ulid is not a valid ULID", err, h.Env)
		return
	}

	h.addOccurrenceForwardPath(w, r, id, req.TargetEventULID, reviewedBy)
}

// addOccurrenceForwardPath handles the potential_duplicate case:
// the review's own event is absorbed into the provided target series.
func (h *AdminReviewQueueHandler) addOccurrenceForwardPath(w http.ResponseWriter, r *http.Request, id int, targetEventULID, reviewedBy string) {
	updatedReview, err := h.AdminService.AddOccurrenceFromReview(r.Context(), id, targetEventULID, reviewedBy)
	if err != nil {
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.add-occurrence", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":        err.Error(),
				"target_event": targetEventULID,
				"path":         "forward",
			})
		}
		h.writeAddOccurrenceError(w, r, id, targetEventULID, err)
		return
	}

	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.review.add-occurrence", "review", strconv.Itoa(id), "success", map[string]string{
			"target_event": targetEventULID,
			"reviewed_by":  reviewedBy,
			"path":         "forward",
		})
	}

	detail, err := buildReviewQueueDetail(*updatedReview)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", fmt.Errorf("add-occurrence review id=%d: build detail: %w", id, err), h.Env)
		return
	}
	writeJSON(w, http.StatusOK, addOccurrenceResponse{reviewQueueDetail: detail, TargetEventULID: targetEventULID}, "application/json")
}

// addOccurrenceNearDupPath handles the near_duplicate_of_new_event case:
// the newly-ingested event (DuplicateOfEventULID) is absorbed into the
// existing series (EventULID).
func (h *AdminReviewQueueHandler) addOccurrenceNearDupPath(w http.ResponseWriter, r *http.Request, id int, review *events.ReviewQueueEntry, reviewedBy string) {
	targetEventULID := review.EventULID // existing series is the target

	updatedReview, _, err := h.AdminService.AddOccurrenceFromReviewNearDup(r.Context(), id, reviewedBy)
	if err != nil {
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.add-occurrence", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":        err.Error(),
				"target_event": targetEventULID,
				"path":         "near-dup",
			})
		}
		h.writeAddOccurrenceError(w, r, id, targetEventULID, err)
		return
	}

	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.review.add-occurrence", "review", strconv.Itoa(id), "success", map[string]string{
			"target_event": targetEventULID,
			"reviewed_by":  reviewedBy,
			"path":         "near-dup",
		})
	}

	detail, err := buildReviewQueueDetail(*updatedReview)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", fmt.Errorf("add-occurrence near-dup review id=%d: build detail: %w", id, err), h.Env)
		return
	}
	writeJSON(w, http.StatusOK, addOccurrenceResponse{reviewQueueDetail: detail, TargetEventULID: targetEventULID}, "application/json")
}

// addOccurrenceResponse wraps ReviewQueueDetail with an explicit targetEventUlid
// field so the frontend can navigate directly to the target series after a
// successful add-as-occurrence action, regardless of which dispatch path was used.
type addOccurrenceResponse struct {
	reviewQueueDetail
	TargetEventULID string `json:"targetEventUlid"`
}

// writeAddOccurrenceError writes the appropriate HTTP error for an add-occurrence failure.
func (h *AdminReviewQueueHandler) writeAddOccurrenceError(w http.ResponseWriter, r *http.Request, id int, targetEventULID string, err error) {
	if errors.Is(err, events.ErrNotFound) {
		problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry or target event not found", fmt.Errorf("add-occurrence review id=%d: %w", id, err), h.Env)
		return
	}
	if errors.Is(err, events.ErrCannotMergeSameEvent) {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Review event and target event are the same", err, h.Env)
		return
	}
	if errors.Is(err, events.ErrOccurrenceOverlap) {
		problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/occurrence-overlap", "New occurrence overlaps an existing occurrence on the target event", err, h.Env)
		return
	}
	if errors.Is(err, events.ErrEventDeleted) {
		problem.Write(w, r, http.StatusGone, "https://sel.events/problems/event-deleted", "Target event has been deleted", err, h.Env)
		return
	}
	if errors.Is(err, events.ErrConflict) {
		problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/conflict", "Review entry is not in pending status", err, h.Env)
		return
	}
	if errors.Is(err, events.ErrInvalidUpdateParams) {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Review entry is missing required duplicate event reference", err, h.Env)
		return
	}
	if errors.Is(err, events.ErrAmbiguousOccurrenceSource) {
		problem.Write(w, r, http.StatusUnprocessableEntity, "https://sel.events/problems/ambiguous-occurrence-source", "Source event has multiple occurrences and cannot be absorbed unambiguously; resolve the source event first", err, h.Env)
		return
	}
	if errors.Is(err, events.ErrZeroOccurrenceSource) {
		problem.Write(w, r, http.StatusUnprocessableEntity, "https://sel.events/problems/zero-occurrence-source", "Source event has no occurrences; cannot determine which occurrence to absorb", err, h.Env)
		return
	}
	if errors.Is(err, events.ErrAmbiguousOccurrenceDispatch) {
		problem.Write(w, r, http.StatusUnprocessableEntity,
			"https://sel.events/problems/ambiguous-occurrence-dispatch",
			"Review entry has both potential_duplicate and near_duplicate_of_new_event warnings; add-occurrence path is ambiguous",
			err, h.Env)
		return
	}
	if errors.Is(err, events.ErrWrongOccurrencePath) {
		problem.Write(w, r, http.StatusUnprocessableEntity,
			"https://sel.events/problems/ambiguous-occurrence-dispatch",
			"Review entry warnings changed; add-occurrence path is no longer valid — retry the request",
			err, h.Env)
		return
	}
	problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to add occurrence", fmt.Errorf("add-occurrence review id=%d target=%s: %w", id, targetEventULID, err), h.Env)
}

// reviewHasNearDupNewEventWarning returns true if the review entry has at least one
// near_duplicate_of_new_event warning. Used to select the inverted add-occurrence path.
func reviewHasNearDupNewEventWarning(review *events.ReviewQueueEntry) bool {
	if review == nil || len(review.Warnings) == 0 {
		return false
	}
	var warnings []events.ValidationWarning
	if err := json.Unmarshal(review.Warnings, &warnings); err != nil {
		return false
	}
	for _, w := range warnings {
		if w.Code == "near_duplicate_of_new_event" {
			return true
		}
	}
	return false
}

// reviewHasPotentialDuplicateWarning returns true if the review entry has at least one
// potential_duplicate warning. Used together with reviewHasNearDupNewEventWarning to
// detect the ambiguous "both warnings" case.
func reviewHasPotentialDuplicateWarning(review *events.ReviewQueueEntry) bool {
	if review == nil || len(review.Warnings) == 0 {
		return false
	}
	var warnings []events.ValidationWarning
	if err := json.Unmarshal(review.Warnings, &warnings); err != nil {
		return false
	}
	for _, w := range warnings {
		if w.Code == "potential_duplicate" {
			return true
		}
	}
	return false
}

// Helper functions

// populateReviewQueueBase extracts the shared fields from a ReviewQueueEntry into
// a reviewQueueBase. Called by both buildReviewQueueItem and buildReviewQueueDetail.
func populateReviewQueueBase(review events.ReviewQueueEntry, warnings []events.ValidationWarning) reviewQueueBase {
	base := reviewQueueBase{
		ID:        review.ID,
		EventID:   review.EventULID,
		Status:    review.Status,
		Warnings:  warnings,
		CreatedAt: review.CreatedAt,
	}
	if review.ReviewedBy != nil {
		base.ReviewedBy = review.ReviewedBy
	}
	if review.ReviewedAt != nil {
		base.ReviewedAt = review.ReviewedAt
	}
	if review.RejectionReason != nil {
		base.RejectionReason = review.RejectionReason
	}
	if review.DuplicateOfEventULID != nil {
		base.DuplicateOfEventID = review.DuplicateOfEventULID
	}
	return base
}

func buildReviewQueueItem(review events.ReviewQueueEntry) (reviewQueueItem, error) {
	// Parse warnings from JSON
	var warnings []events.ValidationWarning
	if len(review.Warnings) > 0 {
		if err := json.Unmarshal(review.Warnings, &warnings); err != nil {
			return reviewQueueItem{}, fmt.Errorf("build review queue item id=%d: parse warnings: %w", review.ID, err)
		}
	}

	// Parse original payload to extract event name
	eventName := ""
	if len(review.OriginalPayload) > 0 {
		var original map[string]any
		if err := json.Unmarshal(review.OriginalPayload, &original); err != nil {
			slog.Warn("failed to parse original payload",
				slog.Int("review_id", review.ID),
				slog.String("error", err.Error()))
		} else if name, ok := original["name"].(string); ok {
			eventName = name
		}
	}

	item := reviewQueueItem{
		reviewQueueBase: populateReviewQueueBase(review, warnings),
		EventName:       eventName,
	}

	if review.EventEndTime != nil {
		item.EventEndTime = review.EventEndTime
	}
	item.EventStartTime = &review.EventStartTime

	return item, nil
}

func buildReviewQueueDetail(review events.ReviewQueueEntry) (reviewQueueDetail, error) {
	// Parse warnings
	var warnings []events.ValidationWarning
	if len(review.Warnings) > 0 {
		if err := json.Unmarshal(review.Warnings, &warnings); err != nil {
			return reviewQueueDetail{}, fmt.Errorf("build review detail id=%d: parse warnings: %w", review.ID, err)
		}
	}

	// Parse original payload
	var original map[string]any
	if err := json.Unmarshal(review.OriginalPayload, &original); err != nil {
		return reviewQueueDetail{}, fmt.Errorf("build review detail id=%d: parse original payload: %w", review.ID, err)
	}

	// Parse normalized payload
	var normalized map[string]any
	if err := json.Unmarshal(review.NormalizedPayload, &normalized); err != nil {
		return reviewQueueDetail{}, fmt.Errorf("build review detail id=%d: parse normalized payload: %w", review.ID, err)
	}

	// Calculate changes
	changes := calculateChanges(original, normalized)

	detail := reviewQueueDetail{
		reviewQueueBase: populateReviewQueueBase(review, warnings),
		Original:        original,
		Normalized:      normalized,
		Changes:         changes,
	}

	if review.ReviewNotes != nil {
		detail.ReviewNotes = review.ReviewNotes
	}

	return detail, nil
}

func calculateChanges(original, normalized map[string]any) []changeDetail {
	changes := []changeDetail{}

	// Check for date changes
	if origEnd, ok := original["endDate"].(string); ok {
		if normEnd, ok := normalized["endDate"].(string); ok && origEnd != normEnd {
			changes = append(changes, changeDetail{
				Field:     "endDate",
				Original:  origEnd,
				Corrected: normEnd,
				Reason:    "Added 24 hours to fix reversed dates",
			})
		}
	}

	// Check for startDate changes (less common but possible)
	if origStart, ok := original["startDate"].(string); ok {
		if normStart, ok := normalized["startDate"].(string); ok && origStart != normStart {
			changes = append(changes, changeDetail{
				Field:     "startDate",
				Original:  origStart,
				Corrected: normStart,
				Reason:    "Date normalization",
			})
		}
	}

	return changes
}

func getUserFromContext(r *http.Request) string {
	// Get user identity from auth middleware claims (typed context key)
	if claims := middleware.AdminClaims(r); claims != nil && claims.Subject != "" {
		return claims.Subject
	}
	// Fallback for unauthenticated admin (e.g., dev mode)
	return "admin"
}

// recordNotDuplicatesFromWarnings extracts potential_duplicate warnings from a review entry
// and records each matched pair as not-duplicates so future ingestion won't re-flag them.
// Errors are logged but not propagated — this is a best-effort enhancement.
func (h *AdminReviewQueueHandler) recordNotDuplicatesFromWarnings(ctx context.Context, review *events.ReviewQueueEntry, reviewedBy string) {
	if review == nil || len(review.Warnings) == 0 {
		return
	}

	var warnings []events.ValidationWarning
	if err := json.Unmarshal(review.Warnings, &warnings); err != nil {
		slog.Warn("recordNotDuplicates: failed to parse warnings",
			slog.Int("review_id", review.ID),
			slog.String("error", err.Error()))
		return
	}

	eventULID := review.EventULID
	for _, w := range warnings {
		if w.Code != "potential_duplicate" {
			continue
		}
		matchesRaw, ok := w.Details["matches"]
		if !ok {
			continue
		}
		// matches is []any where each element is map[string]any with "ulid", "name", "similarity"
		matches, ok := matchesRaw.([]any)
		if !ok {
			continue
		}
		for _, m := range matches {
			matchMap, ok := m.(map[string]any)
			if !ok {
				continue
			}
			candidateULID, ok := matchMap["ulid"].(string)
			if !ok || candidateULID == "" {
				continue
			}
			if err := h.Repository.InsertNotDuplicate(ctx, eventULID, candidateULID, reviewedBy); err != nil {
				slog.Warn("recordNotDuplicates: failed to insert not-duplicate pair",
					slog.Int("review_id", review.ID),
					slog.String("event_ulid", eventULID),
					slog.String("candidate_ulid", candidateULID),
					slog.String("error", err.Error()))
			}
			// Best-effort: dismiss the companion's warning that pointed back at this event.
			h.dismissCompanionDuplicateWarning(ctx, candidateULID, eventULID)
		}
	}
}

// dismissCompanionDuplicateWarning removes any potential_duplicate warning match
// that references eventULID from the companion event's pending review entry.
// This prevents the companion's review from showing a stale duplicate alert after
// the pair has been marked as not-duplicates.
// Uses a single atomic SQL UPDATE — no read-modify-write race.
// Errors are logged but not propagated — this is a best-effort cleanup.
func (h *AdminReviewQueueHandler) dismissCompanionDuplicateWarning(ctx context.Context, companionULID, eventULID string) {
	if err := h.Repository.DismissCompanionWarningMatch(ctx, companionULID, eventULID); err != nil {
		slog.WarnContext(ctx, "dismissCompanionWarning: failed to atomically dismiss warning match",
			slog.String("companion_ulid", companionULID),
			slog.String("event_ulid", eventULID),
			slog.String("error", err.Error()))
	}
}
