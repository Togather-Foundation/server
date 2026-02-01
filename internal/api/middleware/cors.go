package middleware

import (
	"net/http"
	"strings"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/rs/zerolog"
)

// CORS handles Cross-Origin Resource Sharing (CORS) for browser-based API clients.
//
// Configuration:
//   - Development: Allows all localhost origins (http://localhost:*, http://127.0.0.1:*)
//   - Production: Requires explicit CORS_ALLOWED_ORIGINS environment variable (comma-separated)
//
// Headers set:
//   - Access-Control-Allow-Origin: Matched origin or * (dev only)
//   - Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS, PATCH
//   - Access-Control-Allow-Headers: Content-Type, Authorization, Idempotency-Key, Accept
//   - Access-Control-Allow-Credentials: true (for cookie-based auth)
//   - Access-Control-Max-Age: 86400 (24 hours preflight cache)
//
// Preflight Requests (OPTIONS):
//
//	Returns 204 No Content with CORS headers
//
// Security Logging:
//   - Logs rejected CORS requests in production mode for security monitoring
//   - Includes origin, path, and method to detect potential attacks
func CORS(cfg config.CORSConfig, logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Skip CORS processing if no Origin header (same-origin requests)
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Determine if origin is allowed
			allowedOrigin := ""
			originRejected := false
			if cfg.AllowAllOrigins {
				// Development mode: allow all origins
				allowedOrigin = origin
			} else {
				// Production mode: check against whitelist
				if isOriginAllowed(origin, cfg.AllowedOrigins) {
					allowedOrigin = origin
				} else {
					originRejected = true
				}
			}

			// Log rejected CORS requests for security monitoring
			if originRejected {
				logger.Warn().
					Str("origin", origin).
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Msg("CORS request rejected: origin not in whitelist")
			}

			// Set CORS headers if origin is allowed
			if allowedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, Accept, X-Request-ID")
				w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, Retry-After")
				w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours
			}

			// Handle preflight OPTIONS requests
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Continue to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// isOriginAllowed checks if the given origin is in the allowed list.
// Performs case-insensitive exact match.
func isOriginAllowed(origin string, allowedOrigins []string) bool {
	origin = strings.ToLower(strings.TrimSpace(origin))
	for _, allowed := range allowedOrigins {
		if strings.ToLower(strings.TrimSpace(allowed)) == origin {
			return true
		}
	}
	return false
}
