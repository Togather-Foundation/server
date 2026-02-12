package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/auth/oauth"
	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/rs/zerolog"
)

// DeveloperOAuthHandler handles GitHub OAuth authentication for developers
type DeveloperOAuthHandler struct {
	service      *developers.Service
	githubClient *oauth.GitHubClient
	logger       zerolog.Logger
	jwtSecretKey []byte // Derived developer JWT signing key
	jwtExpiry    time.Duration
	issuer       string
	env          string
	auditLogger  AuditLogger
}

// NewDeveloperOAuthHandler creates a new developer OAuth handler.
// jwtSecretKey should be derived using DeriveDeveloperJWTKey for proper domain separation.
func NewDeveloperOAuthHandler(
	service *developers.Service,
	githubClient *oauth.GitHubClient,
	logger zerolog.Logger,
	jwtSecretKey []byte,
	jwtExpiry time.Duration,
	issuer string,
	env string,
	auditLogger AuditLogger,
) *DeveloperOAuthHandler {
	return &DeveloperOAuthHandler{
		service:      service,
		githubClient: githubClient,
		logger:       logger.With().Str("handler", "dev_oauth").Logger(),
		jwtSecretKey: jwtSecretKey,
		jwtExpiry:    jwtExpiry,
		issuer:       issuer,
		env:          env,
		auditLogger:  auditLogger,
	}
}

