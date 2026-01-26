package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const (
	// RequestIDKey is the context key for the request correlation ID
	RequestIDKey contextKey = "request_id"
)

// CorrelationID middleware adds a correlation ID to each request and injects it into the logger
func CorrelationID(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if request already has X-Request-ID header (from proxy/load balancer)
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				// Generate new UUID if not present
				requestID = uuid.New().String()
			}

			// Add to response headers for client tracing
			w.Header().Set("X-Request-ID", requestID)

			// Create logger with correlation ID
			reqLogger := logger.With().Str("request_id", requestID).Logger()

			// Add both logger and request ID to context
			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
			ctx = reqLogger.WithContext(ctx)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetRequestID extracts the request ID from context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// LoggerFromContext extracts the logger from context, or returns a disabled logger
func LoggerFromContext(ctx context.Context) *zerolog.Logger {
	logger := zerolog.Ctx(ctx)
	if logger.GetLevel() == zerolog.Disabled {
		// Return a basic logger if none in context (shouldn't happen in normal flow)
		noop := zerolog.Nop()
		return &noop
	}
	return logger
}
