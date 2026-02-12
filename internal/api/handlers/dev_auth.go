package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/rs/zerolog"
)

// DeveloperAuthHandler handles developer authentication endpoints
type DeveloperAuthHandler struct {
	service     *developers.Service
	logger      zerolog.Logger
	jwtSecret   string
	jwtExpiry   time.Duration
	issuer      string
	env         string
	auditLogger AuditLogger
}

// NewDeveloperAuthHandler creates a new developer auth handler
func NewDeveloperAuthHandler(
	service *developers.Service,
	logger zerolog.Logger,
	jwtSecret string,
	jwtExpiry time.Duration,
	issuer string,
	env string,
	auditLogger AuditLogger,
) *DeveloperAuthHandler {
	return &DeveloperAuthHandler{
		service:     service,
		logger:      logger.With().Str("handler", "dev_auth").Logger(),
		jwtSecret:   jwtSecret,
		jwtExpiry:   jwtExpiry,
		issuer:      issuer,
		env:         env,
		auditLogger: auditLogger,
	}
}

type devLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type devLoginResponse struct {
	Token     string        `json:"token"`
	ExpiresAt string        `json:"expires_at"`
	Developer developerInfo `json:"developer"`
}

type developerInfo struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Login handles POST /api/v1/dev/login
// Authenticates with email/password and sets dev_auth_token cookie
func (h *DeveloperAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req devLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.env)
		return
	}

	if req.Email == "" || req.Password == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Email and password are required", nil, h.env)
		return
	}

	// Authenticate developer
	developer, err := h.service.AuthenticateDeveloper(r.Context(), req.Email, req.Password)
	if err != nil {
		clientIP := extractClientIP(r)

		// Map domain errors to HTTP status codes
		switch {
		case errors.Is(err, developers.ErrInvalidCredentials):
			if h.auditLogger != nil {
				h.auditLogger.LogFailure("dev.login", req.Email, clientIP, map[string]string{
					"reason": "invalid_credentials",
				})
			}
			problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Invalid credentials", nil, h.env)
			return

		case errors.Is(err, developers.ErrDeveloperInactive):
			if h.auditLogger != nil {
				h.auditLogger.LogFailure("dev.login", req.Email, clientIP, map[string]string{
					"reason": "account_inactive",
				})
			}
			problem.Write(w, r, http.StatusForbidden, "https://sel.events/problems/forbidden", "Account is inactive", nil, h.env)
			return

		default:
			if h.auditLogger != nil {
				h.auditLogger.LogFailure("dev.login", req.Email, clientIP, map[string]string{
					"reason": "internal_error",
				})
			}
			problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.env)
			return
		}
	}

	// Generate JWT
	expiryHours := int(h.jwtExpiry.Hours())
	token, expiresAt, err := auth.GenerateDeveloperToken(developer.ID, developer.Email, developer.Name, h.jwtSecret, expiryHours, h.issuer)
	if err != nil {
		h.logger.Error().Err(err).Str("developer_id", developer.ID.String()).Msg("failed to generate JWT")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.env)
		return
	}

	// Log successful login
	if h.auditLogger != nil {
		clientIP := extractClientIP(r)
		h.auditLogger.LogSuccess("dev.login", developer.Email, "developer", developer.ID.String(), clientIP, nil)
	}

	// Set HttpOnly cookie for browser sessions
	requireSecure := h.env == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.DevAuthCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   requireSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// Return JSON response with token for API clients
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(devLoginResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
		Developer: developerInfo{
			ID:    developer.ID.String(),
			Email: developer.Email,
			Name:  developer.Name,
		},
	}); err != nil {
		h.logger.Error().Err(err).Msg("failed to encode login response")
	}
}

// Logout handles POST /api/v1/dev/logout
// Clears the dev_auth_token cookie
func (h *DeveloperAuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Extract developer from context for audit log
	developerEmail := "unknown"
	if claims := middleware.DeveloperClaims(r); claims != nil {
		developerEmail = claims.Email
	}

	// Log logout
	if h.auditLogger != nil {
		clientIP := extractClientIP(r)
		h.auditLogger.LogSuccess("dev.logout", developerEmail, "", "", clientIP, nil)
	}

	// Clear the cookie
	requireSecure := h.env == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.DevAuthCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requireSecure,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "logged out"}); err != nil {
		h.logger.Error().Err(err).Msg("failed to encode logout response")
	}
}

type acceptInvitationRequest struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type acceptInvitationResponse struct {
	Token     string        `json:"token"`
	Developer developerInfo `json:"developer"`
}

// AcceptInvitation handles POST /api/v1/dev/accept-invitation
// Accepts invitation, sets password, and returns JWT token
func (h *DeveloperAuthHandler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var req acceptInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.env)
		return
	}

	if req.Token == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Token is required", nil, h.env)
		return
	}
	if req.Name == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Name is required", nil, h.env)
		return
	}
	if req.Password == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Password is required", nil, h.env)
		return
	}

	// Accept invitation
	developer, err := h.service.AcceptInvitation(r.Context(), req.Token, req.Name, req.Password)
	if err != nil {
		clientIP := extractClientIP(r)

		// Map domain errors to HTTP status codes
		switch {
		case errors.Is(err, developers.ErrInvalidToken):
			if h.auditLogger != nil {
				h.auditLogger.LogFailure("dev.accept_invitation", "anonymous", clientIP, map[string]string{
					"reason": "invalid_token",
				})
			}
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/invalid-invitation", "Invalid or expired invitation", nil, h.env,
				problem.WithDetail("The invitation token is invalid or has expired."))
			return

		case errors.Is(err, developers.ErrPasswordTooShort), errors.Is(err, developers.ErrPasswordTooLong):
			if h.auditLogger != nil {
				h.auditLogger.LogFailure("dev.accept_invitation", "anonymous", clientIP, map[string]string{
					"reason": "weak_password",
				})
			}
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/weak-password", "Password does not meet requirements", nil, h.env,
				problem.WithDetail(err.Error()))
			return

		default:
			if h.auditLogger != nil {
				h.auditLogger.LogFailure("dev.accept_invitation", "anonymous", clientIP, map[string]string{
					"reason": "internal_error",
				})
			}
			problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.env)
			return
		}
	}

	// Generate JWT
	expiryHours := int(h.jwtExpiry.Hours())
	token, _, err := auth.GenerateDeveloperToken(developer.ID, developer.Email, developer.Name, h.jwtSecret, expiryHours, h.issuer)
	if err != nil {
		h.logger.Error().Err(err).Str("developer_id", developer.ID.String()).Msg("failed to generate JWT")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.env)
		return
	}

	// Log successful acceptance
	if h.auditLogger != nil {
		clientIP := extractClientIP(r)
		h.auditLogger.LogSuccess("dev.accept_invitation", developer.Email, "developer", developer.ID.String(), clientIP, nil)
	}

	// Return JWT token
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(acceptInvitationResponse{
		Token: token,
		Developer: developerInfo{
			ID:    developer.ID.String(),
			Email: developer.Email,
			Name:  developer.Name,
		},
	}); err != nil {
		h.logger.Error().Err(err).Msg("failed to encode accept invitation response")
	}
}
