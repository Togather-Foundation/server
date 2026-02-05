package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/users"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

// UserInvitationService defines the interface for user invitation operations
type UserInvitationService interface {
	AcceptInvitation(ctx context.Context, token, password string) (postgres.GetUserByIDRow, error)
}

// AuditLogger defines the interface for audit logging
type AuditLogger interface {
	LogSuccess(action, adminUser, resourceType, resourceID, ipAddress string, details map[string]string)
	LogFailure(action, adminUser, ipAddress string, details map[string]string)
}

// InvitationsHandler handles public (unauthenticated) invitation acceptance
type InvitationsHandler struct {
	userService UserInvitationService
	auditLogger AuditLogger
	env         string
}

// NewInvitationsHandler creates a new invitations handler
func NewInvitationsHandler(userService UserInvitationService, auditLogger AuditLogger, env string) *InvitationsHandler {
	return &InvitationsHandler{
		userService: userService,
		auditLogger: auditLogger,
		env:         env,
	}
}

// AcceptInvitationRequest represents the request body for accepting an invitation
type AcceptInvitationRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// AcceptInvitationResponse represents the response after successfully accepting an invitation
type AcceptInvitationResponse struct {
	Message string                 `json:"message"`
	User    InvitationUserResponse `json:"user"`
}

// InvitationUserResponse represents minimal user information in the invitation acceptance response
type InvitationUserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// AcceptInvitation handles POST /api/v1/accept-invitation
// This is a PUBLIC endpoint (no authentication required)
func (h *InvitationsHandler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req AcceptInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.env)
		return
	}

	// Validate required fields
	if req.Token == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Token is required", nil, h.env)
		return
	}
	if req.Password == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Password is required", nil, h.env)
		return
	}

	// Call service to accept invitation
	user, err := h.userService.AcceptInvitation(r.Context(), req.Token, req.Password)
	if err != nil {
		// Extract client IP for audit logging
		clientIP := extractClientIP(r)

		// Map domain errors to HTTP status codes
		switch {
		case errors.Is(err, users.ErrInvalidToken):
			// Log failed attempt (don't log full token for security)
			if h.auditLogger != nil {
				tokenPreview := req.Token
				if len(tokenPreview) > 8 {
					tokenPreview = tokenPreview[:8] + "..."
				}
				h.auditLogger.LogFailure(
					"user.invitation_accept_failed",
					"anonymous",
					clientIP,
					map[string]string{
						"reason":        "invalid_or_expired_token",
						"token_preview": tokenPreview,
					},
				)
			}
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/invalid-invitation", "Invalid or Expired Invitation", nil, h.env,
				problem.WithDetail("The invitation token is invalid or has expired."))
			return

		case errors.Is(err, users.ErrPasswordTooShort), errors.Is(err, users.ErrPasswordTooLong), errors.Is(err, users.ErrPasswordTooWeak):
			// Log failed attempt due to weak password
			if h.auditLogger != nil {
				h.auditLogger.LogFailure(
					"user.invitation_accept_failed",
					"anonymous",
					clientIP,
					map[string]string{
						"reason": "weak_password",
					},
				)
			}
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/weak-password", "Password Does Not Meet Requirements", nil, h.env,
				problem.WithDetail(err.Error()))
			return

		default:
			// Log internal error
			if h.auditLogger != nil {
				h.auditLogger.LogFailure(
					"user.invitation_accept_failed",
					"anonymous",
					clientIP,
					map[string]string{
						"reason": "internal_error",
					},
				)
			}
			problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Internal Server Error", err, h.env,
				problem.WithDetail("An unexpected error occurred while processing your invitation."))
			return
		}
	}

	// Success! The user service has already handled audit logging for successful acceptance
	// Convert user to response format
	userID := formatUUID(user.ID)

	// Return success response with user info
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := AcceptInvitationResponse{
		Message: "Invitation accepted successfully. You can now log in.",
		User: InvitationUserResponse{
			ID:       userID,
			Username: user.Username,
			Email:    user.Email,
			Role:     user.Role,
		},
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Log error but headers already sent
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to encode response", err, h.env)
	}
}
