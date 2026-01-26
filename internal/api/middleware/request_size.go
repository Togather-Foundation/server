package middleware

import (
	"net/http"
)

const (
	// DefaultMaxBodySize is 1MB for public endpoints
	DefaultMaxBodySize int64 = 1 << 20 // 1MB

	// FederationMaxBodySize is 10MB for federation sync endpoints
	FederationMaxBodySize int64 = 10 << 20 // 10MB

	// AdminMaxBodySize is 5MB for admin endpoints
	AdminMaxBodySize int64 = 5 << 20 // 5MB
)

// RequestSize limits the size of incoming request bodies.
//
// It wraps the request body with http.MaxBytesReader to enforce the limit.
// If the body exceeds maxBytes, it returns 413 Payload Too Large.
//
// Usage:
//
//	// Apply to a specific route
//	http.Handle("/api/v1/events", middleware.RequestSize(1<<20)(eventsHandler))
//
//	// Or use predefined limits
//	http.Handle("/api/v1/admin/events", middleware.AdminRequestSize()(adminHandler))
func RequestSize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Limit request body size
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

			next.ServeHTTP(w, r)
		})
	}
}

// PublicRequestSize limits request bodies to 1MB for public endpoints.
func PublicRequestSize() func(http.Handler) http.Handler {
	return RequestSize(DefaultMaxBodySize)
}

// FederationRequestSize limits request bodies to 10MB for federation endpoints.
func FederationRequestSize() func(http.Handler) http.Handler {
	return RequestSize(FederationMaxBodySize)
}

// AdminRequestSize limits request bodies to 5MB for admin endpoints.
func AdminRequestSize() func(http.Handler) http.Handler {
	return RequestSize(AdminMaxBodySize)
}
