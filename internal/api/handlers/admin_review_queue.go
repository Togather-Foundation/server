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
	EventName       string     `json:"eventName,omitempty"`
	EventStartTime  *time.Time `json:"eventStartTime,omitempty"`
	EventEndTime    *time.Time `json:"eventEndTime,omitempty"`
	OccurrenceCount int        `json:"occurrenceCount,omitempty"`
}

// reviewQueueDetail represents detailed review information.
// Embeds reviewQueueBase for shared fields.
type reviewQueueDetail struct {
	reviewQueueBase
	Original      map[string]any       `json:"original"`
	Normalized    map[string]any       `json:"normalized"`
	Changes       []changeDetail       `json:"changes"`
	ReviewNotes   *string              `json:"reviewNotes,omitempty"`
	Occurrences   []occurrenceDetail   `json:"occurrences,omitempty"`
	RelatedEvents []relatedEventDetail `json:"relatedEvents,omitempty"`
}


// occurrenceDetail represents an occurrence in the API response
type occurrenceDetail struct {
	ID            string     `json:"id"`
	StartTime     time.Time  `json:"startTime"`
	EndTime       *time.Time `json:"endTime,omitempty"`
	Timezone      string     `json:"timezone"`
	DoorTime      *time.Time `json:"doorTime,omitempty"`
	VenueULID     *string    `json:"venueUlid,omitempty"`
	VirtualURL    *string    `json:"virtualUrl,omitempty"`
	TicketURL     string     `json:"ticketUrl,omitempty"`
	PriceMin      *float64   `json:"priceMin,omitempty"`
	PriceMax      *float64   `json:"priceMax,omitempty"`
	PriceCurrency string     `json:"priceCurrency,omitempty"`
	Availability  string     `json:"availability,omitempty"`
}

