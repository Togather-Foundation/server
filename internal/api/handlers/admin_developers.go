package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/google/uuid"
)

// DeveloperService defines the interface for developer management operations
type DeveloperService interface {
	InviteDeveloper(ctx context.Context, email string, invitedBy *uuid.UUID) (string, error)
	ListOwnKeys(ctx context.Context, developerID uuid.UUID) ([]*developers.APIKeyWithUsage, error)
	RevokeOwnKey(ctx context.Context, developerID uuid.UUID, keyID uuid.UUID) error
	GetUsageStats(ctx context.Context, developerID uuid.UUID, startDate, endDate time.Time) (*developers.UsageStats, error)
}

// AdminDevelopersHandler handles developer management operations for admins
type AdminDevelopersHandler struct {
	service     DeveloperService
	repo        developers.Repository
	auditLogger *audit.Logger
	env         string
}

// NewAdminDevelopersHandler creates a new admin developers handler
func NewAdminDevelopersHandler(service DeveloperService, repo developers.Repository, auditLogger *audit.Logger, env string) *AdminDevelopersHandler {
	return &AdminDevelopersHandler{
		service:     service,
		repo:        repo,
		auditLogger: auditLogger,
		env:         env,
	}
}

// Request/Response types

// InviteDeveloperRequest represents the request body for inviting a developer
type InviteDeveloperRequest struct {
	Email   string `json:"email"`
	Name    string `json:"name,omitempty"`
	MaxKeys int    `json:"max_keys,omitempty"`
}

