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
	"github.com/Togather-Foundation/server/internal/domain/users"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserService defines the interface for user management operations
type UserService interface {
	CreateUserAndInvite(ctx context.Context, params users.CreateUserParams) (postgres.User, error)
	ListUsers(ctx context.Context, filters users.ListUsersFilters) ([]postgres.ListUsersWithFiltersRow, int64, error)
	GetUser(ctx context.Context, id pgtype.UUID) (postgres.GetUserByIDRow, error)
	UpdateUser(ctx context.Context, id pgtype.UUID, params users.UpdateUserParams, updatedBy string) error
	DeleteUser(ctx context.Context, id pgtype.UUID, deletedBy string) error
	DeactivateUser(ctx context.Context, id pgtype.UUID, deactivatedBy string) error
	ActivateUser(ctx context.Context, id pgtype.UUID, activatedBy string) error
	ResendInvitation(ctx context.Context, userID pgtype.UUID, resentBy string) error
}

// AdminUsersHandler handles user management CRUD operations
type AdminUsersHandler struct {
	userService UserService
	auditLogger *audit.Logger
	env         string
}

// NewAdminUsersHandler creates a new admin users handler
func NewAdminUsersHandler(userService UserService, auditLogger *audit.Logger, env string) *AdminUsersHandler {
	return &AdminUsersHandler{
		userService: userService,
		auditLogger: auditLogger,
		env:         env,
	}
}

// Request/Response types

// CreateUserRequest represents the request body for creating a user
type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"` // admin, editor, viewer
}

// UpdateUserRequest represents the request body for updating a user
type UpdateUserRequest struct {
	Username *string `json:"username,omitempty"`
	Email    *string `json:"email,omitempty"`
	Role     *string `json:"role,omitempty"`
}

// AdminUserResponse represents a user in API responses (with full details)
type AdminUserResponse struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// ListUsersResponse represents the paginated list of users
type ListUsersResponse struct {
	Items      []AdminUserResponse `json:"items"`
	NextCursor *string             `json:"next_cursor,omitempty"`
	Total      int64               `json:"total"`
}

// MessageResponse represents a simple message response
type MessageResponse struct {
	Message string `json:"message"`
}

// CreateUser handles POST /api/v1/admin/users - Create user and send invitation
func (h *AdminUsersHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.env)
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.Username) == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Username is required", nil, h.env)
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Email is required", nil, h.env)
		return
	}
	if req.Role != "" && req.Role != "admin" && req.Role != "editor" && req.Role != "viewer" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Role must be one of: admin, editor, viewer", nil, h.env)
		return
	}

	// Get admin claims from context
	claims := middleware.AdminClaims(r)
	var createdByUUID pgtype.UUID
	if claims != nil && claims.Subject != "" {
		if err := createdByUUID.Scan(claims.Subject); err != nil {
			// Log warning but continue - audit log will use username
			createdByUUID = pgtype.UUID{}
		}
	}

	// Create user and send invitation
	params := users.CreateUserParams{
		Username:  req.Username,
		Email:     req.Email,
		Role:      req.Role,
		CreatedBy: createdByUUID,
	}

	user, err := h.userService.CreateUserAndInvite(r.Context(), params)
	if err != nil {
		status, problemType, title := mapUserError(err)
		problem.Write(w, r, status, problemType, title, err, h.env)
		return
	}

	// Audit log (already logged in service, but log from request context too)
	h.auditLogger.LogFromRequest(r, "user.created", "user", formatUUID(user.ID), "success", map[string]string{
		"username": user.Username,
		"email":    user.Email,
		"role":     user.Role,
	})

	// Return user object
	resp := AdminUserResponse{
		ID:          formatUUID(user.ID),
		Username:    user.Username,
		Email:       user.Email,
		Role:        user.Role,
		IsActive:    user.IsActive,
		CreatedAt:   user.CreatedAt.Time,
		LastLoginAt: timePtr(user.LastLoginAt),
	}

	writeJSON(w, http.StatusCreated, resp, contentTypeFromRequest(r))
}

