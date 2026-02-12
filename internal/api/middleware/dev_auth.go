package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/Togather-Foundation/server/internal/auth"
)

// DevAuthCookieName is the cookie name for developer authentication
const DevAuthCookieName = "dev_auth_token"

type contextKeyDeveloper string

const developerClaimsKey contextKeyDeveloper = "developerClaims"

// DevCookieAuth validates developer JWT tokens from cookies.
// This middleware is separate from AdminCookieAuth to prevent privilege escalation.
// It explicitly rejects tokens where type != "developer".
// jwtSecretKey should be derived using DeriveDeveloperJWTKey for proper domain separation.
func DevCookieAuth(jwtSecretKey []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(jwtSecretKey) == 0 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Reject Bearer tokens - developers must use cookies
			if strings.HasPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Read developer auth cookie
			cookie, err := r.Cookie(DevAuthCookieName)
			if err != nil || strings.TrimSpace(cookie.Value) == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Validate developer token
			claims, err := auth.ValidateDeveloperToken(cookie.Value, jwtSecretKey)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Double-check token type (ValidateDeveloperToken already does this,
			// but we add defense in depth)
			if claims.Type != "developer" {
				w.WriteHeader(http.StatusForbidden)
				return
			}

			// Add developer claims to context
			ctx := contextWithDeveloper(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// contextWithDeveloper adds developer claims to a context
func contextWithDeveloper(ctx context.Context, claims *auth.DeveloperClaims) context.Context {
	return context.WithValue(ctx, developerClaimsKey, claims)
}

// ContextWithDeveloper adds developer claims to a context (exported for testing)
func ContextWithDeveloper(ctx context.Context, claims *auth.DeveloperClaims) context.Context {
	return contextWithDeveloper(ctx, claims)
}

// DeveloperFromContext retrieves developer claims from the request context.
// Returns nil if no developer claims are present.
func DeveloperFromContext(ctx context.Context) *auth.DeveloperClaims {
	if ctx == nil {
		return nil
	}
	if claims, ok := ctx.Value(developerClaimsKey).(*auth.DeveloperClaims); ok {
		return claims
	}
	return nil
}

// DeveloperClaims retrieves developer claims from the request.
// This is a convenience wrapper around DeveloperFromContext.
func DeveloperClaims(r *http.Request) *auth.DeveloperClaims {
	if r == nil {
		return nil
	}
	return DeveloperFromContext(r.Context())
}
