package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type AdminAuthHandler struct {
	Queries    *postgres.Queries
	JWTManager *auth.JWTManager
	Env        string
	Templates  *template.Template
}

func NewAdminAuthHandler(queries *postgres.Queries, jwtManager *auth.JWTManager, env string, templates *template.Template) *AdminAuthHandler {
	return &AdminAuthHandler{
		Queries:    queries,
		JWTManager: jwtManager,
		Env:        env,
		Templates:  templates,
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
			problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Invalid credentials", nil, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Invalid credentials", nil, h.Env)
		return
	}

	// Generate JWT
	userID, err := uuid.FromBytes(user.ID.Bytes[:])
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}
	token, err := h.JWTManager.Generate(userID.String(), user.Role)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	// Update last login timestamp
	_ = h.Queries.UpdateLastLogin(r.Context(), user.ID)

	// Calculate expiry time
	expiresAt := time.Now().Add(24 * time.Hour) // TODO: use config value

	// Set HttpOnly cookie for HTML UI
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil, // Only secure in HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	// Return JSON response with token for API clients
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(loginResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
		User: userInfo{
			ID:       userID.String(),
			Username: user.Username,
			Email:    user.Email,
			Role:     user.Role,
		},
	})
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
	// Clear the auth cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}
