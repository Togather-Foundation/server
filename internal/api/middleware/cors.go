package middleware

import (
	"net/http"
	"net/url"
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
// Public reads (cfg.PublicReadsOpen, default on): GET/HEAD requests to the public read
// endpoints (see isPublicReadPath) from ANY origin, including "null" (file:// pages),
// get Access-Control-Allow-Origin: * and no Allow-Credentials header. Public reads
// need no cookies or auth header, and a credential-less wildcard cannot be abused
// to make authenticated requests.
//
// Everything else (admin, auth, dev, write endpoints) follows the whitelist:
//   - Access-Control-Allow-Origin: the matched origin (reflected, with Vary: Origin)
//   - Access-Control-Allow-Credentials: true ONLY for origins explicitly listed in
//     CORS_ALLOWED_ORIGINS, or loopback origins in allow-all (dev) mode. Reflecting an
//     arbitrary origin together with credentials would let any website make
//     authenticated requests as a logged-in user.
//
// Other headers set:
//   - Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS, PATCH
//   - Access-Control-Allow-Headers: Content-Type, Authorization, Idempotency-Key, Accept
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

			// Public reads: open to any origin, never with credentials.
			if cfg.PublicReadsOpen && isPublicReadRequest(r) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", "*")
				h.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
				h.Set("Access-Control-Allow-Headers", allowHeaders)
				h.Set("Access-Control-Expose-Headers", exposeHeaders)
				h.Set("Access-Control-Max-Age", "86400")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Determine if origin is allowed
			allowedOrigin := ""
			originRejected := false
			whitelisted := isOriginAllowed(origin, cfg.AllowedOrigins)
			if cfg.AllowAllOrigins || whitelisted {
				allowedOrigin = origin
			} else {
				originRejected = true
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
				h := w.Header()
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Allow-Origin", allowedOrigin)
				if whitelisted || (cfg.AllowAllOrigins && isLoopbackOrigin(origin)) {
					h.Set("Access-Control-Allow-Credentials", "true")
				}
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
				h.Set("Access-Control-Allow-Headers", allowHeaders)
				h.Set("Access-Control-Expose-Headers", exposeHeaders)
				h.Set("Access-Control-Max-Age", "86400") // 24 hours
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

const (
	allowHeaders  = "Content-Type, Authorization, Idempotency-Key, Accept, X-Request-ID"
	exposeHeaders = "X-Request-ID, Retry-After"
)

// isPublicReadRequest reports whether r is a cross-origin read of a public endpoint:
// a GET/HEAD, or a preflight announcing one, for a path in isPublicReadPath.
func isPublicReadRequest(r *http.Request) bool {
	if !isPublicReadPath(r.URL.Path) {
		return false
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return true
	case http.MethodOptions:
		// A preflight for a write on a public path (POST /api/v1/events) is not a read.
		switch strings.ToUpper(r.Header.Get("Access-Control-Request-Method")) {
		case http.MethodGet, http.MethodHead:
			return true
		}
	}
	return false
}

// publicReadExact are public read endpoints with no path parameter.
var publicReadExact = map[string]bool{
	"/api/v1/events":        true,
	"/api/v1/events.ics":    true,
	"/api/v1/places":        true,
	"/api/v1/organizations": true,
	"/api/v1/openapi.json":  true,
	"/api/v1/openapi.yaml":  true,
	"/api/v1/stats":         true,
}

// publicReadCollections are public collections whose items are readable at
// /{collection}/{id}. Events also expose /{id}/ics.
var publicReadCollections = []string{
	"/api/v1/events/",
	"/api/v1/places/",
	"/api/v1/organizations/",
}

// isPublicReadPath is an allowlist of the unauthenticated read endpoints. Keep it
// narrow: anything not listed here (admin, auth, dev, batch, geocode, feeds) falls
// back to the whitelist.
func isPublicReadPath(path string) bool {
	if publicReadExact[path] {
		return true
	}
	for _, prefix := range publicReadCollections {
		rest, ok := strings.CutPrefix(path, prefix)
		if !ok || rest == "" {
			continue
		}
		if !strings.Contains(rest, "/") {
			return true // /{collection}/{id}
		}
		if prefix == "/api/v1/events/" {
			if id, ok := strings.CutSuffix(rest, "/ics"); ok && id != "" && !strings.Contains(id, "/") {
				return true
			}
		}
	}
	return false
}

// isLoopbackOrigin reports whether origin is http(s)://localhost or 127.0.0.1 on any port.
func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
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