// AdminDeveloperResponse represents a developer in API responses
type AdminDeveloperResponse struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	GitHubUsername  *string    `json:"github_username,omitempty"`
	MaxKeys         int        `json:"max_keys"`
	KeyCount        int        `json:"key_count"`
	IsActive        bool       `json:"is_active"`
	Status          string     `json:"status"` // "active", "invited", "deactivated"
	CreatedAt       time.Time  `json:"created_at"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	InvitationSent  bool       `json:"invitation_sent,omitempty"`
	RequestsLast30d int64      `json:"requests_last_30d,omitempty"`
}

// UpdateDeveloperRequest represents the request body for updating a developer
type UpdateDeveloperRequest struct {
	MaxKeys  *int  `json:"max_keys,omitempty"`
	IsActive *bool `json:"is_active,omitempty"`
}

// ListDevelopersResponse represents the paginated list of developers
type ListDevelopersResponse struct {
	Items      []AdminDeveloperResponse `json:"items"`
	NextCursor *string                  `json:"next_cursor,omitempty"`
	Total      int64                    `json:"total"`
}

// InviteDeveloper handles POST /api/v1/admin/developers/invite - Invite developer by email
func (h *AdminDevelopersHandler) InviteDeveloper(w http.ResponseWriter, r *http.Request) {
	var req InviteDeveloperRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.env)
		return
	}

	// Validate email
	if strings.TrimSpace(req.Email) == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Email is required", nil, h.env)
		return
	}

	// Get admin claims from context
	claims := middleware.AdminClaims(r)
	var invitedBy *uuid.UUID
	if claims != nil && claims.Subject != "" {
		id, err := uuid.Parse(claims.Subject)
		if err == nil {
			invitedBy = &id
		}
	}

	// Send invitation
	token, err := h.service.InviteDeveloper(r.Context(), req.Email, invitedBy)
	if err != nil {
		status, problemType, title := mapDeveloperError(err)
		problem.Write(w, r, status, problemType, title, err, h.env)
		return
	}

	// Audit log
	h.auditLogger.LogFromRequest(r, "developer.invited", "developer", req.Email, "success", map[string]string{
		"email": req.Email,
	})

	// Return invitation details (token would be sent via email in production)
	resp := map[string]interface{}{
		"email":                 req.Email,
		"status":                "invited",
		"invitation_sent":       true,
		"invitation_token":      token, // In production, this would only be in the email
		"invitation_expires_at": time.Now().Add(168 * time.Hour).Format(time.RFC3339),
	}

	writeJSON(w, http.StatusCreated, resp, contentTypeFromRequest(r))
}

// ListDevelopers handles GET /api/v1/admin/developers - List all developers
func (h *AdminDevelopersHandler) ListDevelopers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse pagination
	limit := int(50)
	if limitParam := query.Get("limit"); limitParam != "" {
		parsedLimit, err := strconv.ParseInt(limitParam, 10, 32)
		if err != nil || parsedLimit < 1 || parsedLimit > 100 {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Limit must be between 1 and 100", nil, h.env)
			return
		}
		limit = int(parsedLimit)
	}

	offset := int(0)
	if offsetParam := query.Get("offset"); offsetParam != "" {
		parsedOffset, err := strconv.ParseInt(offsetParam, 10, 32)
		if err != nil || parsedOffset < 0 {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Offset must be >= 0", nil, h.env)
			return
		}
		offset = int(parsedOffset)
	}

	// List developers
	devs, err := h.repo.ListDevelopers(r.Context(), limit, offset)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to list developers", err, h.env)
		return
	}

	// Get total count
	total, err := h.repo.CountDevelopers(r.Context())
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to count developers", err, h.env)
		return
	}

	// Build response with key counts and usage stats
	items := make([]AdminDeveloperResponse, 0, len(devs))
	for _, dev := range devs {
		// Get key count
		keyCount, _ := h.repo.CountDeveloperAPIKeys(r.Context(), dev.ID)

		// Get 30-day usage stats
		startDate := time.Now().AddDate(0, 0, -30)
		endDate := time.Now()
		usage, _, _ := h.repo.GetDeveloperUsageTotal(r.Context(), dev.ID, startDate, endDate)

		status := "active"
		if !dev.IsActive {
			status = "deactivated"
		} else if dev.LastLoginAt == nil {
			status = "invited"
		}

		items = append(items, AdminDeveloperResponse{
			ID:              dev.ID.String(),
			Email:           dev.Email,
			Name:            dev.Name,
			GitHubUsername:  dev.GitHubUsername,
			MaxKeys:         dev.MaxKeys,
			KeyCount:        int(keyCount),
			IsActive:        dev.IsActive,
			Status:          status,
			CreatedAt:       dev.CreatedAt,
			LastLoginAt:     dev.LastLoginAt,
			RequestsLast30d: usage,
		})
	}

	// Build next cursor if there are more results
	var nextCursor *string
	if int64(offset+limit) < total {
		cursor := strconv.Itoa(offset + limit)
		nextCursor = &cursor
	}

	resp := ListDevelopersResponse{
		Items:      items,
		NextCursor: nextCursor,
		Total:      total,
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// GetDeveloper handles GET /api/v1/admin/developers/{id} - Get developer details with keys
func (h *AdminDevelopersHandler) GetDeveloper(w http.ResponseWriter, r *http.Request) {
	// Extract and validate UUID from path
	idStr := pathParam(r, "id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Developer ID is required", nil, h.env)
		return
	}

	developerID, err := uuid.Parse(idStr)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid developer ID format", err, h.env)
		return
	}

	// Get developer
	dev, err := h.repo.GetDeveloperByID(r.Context(), developerID)
	if err != nil {
		if errors.Is(err, developers.ErrDeveloperNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Developer not found", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get developer", err, h.env)
		return
	}

	// Get developer's API keys
	keys, err := h.service.ListOwnKeys(r.Context(), developerID)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get developer keys", err, h.env)
		return
	}

	// Get 30-day usage stats
	startDate := time.Now().AddDate(0, 0, -30)
	endDate := time.Now()
	usage, _, _ := h.repo.GetDeveloperUsageTotal(r.Context(), developerID, startDate, endDate)

	status := "active"
	if !dev.IsActive {
		status = "deactivated"
	} else if dev.LastLoginAt == nil {
		status = "invited"
	}

	// Format keys for response
	keyResponses := make([]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		keyResponses = append(keyResponses, map[string]interface{}{
			"id":              key.ID.String(),
			"prefix":          key.Prefix,
			"name":            key.Name,
			"role":            key.Role,
			"rate_limit_tier": key.RateLimitTier,
			"is_active":       key.IsActive,
			"created_at":      key.CreatedAt,
			"last_used_at":    key.LastUsedAt,
			"expires_at":      key.ExpiresAt,
			"usage_today":     key.UsageToday,
			"usage_7d":        key.Usage7d,
			"usage_30d":       key.Usage30d,
		})
	}

	resp := map[string]interface{}{
		"id":                dev.ID.String(),
		"email":             dev.Email,
		"name":              dev.Name,
		"github_username":   dev.GitHubUsername,
		"max_keys":          dev.MaxKeys,
		"key_count":         len(keys),
		"is_active":         dev.IsActive,
		"status":            status,
		"created_at":        dev.CreatedAt,
		"last_login_at":     dev.LastLoginAt,
		"requests_last_30d": usage,
		"keys":              keyResponses,
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// UpdateDeveloper handles PUT /api/v1/admin/developers/{id} - Update developer
func (h *AdminDevelopersHandler) UpdateDeveloper(w http.ResponseWriter, r *http.Request) {
	// Extract and validate UUID from path
	idStr := pathParam(r, "id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Developer ID is required", nil, h.env)
		return
	}

	developerID, err := uuid.Parse(idStr)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid developer ID format", err, h.env)
		return
	}

	// Parse request body
	var req UpdateDeveloperRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.env)
		return
	}

	// Validate max_keys if provided
	if req.MaxKeys != nil && *req.MaxKeys < 0 {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "max_keys must be >= 0", nil, h.env)
		return
	}

	// Update developer
	updateParams := developers.UpdateDeveloperParams{
		MaxKeys:  req.MaxKeys,
		IsActive: req.IsActive,
	}

	updatedDev, err := h.repo.UpdateDeveloper(r.Context(), developerID, updateParams)
	if err != nil {
		if errors.Is(err, developers.ErrDeveloperNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Developer not found", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to update developer", err, h.env)
		return
	}

	// Audit log
	h.auditLogger.LogFromRequest(r, "developer.updated", "developer", developerID.String(), "success", map[string]string{
		"email":     updatedDev.Email,
		"max_keys":  fmt.Sprintf("%d", updatedDev.MaxKeys),
		"is_active": fmt.Sprintf("%t", updatedDev.IsActive),
	})

	// Return updated developer
	status := "active"
	if !updatedDev.IsActive {
		status = "deactivated"
	} else if updatedDev.LastLoginAt == nil {
		status = "invited"
	}

	resp := AdminDeveloperResponse{
		ID:          updatedDev.ID.String(),
		Email:       updatedDev.Email,
		Name:        updatedDev.Name,
		MaxKeys:     updatedDev.MaxKeys,
		IsActive:    updatedDev.IsActive,
		Status:      status,
		CreatedAt:   updatedDev.CreatedAt,
		LastLoginAt: updatedDev.LastLoginAt,
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// DeactivateDeveloper handles DELETE /api/v1/admin/developers/{id} - Deactivate developer and revoke all keys
func (h *AdminDevelopersHandler) DeactivateDeveloper(w http.ResponseWriter, r *http.Request) {
	// Extract and validate UUID from path
	idStr := pathParam(r, "id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Developer ID is required", nil, h.env)
		return
	}

	developerID, err := uuid.Parse(idStr)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid developer ID format", err, h.env)
		return
	}

	// Get developer to verify existence
	dev, err := h.repo.GetDeveloperByID(r.Context(), developerID)
	if err != nil {
		if errors.Is(err, developers.ErrDeveloperNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Developer not found", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get developer", err, h.env)
		return
	}

	// Deactivate developer
	if err := h.repo.DeactivateDeveloper(r.Context(), developerID); err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to deactivate developer", err, h.env)
		return
	}

	// Revoke all API keys
	keys, _ := h.repo.ListDeveloperAPIKeys(r.Context(), developerID)
	for _, key := range keys {
		_ = h.repo.DeactivateAPIKey(r.Context(), key.ID)
	}

	// Audit log
	h.auditLogger.LogFromRequest(r, "developer.deactivated", "developer", developerID.String(), "success", map[string]string{
		"email":      dev.Email,
		"keys_count": fmt.Sprintf("%d", len(keys)),
	})

	w.WriteHeader(http.StatusNoContent)
}

// Helper functions

// mapDeveloperError maps domain errors to HTTP status codes and problem types
func mapDeveloperError(err error) (status int, problemType, title string) {
	switch {
	case errors.Is(err, developers.ErrEmailTaken):
		return http.StatusConflict, "https://sel.events/problems/conflict", "Email already taken"
	case errors.Is(err, developers.ErrDeveloperNotFound):
		return http.StatusNotFound, "https://sel.events/problems/not-found", "Developer not found"
	case errors.Is(err, developers.ErrInvalidToken):
		return http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid or expired invitation token"
	default:
		return http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error"
	}
}
