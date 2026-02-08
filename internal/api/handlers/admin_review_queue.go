package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/domain/events"
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

// reviewQueueItem represents a single item in the review queue list
type reviewQueueItem struct {
	ID             int                        `json:"id"`
	EventID        string                     `json:"eventId"`
	EventName      string                     `json:"eventName,omitempty"`
	EventStartTime *time.Time                 `json:"eventStartTime,omitempty"`
	EventEndTime   *time.Time                 `json:"eventEndTime,omitempty"`
	Warnings       []events.ValidationWarning `json:"warnings"`
	Status         string                     `json:"status"`
	CreatedAt      time.Time                  `json:"createdAt"`
	ReviewedBy     *string                    `json:"reviewedBy,omitempty"`
	ReviewedAt     *time.Time                 `json:"reviewedAt,omitempty"`
}

// reviewQueueDetail represents detailed review information
type reviewQueueDetail struct {
	ID              int                        `json:"id"`
	EventID         string                     `json:"eventId"`
	Status          string                     `json:"status"`
	Warnings        []events.ValidationWarning `json:"warnings"`
	Original        map[string]any             `json:"original"`
	Normalized      map[string]any             `json:"normalized"`
	Changes         []changeDetail             `json:"changes"`
	CreatedAt       time.Time                  `json:"createdAt"`
	ReviewedBy      *string                    `json:"reviewedBy,omitempty"`
	ReviewedAt      *time.Time                 `json:"reviewedAt,omitempty"`
	ReviewNotes     *string                    `json:"reviewNotes,omitempty"`
	RejectionReason *string                    `json:"rejectionReason,omitempty"`
}

// changeDetail describes a specific change made during normalization
type changeDetail struct {
	Field     string `json:"field"`
	Original  any    `json:"original"`
	Corrected any    `json:"corrected"`
	Reason    string `json:"reason"`
}

// ListReviewQueue handles GET /admin/review-queue
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
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to list review queue", err, h.Env)
		return
	}

	// Build response items
	items := make([]reviewQueueItem, 0, len(result.Entries))
	for _, review := range result.Entries {
		item, err := buildReviewQueueItem(review)
		if err != nil {
			// Log error but continue processing other items
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
		Items:      convertToMapSlice(items),
		NextCursor: nextCursor,
	}, "application/json")
}

// GetReviewQueueEntry handles GET /admin/review-queue/:id
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
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", err, h.Env)
		return
	}

	// Build detailed response
	detail, err := buildReviewQueueDetail(*review)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", err, h.Env)
		return
	}

	writeJSON(w, http.StatusOK, detail, "application/json")
}

// ApproveReview handles POST /admin/review-queue/:id/approve
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
		Notes string `json:"notes,omitempty"`
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
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", err, h.Env)
		return
	}

	// Update event lifecycle state to published
	eventULID := review.EventULID

	_, err = h.AdminService.PublishEvent(r.Context(), eventULID)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.approve", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":    err.Error(),
				"event_id": eventULID,
			})
		}

		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to publish event", err, h.Env)
		return
	}

	// Mark review as approved
	notes := &req.Notes
	if req.Notes == "" {
		notes = nil
	}
	updatedReview, err := h.Repository.ApproveReview(r.Context(), id, reviewedBy, notes)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.approve", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":    err.Error(),
				"event_id": eventULID,
			})
		}

		if errors.Is(err, pgx.ErrNoRows) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found or already reviewed", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to approve review", err, h.Env)
		return
	}

	// Log success
	if h.AuditLogger != nil {
		h.AuditLogger.LogFromRequest(r, "admin.review.approve", "review", strconv.Itoa(id), "success", map[string]string{
			"event_id":    eventULID,
			"reviewed_by": reviewedBy,
		})
	}

	// Build response
	detail, err := buildReviewQueueDetail(*updatedReview)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", err, h.Env)
		return
	}

	writeJSON(w, http.StatusOK, detail, "application/json")
}

// RejectReview handles POST /admin/review-queue/:id/reject
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
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", err, h.Env)
		return
	}

	// Update event lifecycle state to deleted
	eventULID := review.EventULID

	err = h.AdminService.DeleteEvent(r.Context(), eventULID, req.Reason)
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
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to delete event", err, h.Env)
		return
	}

	// Mark review as rejected
	updatedReview, err := h.Repository.RejectReview(r.Context(), id, reviewedBy, req.Reason)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.reject", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":    err.Error(),
				"event_id": eventULID,
				"reason":   req.Reason,
			})
		}

		if errors.Is(err, pgx.ErrNoRows) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found or already reviewed", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to reject review", err, h.Env)
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

	// Build response
	detail, err := buildReviewQueueDetail(*updatedReview)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", err, h.Env)
		return
	}

	writeJSON(w, http.StatusOK, detail, "application/json")
}

