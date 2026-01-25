package middleware

import (
	"context"
	"net/http"
	"strings"

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
