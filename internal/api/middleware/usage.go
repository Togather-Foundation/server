package middleware

import (
	"net/http"

	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// usageResponseWriter wraps http.ResponseWriter to capture the status code
type usageResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *usageResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *usageResponseWriter) Write(p []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

// UsageTracking records API key usage (request counts and error counts) to the usage recorder.
// It must be placed after AgentAuth middleware in the chain so that API key info is available in context.
func UsageTracking(recorder *developers.UsageRecorder, logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract API key from context (set by AgentAuth middleware)
			key := AgentKey(r)
			if key == nil {
				// No API key in context - skip usage tracking (might be admin auth or public endpoint)
				next.ServeHTTP(w, r)
				return
			}

			// Parse the API key ID
			apiKeyID, err := uuid.Parse(key.ID)
			if err != nil {
				logger.Warn().
					Err(err).
					Str("api_key_id", key.ID).
					Msg("failed to parse API key ID for usage tracking")
				next.ServeHTTP(w, r)
				return
			}

			// Wrap the response writer to capture status code
			wrapped := &usageResponseWriter{
				ResponseWriter: w,
				statusCode:     0,
			}

			// Call the next handler
			next.ServeHTTP(wrapped, r)

			// Record usage after the handler completes
			isError := wrapped.statusCode >= 400
			recorder.RecordRequest(apiKeyID, isError)
		})
	}
}
