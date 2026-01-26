package middleware

import (
	"net/http"

	"github.com/gorilla/csrf"
)

// CSRFProtection creates CSRF middleware for protecting cookie-based admin routes
// from cross-site request forgery attacks.
//
// Usage:
//   - Applied to HTML admin pages (GET /admin/*) that render forms
//   - Protects state-changing operations (POST/PUT/DELETE) via cookie-based auth
//   - NOT applied to API endpoints using Bearer token auth (already CSRF-resistant)
//
// The middleware generates and validates CSRF tokens using the double-submit cookie pattern:
//  1. Token embedded in HTML form as hidden field: {{ .csrfToken }}
//  2. Token sent as cookie: _gorilla_csrf
//  3. On form submit, both values must match and be valid
func CSRFProtection(authKey []byte, secure bool) func(http.Handler) http.Handler {
	opts := []csrf.Option{
		csrf.Secure(secure),
		csrf.Path("/"),
		csrf.HttpOnly(true),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.ErrorHandler(http.HandlerFunc(csrfErrorHandler)),
	}

	return csrf.Protect(authKey, opts...)
}

// csrfErrorHandler returns a 403 Forbidden response for CSRF validation failures
func csrfErrorHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusForbidden)
	w.Header().Set("Content-Type", "application/json")
	// Simple JSON error response
	w.Write([]byte(`{"error":"CSRF token validation failed","type":"https://sel.events/problems/csrf-failure","status":403}`))
}

// CSRFToken extracts the CSRF token from the request context for embedding in forms
// Use in HTML templates: <input type="hidden" name="gorilla.csrf.Token" value="{{ .csrfToken }}">
func CSRFToken(r *http.Request) string {
	return csrf.Token(r)
}

// CSRFFieldName returns the name attribute for the CSRF token hidden field
func CSRFFieldName() string {
	return csrf.TemplateTag
}
