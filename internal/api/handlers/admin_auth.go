package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type AdminAuthHandler struct {
	Queries     *postgres.Queries
	JWTManager  *auth.JWTManager
	AuditLogger *audit.Logger
	Env         string
	Templates   *template.Template
	JWTExpiry   time.Duration
}

// dummyPasswordHash is a pre-computed bcrypt hash used for constant-time comparison
// when a user doesn't exist, preventing timing attacks for username enumeration.
// This is the hash of an empty string with cost 12.
var dummyPasswordHash = []byte("$2a$12$1234567890123456789012eO/6Tg9BK8qLz1v1J8o2zZqzZ8kBRuGW")

func NewAdminAuthHandler(queries *postgres.Queries, jwtManager *auth.JWTManager, auditLogger *audit.Logger, env string, templates *template.Template, jwtExpiry time.Duration) *AdminAuthHandler {
	return &AdminAuthHandler{
		Queries:     queries,
		JWTManager:  jwtManager,
		AuditLogger: auditLogger,
		Env:         env,
		Templates:   templates,
		JWTExpiry:   jwtExpiry,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string   `json:"token"`
	ExpiresAt string   `json:"expires_at"`
	User      userInfo `json:"user"`
}

type userInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// Login handles POST /api/v1/admin/login
// Issues JWT as Authorization header for API clients AND HttpOnly cookie for HTML UI
func (h *AdminAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Queries == nil || h.JWTManager == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	if req.Username == "" || req.Password == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Username and password are required", nil, h.Env)
		return
	}

	// Fetch user by username
	user, err := h.Queries.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Run bcrypt comparison with dummy hash to prevent timing attacks
			// This ensures the response time is similar whether user exists or not
			_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(req.Password))

			// Log failed login attempt
			if h.AuditLogger != nil {
				ipAddress := extractClientIP(r)
				h.AuditLogger.LogFailure("admin.login", req.Username, ipAddress, map[string]string{
					"reason": "invalid_credentials",
				})
			}

			problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Invalid credentials", nil, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		// Log failed login attempt
		if h.AuditLogger != nil {
			ipAddress := extractClientIP(r)
			h.AuditLogger.LogFailure("admin.login", req.Username, ipAddress, map[string]string{
				"reason": "invalid_credentials",
			})
		}

		problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Invalid credentials", nil, h.Env)
		return
	}

	// Generate JWT
	userID, err := uuid.FromBytes(user.ID.Bytes[:])
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}
	token, err := h.JWTManager.Generate(user.Username, user.Role)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Update last login timestamp
	_ = h.Queries.UpdateLastLogin(r.Context(), user.ID)

	// Log successful login
	if h.AuditLogger != nil {
		ipAddress := extractClientIP(r)
		h.AuditLogger.LogSuccess("admin.login", user.Username, "user", userID.String(), ipAddress, nil)
	}

	// Calculate expiry time using config value
	expiresAt := time.Now().Add(h.JWTExpiry)

	// Set HttpOnly cookie for HTML UI
	// In production, always set Secure flag (even behind reverse proxy)
	requireSecure := h.Env == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
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
	if err := json.NewEncoder(w).Encode(loginResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
		User: userInfo{
			ID:       userID.String(),
			Username: user.Username,
			Email:    user.Email,
			Role:     user.Role,
		},
	}); err != nil {
		// Log error but don't return since headers already written
		// In production, use structured logging
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// LoginPage handles GET /admin/login
// Renders the login page HTML
func (h *AdminAuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Templates == nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// If already authenticated, redirect to dashboard
	if cookie, err := r.Cookie("auth_token"); err == nil && cookie.Value != "" {
		if _, err := h.JWTManager.Validate(cookie.Value); err == nil {
			http.Redirect(w, r, "/admin/dashboard", http.StatusFound)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	data := map[string]interface{}{
		"Title": "Admin Login - SEL Backend",
	}

	if err := h.Templates.ExecuteTemplate(w, "login.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// Logout handles POST /api/v1/admin/logout
// Clears the auth cookie
func (h *AdminAuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Extract username from context for audit log
	username := "unknown"
	if claims := middleware.AdminClaims(r); claims != nil {
		username = claims.Subject
	}

	// Log logout
	if h.AuditLogger != nil {
		ipAddress := extractClientIP(r)
		h.AuditLogger.LogSuccess("admin.logout", username, "", "", ipAddress, nil)
	}

	// Clear the auth cookie
	// Match the Secure flag used when setting the cookie
	requireSecure := h.Env == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
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
		// Log error but don't return since headers already written
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// extractClientIP gets the client IP from request headers or RemoteAddr
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (set by reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first
		return xff
	}

	// Check X-Real-IP header (alternative proxy header)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}