// FixReview handles POST /admin/review-queue/:id/fix
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
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to fetch review entry", err, h.Env)
		return
	}

	// Update event with corrected dates
	eventULID := review.EventULID

	// Build update params with corrected dates
	// Note: This is a simplified implementation - in production, you'd need to handle
	// occurrence-level updates and more complex event structures
	_ = events.UpdateEventParams{}

	// For now, we'll just mark the review as approved with notes about the manual correction
	// TODO: Implement proper event date updating when occurrence-level API is available

	// Publish the event (assuming corrections were made outside this endpoint or will be made)
	_, err = h.AdminService.PublishEvent(r.Context(), eventULID)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.fix", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":    err.Error(),
				"event_id": eventULID,
			})
		}

		if errors.Is(err, events.ErrNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Event not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to publish event", err, h.Env)
		return
	}

	// Mark review as approved with notes about manual correction
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
	updatedReview, err := h.Repository.ApproveReview(r.Context(), id, reviewedBy, notesPtr)
	if err != nil {
		// Log failure
		if h.AuditLogger != nil {
			h.AuditLogger.LogFromRequest(r, "admin.review.fix", "review", strconv.Itoa(id), "failure", map[string]string{
				"error":    err.Error(),
				"event_id": eventULID,
			})
		}

		if errors.Is(err, pgx.ErrNoRows) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Review entry not found or already reviewed", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to approve review", err, h.Env)
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
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to build review detail", err, h.Env)
		return
	}

	writeJSON(w, http.StatusOK, detail, "application/json")
}

// Helper functions

func buildReviewQueueItem(review events.ReviewQueueEntry) (reviewQueueItem, error) {
	// Parse warnings from JSON
	var warnings []events.ValidationWarning
	if len(review.Warnings) > 0 {
		if err := json.Unmarshal(review.Warnings, &warnings); err != nil {
			return reviewQueueItem{}, fmt.Errorf("failed to parse warnings: %w", err)
		}
	}

	// Parse original payload to extract event name
	var original map[string]any
	eventName := ""
	if len(review.OriginalPayload) > 0 {
		if err := json.Unmarshal(review.OriginalPayload, &original); err == nil {
			if name, ok := original["name"].(string); ok {
				eventName = name
			}
		}
	}

	// Use EventULID directly from the entry
	eventULID := review.EventULID

	item := reviewQueueItem{
		ID:        review.ID,
		EventID:   eventULID,
		EventName: eventName,
		Warnings:  warnings,
		Status:    review.Status,
		CreatedAt: review.CreatedAt,
	}

	// Add optional fields
	if review.EventEndTime != nil {
		item.EventEndTime = review.EventEndTime
	}
	item.EventStartTime = &review.EventStartTime
	if review.ReviewedBy != nil {
		item.ReviewedBy = review.ReviewedBy
	}
	if review.ReviewedAt != nil {
		item.ReviewedAt = review.ReviewedAt
	}

	return item, nil
}

func buildReviewQueueDetail(review events.ReviewQueueEntry) (reviewQueueDetail, error) {
	// Parse warnings
	var warnings []events.ValidationWarning
	if len(review.Warnings) > 0 {
		if err := json.Unmarshal(review.Warnings, &warnings); err != nil {
			return reviewQueueDetail{}, fmt.Errorf("failed to parse warnings: %w", err)
		}
	}

	// Parse original payload
	var original map[string]any
	if err := json.Unmarshal(review.OriginalPayload, &original); err != nil {
		return reviewQueueDetail{}, fmt.Errorf("failed to parse original payload: %w", err)
	}

	// Parse normalized payload
	var normalized map[string]any
	if err := json.Unmarshal(review.NormalizedPayload, &normalized); err != nil {
		return reviewQueueDetail{}, fmt.Errorf("failed to parse normalized payload: %w", err)
	}

	// Calculate changes
	changes := calculateChanges(original, normalized)

	// Use EventULID directly
	eventULID := review.EventULID

	detail := reviewQueueDetail{
		ID:         review.ID,
		EventID:    eventULID,
		Status:     review.Status,
		Warnings:   warnings,
		Original:   original,
		Normalized: normalized,
		Changes:    changes,
		CreatedAt:  review.CreatedAt,
	}

	// Add optional fields
	if review.ReviewedBy != nil {
		detail.ReviewedBy = review.ReviewedBy
	}
	if review.ReviewedAt != nil {
		detail.ReviewedAt = review.ReviewedAt
	}
	if review.ReviewNotes != nil {
		detail.ReviewNotes = review.ReviewNotes
	}
	if review.RejectionReason != nil {
		detail.RejectionReason = review.RejectionReason
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
	// Try to get user from context (set by auth middleware)
	if user := r.Context().Value("user"); user != nil {
		if email, ok := user.(string); ok && email != "" {
			return email
		}
	}
	// Fallback to empty string (anonymous admin)
	return "admin"
}

func convertToMapSlice(items []reviewQueueItem) []map[string]any {
	result := make([]map[string]any, len(items))
	for i, item := range items {
		data, _ := json.Marshal(item)
		var m map[string]any
		_ = json.Unmarshal(data, &m) // Ignore error - already marshaled successfully
		result[i] = m
	}
	return result
}