// relatedEventDetail represents a related event ULID in the API response
type relatedEventDetail struct {
	ULID string `json:"ulid"`
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
		item, err := buildReviewQueueItem(r.Context(), h.Repository, review)
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
		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", fmt.Errorf("get review queue entry id=%d: %w", id, err), h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", fmt.Errorf("get review queue entry id=%d: %w", id, err), h.Env)
		return
	}

	// Build detailed response
	detail, err := buildReviewQueueDetail(r.Context(), h.Repository, *review)
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
		if errors.Is(err, events.ErrNotFound) {
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
	detail, err := buildReviewQueueDetail(r.Context(), h.Repository, *updatedReview)
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
		if errors.Is(err, events.ErrNotFound) {
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
	detail, err := buildReviewQueueDetail(r.Context(), h.Repository, *updatedReview)
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
		if errors.Is(err, events.ErrNotFound) {
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
	detail, err := buildReviewQueueDetail(r.Context(), h.Repository, *updatedReview)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", fmt.Errorf("fix review id=%d: build detail: %w", id, err), h.Env)
		return
	}

	writeJSON(w, http.StatusOK, detail, "application/json")
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

func buildReviewQueueItem(ctx context.Context, repo events.Repository, review events.ReviewQueueEntry) (reviewQueueItem, error) {
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

	// Fetch occurrence count for the event
	occCount, err := repo.CountOccurrences(ctx, review.EventID)
	if err != nil {
		// Log but don't fail — occurrence count is optional
		slog.WarnContext(ctx, "buildReviewQueueItem: failed to fetch occurrence count",
			slog.String("event_id", review.EventID),
			slog.String("error", err.Error()))
		item.OccurrenceCount = 0
	} else {
		item.OccurrenceCount = int(occCount)
	}

	return item, nil
}

func buildReviewQueueDetail(ctx context.Context, repo events.Repository, review events.ReviewQueueEntry) (reviewQueueDetail, error) {
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

	// Fetch live event to get occurrences
	event, err := repo.GetByULID(ctx, review.EventULID)
	if err != nil {
		// Log but don't fail — occurrences are optional
		slog.WarnContext(ctx, "buildReviewQueueDetail: failed to fetch event for occurrences",
			slog.String("event_ulid", review.EventULID),
			slog.String("error", err.Error()))
		event = nil
	}

	// Extract and convert occurrences
	var occurrences []occurrenceDetail
	if event != nil && len(event.Occurrences) > 0 {
		occurrences = make([]occurrenceDetail, len(event.Occurrences))
		for i, occ := range event.Occurrences {
			occurrences[i] = occurrenceDetail{
				ID:            occ.ID,
				StartTime:     occ.StartTime,
				EndTime:       occ.EndTime,
				Timezone:      occ.Timezone,
				DoorTime:      occ.DoorTime,
				VenueULID:     occ.VenueULID,
				VirtualURL:    occ.VirtualURL,
				TicketURL:     occ.TicketURL,
				PriceMin:      occ.PriceMin,
				PriceMax:      occ.PriceMax,
				PriceCurrency: occ.PriceCurrency,
				Availability:  occ.Availability,
			}
		}
	}

	// Extract related event ULIDs from warnings and DuplicateOfEventULID
	relatedULIDs := extractRelatedEventULIDs(warnings, review.DuplicateOfEventULID)
	relatedEvents := make([]relatedEventDetail, len(relatedULIDs))
	for i, ulid := range relatedULIDs {
		relatedEvents[i] = relatedEventDetail{ULID: ulid}
	}

	detail := reviewQueueDetail{
		reviewQueueBase: populateReviewQueueBase(review, warnings),
		Original:        original,
		Normalized:      normalized,
		Changes:         changes,
		Occurrences:     occurrences,
		RelatedEvents:   relatedEvents,
	}

	if review.ReviewNotes != nil {
		detail.ReviewNotes = review.ReviewNotes
	}

	return detail, nil
}


// extractRelatedEventULIDs collects unique related event ULIDs from:
// 1. review.DuplicateOfEventULID (companion review ULID)
// 2. warning.details.matches[].ulid (from potential_duplicate, near_duplicate_of_new_event)
// Deduplicates ULIDs before returning.
func extractRelatedEventULIDs(warnings []events.ValidationWarning, duplicateOfEventULID *string) []string {
	seen := make(map[string]bool)
	var ulids []string

	// Add companion ULID if present
	if duplicateOfEventULID != nil && *duplicateOfEventULID != "" {
		ulids = append(ulids, *duplicateOfEventULID)
		seen[*duplicateOfEventULID] = true
	}

	// Extract ULIDs from warning matches
	for _, w := range warnings {
		if w.Code != "potential_duplicate" && w.Code != "near_duplicate_of_new_event" {
			continue
		}
		if w.Details == nil {
			continue
		}

		matches, ok := w.Details["matches"]
		if !ok {
			continue
		}

		matchesSlice, ok := matches.([]interface{})
		if !ok {
			continue
		}

		for _, m := range matchesSlice {
			matchMap, ok := m.(map[string]interface{})
			if !ok {
				continue
			}

			ulidVal, ok := matchMap["ulid"]
			if !ok {
				continue
			}

			ulidStr, ok := ulidVal.(string)
			if !ok || ulidStr == "" {
				continue
			}

			// Deduplicate
			if !seen[ulidStr] {
				ulids = append(ulids, ulidStr)
				seen[ulidStr] = true
			}
		}
	}

	return ulids
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

// recordNotDuplicatesFromWarnings extracts duplicate-related warnings from a review entry
// and records each matched pair as not-duplicates so future ingestion won't re-flag them.
// Handles both potential_duplicate and near_duplicate_of_new_event warning codes so that
// approving either side of a companion pair records the not-duplicate decision correctly.
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
		switch w.Code {
		case "potential_duplicate":
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
				// Best-effort: refresh the companion review and auto-approve it if no
				// issues remain after the duplicate pair is removed.
				h.recheckCompanionNotDuplicateReview(ctx, candidateULID, eventULID, reviewedBy)
			}

		case "near_duplicate_of_new_event":
			// The companion (new) event ULID is stored in DuplicateOfEventULID on the
			// review entry — the warning details only contain display fields (name, dates)
			// and do not embed a ULID.  Use the FK directly.
			if review.DuplicateOfEventULID == nil || *review.DuplicateOfEventULID == "" {
				continue
			}
			companionULID := *review.DuplicateOfEventULID
			if err := h.Repository.InsertNotDuplicate(ctx, eventULID, companionULID, reviewedBy); err != nil {
				slog.Warn("recordNotDuplicates: failed to insert not-duplicate pair (near-dup side)",
					slog.Int("review_id", review.ID),
					slog.String("event_ulid", eventULID),
					slog.String("companion_ulid", companionULID),
					slog.String("error", err.Error()))
			}
			// Best-effort: refresh the companion review and auto-approve it if no
			// issues remain after the duplicate pair is removed.
			h.recheckCompanionNotDuplicateReview(ctx, companionULID, eventULID, reviewedBy)
		}
	}
}

