package middleware

import (
	"net/http"
)

// SecurityHeaders adds security-related HTTP headers to all responses.
//
// Headers added:
//   - X-Frame-Options: DENY (prevents clickjacking via iframe embedding)
//   - X-Content-Type-Options: nosniff (prevents MIME sniffing attacks)
//   - X-XSS-Protection: 1; mode=block (legacy XSS filter for old browsers)
//   - Referrer-Policy: strict-origin-when-cross-origin (privacy protection)
//   - Content-Security-Policy: default-src 'self' (XSS defense)
//
// Production-only headers (requireHTTPS):
//   - Strict-Transport-Security: max-age=31536000; includeSubDomains; preload (HSTS)
//
// Note: CSP is intentionally permissive for initial launch. Tighten based on actual requirements.
func SecurityHeaders(requireHTTPS bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()

			// Clickjacking protection: prevent iframe embedding
			h.Set("X-Frame-Options", "DENY")

			// MIME sniffing protection: browser must respect Content-Type
			h.Set("X-Content-Type-Options", "nosniff")

			// Legacy XSS filter for older browsers
			h.Set("X-XSS-Protection", "1; mode=block")

			// Referrer policy: limit referrer information leakage
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Content Security Policy: XSS defense
			// All scripts and styles are in external files (no inline code)
			// This prevents XSS attacks via injected inline scripts/styles
			h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'")

			// HSTS: enforce HTTPS in production
			// Only set on HTTPS connections to avoid browser warnings
			if requireHTTPS && r.TLS != nil {
				// max-age=31536000: 1 year
				// includeSubDomains: apply to all subdomains
				// preload: eligible for browser HSTS preload list
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
			}

			next.ServeHTTP(w, r)
		})
	}
}
