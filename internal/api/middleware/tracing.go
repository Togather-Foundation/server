package middleware

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/Togather-Foundation/server/internal/api"

// Tracing creates spans for HTTP requests with automatic trace context propagation.
// This middleware should be applied early in the middleware chain (before request logging)
// to ensure spans capture the full request lifecycle.
//
// Span attributes include:
//   - http.method: HTTP method (GET, POST, etc.)
//   - http.url: Full request URL
//   - http.status_code: Response status code
//   - http.route: Matched route pattern (e.g., "/api/v1/events/{id}")
//   - http.user_agent: Client user agent
//   - request_id: Correlation ID from CorrelationID middleware
//
// The span context is propagated via W3C Trace Context headers (traceparent, tracestate).
func Tracing(next http.Handler) http.Handler {
	tracer := otel.Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract trace context from incoming request headers
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Start span with semantic conventions for HTTP requests
		spanName := r.Method + " " + r.URL.Path
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPMethod(r.Method),
				semconv.HTTPURL(r.URL.String()),
				semconv.HTTPRoute(r.URL.Path),
				attribute.String("http.user_agent", r.UserAgent()),
				semconv.HTTPScheme(schemeFromRequest(r)),
				semconv.NetHostName(r.Host),
			),
		)
		defer span.End()

		// Add correlation ID to span if available (set by CorrelationID middleware)
		if requestID := GetRequestID(ctx); requestID != "" {
			span.SetAttributes(attribute.String("request_id", requestID))
		}

		// Wrap response writer to capture status code
		ww := &tracingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default to 200 if not explicitly set
		}

		// Process request
		next.ServeHTTP(ww, r.WithContext(ctx))

		// Record response status code
		span.SetAttributes(semconv.HTTPStatusCode(ww.statusCode))

		// Mark span as error if status code indicates failure
		if ww.statusCode >= 400 {
			span.SetStatus(codes.Error, http.StatusText(ww.statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}
	})
}

// tracingResponseWriter wraps http.ResponseWriter to capture the status code
type tracingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *tracingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *tracingResponseWriter) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(b)
}

// schemeFromRequest determines the HTTP scheme (http or https) from the request
func schemeFromRequest(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}
	return "http"
}