// recheckCompanionNotDuplicateReview removes duplicate-pair warnings from the exact
// companion review row and then rechecks that pending event: if no warnings remain,
// the companion review is auto-approved; otherwise it stays pending with refreshed
// warnings.
//
// Errors are logged but not propagated — this is a best-effort companion cleanup.
func (h *AdminReviewQueueHandler) recheckCompanionNotDuplicateReview(ctx context.Context, companionULID, eventULID, reviewedBy string) {
	// Find the exact companion review that is cross-linked to eventULID.
	companion, err := h.Repository.GetPendingReviewByEventUlidAndDuplicateUlid(ctx, companionULID, eventULID)
	if err != nil {
		// ErrNotFound is normal (companion may have been already processed or never
		// existed).  Any other error is unexpected but still best-effort.
		if !errors.Is(err, events.ErrNotFound) {
			slog.WarnContext(ctx, "recheckCompanionNotDuplicateReview: failed to look up companion review",
				slog.String("companion_ulid", companionULID),
				slog.String("event_ulid", eventULID),
				slog.String("error", err.Error()))
		}
		return
	}
	if companion == nil {
		// No pending review for this companion/duplicate pair — nothing to do.
		return
	}

	updatedWarnings, err := filteredCompanionWarnings(companion, eventULID)
	if err != nil {
		slog.WarnContext(ctx, "recheckCompanionNotDuplicateReview: failed to filter companion warnings",
			slog.Int("review_id", companion.ID),
			slog.String("companion_ulid", companionULID),
			slog.String("event_ulid", eventULID),
			slog.String("error", err.Error()))
		return
	}

	if err := h.Repository.UpdateReviewWarnings(ctx, companion.ID, updatedWarnings); err != nil {
		slog.WarnContext(ctx, "recheckCompanionNotDuplicateReview: failed to update companion warnings",
			slog.Int("review_id", companion.ID),
			slog.String("companion_ulid", companionULID),
			slog.String("event_ulid", eventULID),
			slog.String("error", err.Error()))
		return
	}

	if string(updatedWarnings) == "[]" {
		if h.AdminService == nil {
			slog.WarnContext(ctx, "recheckCompanionNotDuplicateReview: admin service unavailable for auto-approval",
				slog.Int("review_id", companion.ID),
				slog.String("companion_ulid", companionULID),
				slog.String("event_ulid", eventULID))
			return
		}
		note := "Auto-approved after companion not-duplicate recheck"
		if _, err := h.AdminService.ApproveEventWithReview(ctx, companionULID, companion.ID, reviewedBy, &note); err != nil {
			slog.WarnContext(ctx, "recheckCompanionNotDuplicateReview: failed to auto-approve companion review",
				slog.Int("review_id", companion.ID),
				slog.String("companion_ulid", companionULID),
				slog.String("event_ulid", eventULID),
				slog.String("error", err.Error()))
		}
		return
	}
}

func filteredCompanionWarnings(companion *events.ReviewQueueEntry, eventULID string) ([]byte, error) {
	if companion == nil || len(companion.Warnings) == 0 {
		return []byte("[]"), nil
	}

	var warnings []events.ValidationWarning
	if err := json.Unmarshal(companion.Warnings, &warnings); err != nil {
		return nil, fmt.Errorf("parse companion warnings: %w", err)
	}

	filtered := make([]events.ValidationWarning, 0, len(warnings))
	for _, warning := range warnings {
		switch warning.Code {
		case "potential_duplicate":
			updatedWarning, keep := filterPotentialDuplicateWarning(warning, eventULID)
			if keep {
				filtered = append(filtered, updatedWarning)
			}
		case "near_duplicate_of_new_event":
			if companion.DuplicateOfEventULID != nil && *companion.DuplicateOfEventULID == eventULID {
				continue
			}
			filtered = append(filtered, warning)
		default:
			filtered = append(filtered, warning)
		}
	}

	encoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, fmt.Errorf("marshal companion warnings: %w", err)
	}
	return encoded, nil
}

func filterPotentialDuplicateWarning(warning events.ValidationWarning, eventULID string) (events.ValidationWarning, bool) {
	matchesRaw, ok := warning.Details["matches"]
	if !ok {
		return warning, true
	}
	matches, ok := matchesRaw.([]any)
	if !ok {
		return warning, true
	}

	filteredMatches := make([]any, 0, len(matches))
	removed := false
	for _, match := range matches {
		matchMap, ok := match.(map[string]any)
		if !ok {
			filteredMatches = append(filteredMatches, match)
			continue
		}
		candidateULID, ok := matchMap["ulid"].(string)
		if ok && candidateULID == eventULID {
			removed = true
			continue
		}
		filteredMatches = append(filteredMatches, match)
	}

	if !removed {
		return warning, true
	}
	if len(filteredMatches) == 0 {
		return warning, false
	}

	updatedDetails := make(map[string]any, len(warning.Details))
	for key, value := range warning.Details {
		updatedDetails[key] = value
	}
	updatedDetails["matches"] = filteredMatches
	warning.Details = updatedDetails
	return warning, true
}