// ListUsers handles GET /api/v1/admin/users - List all users with filters
func (h *AdminUsersHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse filters
	var isActive *bool
	if statusParam := query.Get("status"); statusParam != "" {
		switch statusParam {
		case "active":
			active := true
			isActive = &active
		case "inactive":
			inactive := false
			isActive = &inactive
		default:
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Status must be 'active' or 'inactive'", nil, h.env)
			return
		}
	}

	var role *string
	if roleParam := query.Get("role"); roleParam != "" {
		if roleParam != "admin" && roleParam != "editor" && roleParam != "viewer" {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Role must be 'admin', 'editor', or 'viewer'", nil, h.env)
			return
		}
		role = &roleParam
	}

	// Parse pagination (offset-based for now, can add cursor later)
	limit := int32(50)
	if limitParam := query.Get("limit"); limitParam != "" {
		parsedLimit, err := strconv.ParseInt(limitParam, 10, 32)
		if err != nil || parsedLimit < 1 || parsedLimit > 100 {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Limit must be between 1 and 100", nil, h.env)
			return
		}
		limit = int32(parsedLimit)
	}

	offset := int32(0)
	if offsetParam := query.Get("offset"); offsetParam != "" {
		parsedOffset, err := strconv.ParseInt(offsetParam, 10, 32)
		if err != nil || parsedOffset < 0 {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Offset must be >= 0", nil, h.env)
			return
		}
		offset = int32(parsedOffset)
	}

	// List users
	filters := users.ListUsersFilters{
		IsActive: isActive,
		Role:     role,
		Limit:    limit,
		Offset:   offset,
	}

	userRows, total, err := h.userService.ListUsers(r.Context(), filters)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to list users", err, h.env)
		return
	}

	// Build response
	items := make([]AdminUserResponse, 0, len(userRows))
	for _, user := range userRows {
		items = append(items, AdminUserResponse{
			ID:          formatUUID(user.ID),
			Username:    user.Username,
			Email:       user.Email,
			Role:        user.Role,
			IsActive:    user.IsActive,
			CreatedAt:   user.CreatedAt.Time,
			LastLoginAt: timePtr(user.LastLoginAt),
		})
	}

	// Build next cursor if there are more results
	var nextCursor *string
	if int64(offset+limit) < total {
		cursor := strconv.Itoa(int(offset + limit))
		nextCursor = &cursor
	}

	resp := ListUsersResponse{
		Items:      items,
		NextCursor: nextCursor,
		Total:      total,
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// GetUser handles GET /api/v1/admin/users/{id} - Get single user details
func (h *AdminUsersHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	// Extract and validate UUID from path
	idStr := pathParam(r, "id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "User ID is required", nil, h.env)
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(idStr); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid user ID format", err, h.env)
		return
	}

	// Get user
	user, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, users.ErrUserNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "User not found", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get user", err, h.env)
		return
	}

	// Return user object
	resp := AdminUserResponse{
		ID:          formatUUID(user.ID),
		Username:    user.Username,
		Email:       user.Email,
		Role:        user.Role,
		IsActive:    user.IsActive,
		CreatedAt:   user.CreatedAt.Time,
		LastLoginAt: timePtr(user.LastLoginAt),
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// UpdateUser handles PUT /api/v1/admin/users/{id} - Update user details
func (h *AdminUsersHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	// Extract and validate UUID from path
	idStr := pathParam(r, "id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "User ID is required", nil, h.env)
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(idStr); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid user ID format", err, h.env)
		return
	}

	// Parse request body
	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.env)
		return
	}

	// Get existing user to merge updates
	existingUser, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, users.ErrUserNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "User not found", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get user", err, h.env)
		return
	}

	// Merge updates with existing values
	username := existingUser.Username
	if req.Username != nil && strings.TrimSpace(*req.Username) != "" {
		username = strings.TrimSpace(*req.Username)
	}

	email := existingUser.Email
	if req.Email != nil && strings.TrimSpace(*req.Email) != "" {
		email = strings.TrimSpace(*req.Email)
	}

	role := existingUser.Role
	if req.Role != nil && strings.TrimSpace(*req.Role) != "" {
		roleVal := strings.TrimSpace(*req.Role)
		if roleVal != "admin" && roleVal != "editor" && roleVal != "viewer" {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Role must be one of: admin, editor, viewer", nil, h.env)
			return
		}
		role = roleVal
	}

	// Get admin username for audit log
	adminUsername := "unknown"
	if claims := middleware.AdminClaims(r); claims != nil {
		adminUsername = claims.Subject
	}

	// Update user
	updateParams := users.UpdateUserParams{
		Username: username,
		Email:    email,
		Role:     role,
	}

	if err := h.userService.UpdateUser(r.Context(), userID, updateParams, adminUsername); err != nil {
		status, problemType, title := mapUserError(err)
		problem.Write(w, r, status, problemType, title, err, h.env)
		return
	}

	// Get updated user
	updatedUser, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get updated user", err, h.env)
		return
	}

	// Audit log (already logged in service, but log from request context too)
	h.auditLogger.LogFromRequest(r, "user.updated", "user", formatUUID(userID), "success", map[string]string{
		"username": updatedUser.Username,
		"email":    updatedUser.Email,
		"role":     updatedUser.Role,
	})

	// Return updated user
	resp := AdminUserResponse{
		ID:          formatUUID(updatedUser.ID),
		Username:    updatedUser.Username,
		Email:       updatedUser.Email,
		Role:        updatedUser.Role,
		IsActive:    updatedUser.IsActive,
		CreatedAt:   updatedUser.CreatedAt.Time,
		LastLoginAt: timePtr(updatedUser.LastLoginAt),
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// DeleteUser handles DELETE /api/v1/admin/users/{id} - Soft delete user
func (h *AdminUsersHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	// Extract and validate UUID from path
	idStr := pathParam(r, "id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "User ID is required", nil, h.env)
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(idStr); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid user ID format", err, h.env)
		return
	}

	// Get admin username for audit log
	adminUsername := "unknown"
	if claims := middleware.AdminClaims(r); claims != nil {
		adminUsername = claims.Subject
	}

	// Delete user (soft delete)
	if err := h.userService.DeleteUser(r.Context(), userID, adminUsername); err != nil {
		if errors.Is(err, users.ErrUserNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "User not found", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to delete user", err, h.env)
		return
	}

	// Audit log (already logged in service)
	h.auditLogger.LogFromRequest(r, "user.deleted", "user", formatUUID(userID), "success", nil)

	w.WriteHeader(http.StatusNoContent)
}

// DeactivateUser handles POST /api/v1/admin/users/{id}/deactivate - Deactivate user
func (h *AdminUsersHandler) DeactivateUser(w http.ResponseWriter, r *http.Request) {
	// Extract and validate UUID from path
	idStr := pathParam(r, "id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "User ID is required", nil, h.env)
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(idStr); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid user ID format", err, h.env)
		return
	}

	// Get admin username for audit log
	adminUsername := "unknown"
	if claims := middleware.AdminClaims(r); claims != nil {
		adminUsername = claims.Subject
	}

	// Deactivate user
	if err := h.userService.DeactivateUser(r.Context(), userID, adminUsername); err != nil {
		if errors.Is(err, users.ErrUserNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "User not found", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to deactivate user", err, h.env)
		return
	}

	// Get updated user
	user, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get user", err, h.env)
		return
	}

	// Audit log (already logged in service)
	h.auditLogger.LogFromRequest(r, "user.deactivated", "user", formatUUID(userID), "success", map[string]string{
		"username": user.Username,
		"email":    user.Email,
	})

	// Return updated user
	resp := AdminUserResponse{
		ID:          formatUUID(user.ID),
		Username:    user.Username,
		Email:       user.Email,
		Role:        user.Role,
		IsActive:    user.IsActive,
		CreatedAt:   user.CreatedAt.Time,
		LastLoginAt: timePtr(user.LastLoginAt),
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// ActivateUser handles POST /api/v1/admin/users/{id}/activate - Reactivate user
func (h *AdminUsersHandler) ActivateUser(w http.ResponseWriter, r *http.Request) {
	// Extract and validate UUID from path
	idStr := pathParam(r, "id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "User ID is required", nil, h.env)
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(idStr); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid user ID format", err, h.env)
		return
	}

	// Get admin username for audit log
	adminUsername := "unknown"
	if claims := middleware.AdminClaims(r); claims != nil {
		adminUsername = claims.Subject
	}

	// Activate user
	if err := h.userService.ActivateUser(r.Context(), userID, adminUsername); err != nil {
		if errors.Is(err, users.ErrUserNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "User not found", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to activate user", err, h.env)
		return
	}

	// Get updated user
	user, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get user", err, h.env)
		return
	}

	// Audit log (already logged in service)
	h.auditLogger.LogFromRequest(r, "user.activated", "user", formatUUID(userID), "success", map[string]string{
		"username": user.Username,
		"email":    user.Email,
	})

	// Return updated user
	resp := AdminUserResponse{
		ID:          formatUUID(user.ID),
		Username:    user.Username,
		Email:       user.Email,
		Role:        user.Role,
		IsActive:    user.IsActive,
		CreatedAt:   user.CreatedAt.Time,
		LastLoginAt: timePtr(user.LastLoginAt),
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// ResendInvitation handles POST /api/v1/admin/users/{id}/resend-invitation - Resend invitation email
func (h *AdminUsersHandler) ResendInvitation(w http.ResponseWriter, r *http.Request) {
	// Extract and validate UUID from path
	idStr := pathParam(r, "id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "User ID is required", nil, h.env)
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(idStr); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid user ID format", err, h.env)
		return
	}

	// Get admin username for audit log
	adminUsername := "unknown"
	if claims := middleware.AdminClaims(r); claims != nil {
		adminUsername = claims.Subject
	}

	// Resend invitation
	if err := h.userService.ResendInvitation(r.Context(), userID, adminUsername); err != nil {
		if errors.Is(err, users.ErrUserNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "User not found", err, h.env)
			return
		}
		if errors.Is(err, users.ErrUserAlreadyActive) {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "User is already active", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to resend invitation", err, h.env)
		return
	}

	// Audit log (already logged in service)
	h.auditLogger.LogFromRequest(r, "user.invitation_resent", "user", formatUUID(userID), "success", nil)

	resp := MessageResponse{
		Message: "Invitation email has been resent successfully",
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// GetUserActivity handles GET /api/v1/admin/users/{id}/activity - Get user activity audit log
func (h *AdminUsersHandler) GetUserActivity(w http.ResponseWriter, r *http.Request) {
	// Extract and validate UUID from path
	idStr := pathParam(r, "id")
	if idStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "User ID is required", nil, h.env)
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(idStr); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid user ID format", err, h.env)
		return
	}

	// Verify user exists
	_, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, users.ErrUserNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "User not found", err, h.env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to get user", err, h.env)
		return
	}

	// TODO: Implement audit log querying when audit storage is implemented
	// For now, return empty activity log
	// Query params to support: ?event_type=...&limit=50&cursor=...

	resp := map[string]any{
		"items":       []map[string]any{},
		"next_cursor": nil,
		"message":     "Audit log storage not yet implemented",
	}

	writeJSON(w, http.StatusOK, resp, contentTypeFromRequest(r))
}

// Helper functions

// mapUserError maps domain errors to HTTP status codes and problem types
func mapUserError(err error) (status int, problemType, title string) {
	switch {
	case errors.Is(err, users.ErrEmailTaken):
		return http.StatusConflict, "https://sel.events/problems/conflict", "Email already taken"
	case errors.Is(err, users.ErrUsernameTaken):
		return http.StatusConflict, "https://sel.events/problems/conflict", "Username already taken"
	case errors.Is(err, users.ErrUserNotFound):
		return http.StatusNotFound, "https://sel.events/problems/not-found", "User not found"
	case errors.Is(err, users.ErrInvalidToken):
		return http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid or expired invitation token"
	case errors.Is(err, pgx.ErrNoRows):
		return http.StatusNotFound, "https://sel.events/problems/not-found", "Not found"
	default:
		return http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error"
	}
}

// formatUUID converts a pgtype.UUID to a string, returning empty string if invalid
func formatUUID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	// Format as standard UUID string: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}

// timePtr converts a pgtype.Timestamptz to a *time.Time
func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