// GitHubLogin handles GET /auth/github
// Redirects to GitHub OAuth authorization with CSRF state protection
func (h *DeveloperOAuthHandler) GitHubLogin(w http.ResponseWriter, r *http.Request) {
	// Generate random state parameter for CSRF protection
	state, err := oauth.GenerateState()
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to generate oauth state")
		http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
		return
	}

	// Store state in cookie (5 min expiry, HttpOnly, Secure, SameSite=Lax)
	requireSecure := h.env == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   requireSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to GitHub authorization URL
	authURL := h.githubClient.GenerateAuthURL(state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// GitHubCallback handles GET /auth/github/callback
// Completes OAuth flow with developer auto-creation/matching and account linking
func (h *DeveloperOAuthHandler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	// Validate state parameter (CSRF protection)
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		h.logger.Warn().Msg("oauth state cookie missing")
		http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
		return
	}

	stateParam := r.URL.Query().Get("state")
	if stateParam == "" || stateParam != stateCookie.Value {
		h.logger.Warn().Str("expected", stateCookie.Value).Str("received", stateParam).Msg("oauth state mismatch")
		http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
		return
	}

	// Clear state cookie immediately after validation
	clearStateCookie := &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.env == "production",
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, clearStateCookie)

	// Check for OAuth errors from GitHub
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		h.logger.Warn().Str("error", errParam).Str("description", r.URL.Query().Get("error_description")).Msg("github oauth error")
		http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
		return
	}

	// Exchange authorization code for access token
	code := r.URL.Query().Get("code")
	if code == "" {
		h.logger.Warn().Msg("oauth code parameter missing")
		http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
		return
	}

	accessToken, err := h.githubClient.ExchangeCode(r.Context(), code)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to exchange oauth code")
		http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
		return
	}

	// Fetch GitHub user profile
	githubUser, err := h.githubClient.FetchUserProfile(r.Context(), accessToken)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to fetch github user profile")
		http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
		return
	}

	// Require email (GitHub may not provide email if user has it private and we don't have permission)
	if githubUser.Email == "" {
		h.logger.Warn().Int64("github_id", githubUser.ID).Str("login", githubUser.Login).Msg("github user has no email")
		http.Redirect(w, r, "/dev/login?error=no_email", http.StatusFound)
		return
	}

	clientIP := extractClientIP(r)

	// Auto-creation/matching logic:
	// 1. If github_id matches existing developer → log them in
	// 2. If email matches existing developer without github_id → link GitHub account
	// 3. Otherwise → create new developer record
	var developer *developers.Developer

	// Try to find by GitHub ID first
	existingByGitHub, err := h.service.GetDeveloperByGitHubID(r.Context(), githubUser.ID)
	if err == nil && existingByGitHub != nil {
		// Case 1: Existing developer with this GitHub ID - just log them in
		developer = existingByGitHub
		h.logger.Info().Str("developer_id", developer.ID.String()).Int64("github_id", githubUser.ID).Msg("developer logged in via github")
	} else {
		// Try to find by email
		existingByEmail, err := h.service.GetDeveloperByEmail(r.Context(), githubUser.Email)
		if err == nil && existingByEmail != nil {
			// Case 2: Email matches existing developer without GitHub ID - link accounts
			if existingByEmail.GitHubID == nil {
				// Link GitHub account to existing developer
				githubID := githubUser.ID
				githubUsername := githubUser.Login
				updateParams := developers.UpdateDeveloperParams{
					GitHubID:       &githubID,
					GitHubUsername: &githubUsername,
				}
				updatedDev, err := h.service.UpdateDeveloper(r.Context(), existingByEmail.ID, updateParams)
				if err != nil {
					h.logger.Error().Err(err).Str("developer_id", existingByEmail.ID.String()).Msg("failed to link github account")
					http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
					return
				}
				developer = updatedDev
				h.logger.Info().Str("developer_id", developer.ID.String()).Int64("github_id", githubUser.ID).Msg("github account linked to existing developer")
			} else {
				// Email exists but already linked to a different GitHub account
				h.logger.Warn().
					Str("developer_id", existingByEmail.ID.String()).
					Int64("existing_github_id", *existingByEmail.GitHubID).
					Int64("attempted_github_id", githubUser.ID).
					Msg("email already linked to different github account")
				if h.auditLogger != nil {
					h.auditLogger.LogFailure("dev.oauth.callback", githubUser.Email, clientIP, map[string]string{
						"reason": "email_conflict",
					})
				}
				http.Redirect(w, r, "/dev/login?error=account_conflict", http.StatusFound)
				return
			}
		} else {
			// Case 3: No existing developer - create new account
			githubID := githubUser.ID
			githubUsername := githubUser.Login
			name := githubUser.Name
			if name == "" {
				name = githubUser.Login // Fallback to username if name not provided
			}

			createParams := developers.CreateDeveloperParams{
				Email:          githubUser.Email,
				Name:           name,
				Password:       "", // No password for OAuth-only accounts
				GitHubID:       &githubID,
				GitHubUsername: &githubUsername,
				MaxKeys:        developers.DefaultMaxKeys,
			}

			newDev, err := h.service.CreateDeveloper(r.Context(), createParams)
			if err != nil {
				if errors.Is(err, developers.ErrEmailTaken) {
					// Race condition: email was taken between checks
					h.logger.Warn().Str("email", githubUser.Email).Msg("email taken during developer creation")
					http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
					return
				}
				h.logger.Error().Err(err).Msg("failed to create developer from github oauth")
				http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
				return
			}

			developer = newDev
			h.logger.Info().Str("developer_id", developer.ID.String()).Int64("github_id", githubUser.ID).Msg("new developer created via github oauth")
		}
	}

	// Check if developer is active
	if !developer.IsActive {
		h.logger.Warn().Str("developer_id", developer.ID.String()).Msg("inactive developer attempted github login")
		if h.auditLogger != nil {
			h.auditLogger.LogFailure("dev.oauth.callback", developer.Email, clientIP, map[string]string{
				"reason": "account_inactive",
			})
		}
		http.Redirect(w, r, "/dev/login?error=account_inactive", http.StatusFound)
		return
	}

	// Generate JWT token
	expiryHours := int(h.jwtExpiry.Hours())
	token, expiresAt, err := auth.GenerateDeveloperToken(developer.ID, developer.Email, developer.Name, h.jwtSecretKey, expiryHours, h.issuer)
	if err != nil {
		h.logger.Error().Err(err).Str("developer_id", developer.ID.String()).Msg("failed to generate JWT")
		http.Redirect(w, r, "/dev/login?error=oauth_failed", http.StatusFound)
		return
	}

	// Log successful OAuth login
	if h.auditLogger != nil {
		h.auditLogger.LogSuccess("dev.oauth.callback", developer.Email, "developer", developer.ID.String(), clientIP, map[string]string{
			"oauth_provider": "github",
		})
	}

	// Update last login timestamp
	if err := h.service.UpdateDeveloperLastLogin(r.Context(), developer.ID); err != nil {
		h.logger.Warn().Err(err).Str("developer_id", developer.ID.String()).Msg("failed to update last login")
		// Don't fail - just log the warning
	}

	// Set dev_auth_token cookie
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

	// Redirect to developer dashboard
	http.Redirect(w, r, "/dev/dashboard", http.StatusFound)
}
