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
			// - default-src 'self': Only load resources from same origin
			// - style-src 'self' 'unsafe-inline': Allow external stylesheets + inline styles
			//   (unsafe-inline needed for display:none on modals/hidden elements)
			// - script-src 'self': Only allow scripts from same origin (no inline scripts)
			// - img-src 'self' data:: Allow same-origin images + data URIs
			//   (data: needed for Tabler's inline SVG icons in CSS)
			//
			// Security considerations:
			// - Inline styles ('unsafe-inline') pose minimal XSS risk since user content
			//   is text-only (event descriptions, etc.) and never rendered as HTML attributes
			// - Data URIs for images only allow framework icons, not user-uploaded content
			// - Scripts remain strict (no 'unsafe-inline', no 'unsafe-eval')
			h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:")

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
