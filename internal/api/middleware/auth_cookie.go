package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/auth"
)

const AdminAuthCookieName = "sel_admin_token"

type contextKeyAuth string

const adminClaimsKey contextKeyAuth = "adminClaims"

func AdminAuthCookie(manager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if manager == nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			cookie, err := r.Cookie(AdminAuthCookieName)
			if err != nil || strings.TrimSpace(cookie.Value) == "" {
				http.Redirect(w, r, "/admin/login", http.StatusFound)
				return
			}

			claims, err := manager.Validate(cookie.Value)
			if err != nil {
				http.Redirect(w, r, "/admin/login", http.StatusFound)
				return
			}

			ctx := r.Context()
			ctx = contextWithAdminClaims(ctx, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func contextWithAdminClaims(ctx context.Context, claims *auth.Claims) context.Context {
	return context.WithValue(ctx, adminClaimsKey, claims)
}

func AdminClaims(r *http.Request) *auth.Claims {
	if r == nil {
		return nil
	}
	if claims, ok := r.Context().Value(adminClaimsKey).(*auth.Claims); ok {
		return claims
	}
	return nil
}

// JWTAuth validates JWT tokens from Authorization header (Bearer tokens)
// Used for admin API routes (/api/v1/admin/*)
func JWTAuth(manager *auth.JWTManager, env string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if manager == nil {
				problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Unauthorized", problem.ErrUnauthorized, env)
				return
			}

			// Extract Bearer token from Authorization header
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if authHeader == "" {
				problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Missing authorization header", problem.ErrUnauthorized, env)
				return
			}

			// Check for Bearer prefix
			if !strings.HasPrefix(authHeader, "Bearer ") {
				problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Invalid authorization format", problem.ErrUnauthorized, env)
				return
			}

			token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			if token == "" {
				problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Missing token", problem.ErrUnauthorized, env)
				return
			}

			// Validate JWT token
			claims, err := manager.Validate(token)
			if err != nil {
				problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Invalid token", err, env)
				return
			}

			// Check role-based access (admin routes require admin role)
			if claims.Role != "admin" {
				problem.Write(w, r, http.StatusForbidden, "https://sel.events/problems/forbidden", "Insufficient permissions", nil, env)
				return
			}

			// Add claims to context
			ctx := contextWithAdminClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type contextKeyAgent string

const agentKey contextKeyAgent = "agentKey"

func AgentAuth(store auth.APIKeyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if store == nil {
				problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Unauthorized", problem.ErrUnauthorized, "")
				return
			}
			key, err := auth.ValidateAPIKey(r.Context(), store, r.Header.Get("Authorization"))
			if err != nil {
				problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Unauthorized", err, "")
				return
			}
			ctx := context.WithValue(r.Context(), agentKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AgentKey(r *http.Request) *auth.APIKey {
	if r == nil {
		return nil
	}
	if key, ok := r.Context().Value(agentKey).(*auth.APIKey); ok {
		return key
	}
	return nil
}
