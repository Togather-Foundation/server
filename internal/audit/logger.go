package audit

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// Entry represents a single audit log entry with structured fields
type Entry struct {
	Timestamp    time.Time         `json:"timestamp"`
	Action       string            `json:"action"`
	AdminUser    string            `json:"admin_user"`
	ResourceType string            `json:"resource_type,omitempty"`
	ResourceID   string            `json:"resource_id,omitempty"`
	IPAddress    string            `json:"ip_address"`
	Status       string            `json:"status"` // "success" or "failure"
	Details      map[string]string `json:"details,omitempty"`
}

// Logger provides structured audit logging for admin operations
type Logger struct {
	output *log.Logger
}

// NewLogger creates a new audit logger
func NewLogger() *Logger {
	return &Logger{
		output: log.New(log.Writer(), "[AUDIT] ", 0),
	}
}

// Log writes an audit entry to the log output
func (l *Logger) Log(entry Entry) {
	// Set timestamp if not provided
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("ERROR: Failed to marshal audit entry: %v", err)
		return
	}

	// Write to output
	l.output.Println(string(data))
}

// LogSuccess logs a successful admin operation
func (l *Logger) LogSuccess(action, adminUser, resourceType, resourceID, ipAddress string, details map[string]string) {
	l.Log(Entry{
		Action:       action,
		AdminUser:    adminUser,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPAddress:    ipAddress,
		Status:       "success",
		Details:      details,
	})
}

// LogFailure logs a failed admin operation
func (l *Logger) LogFailure(action, adminUser, ipAddress string, details map[string]string) {
	l.Log(Entry{
		Action:    action,
		AdminUser: adminUser,
		IPAddress: ipAddress,
		Status:    "failure",
		Details:   details,
	})
}

// extractClientIP gets the client IP from request headers or RemoteAddr
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (set by reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first
		return xff
	}

	// Check X-Real-IP header (alternative proxy header)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// LogFromRequest is a helper to log an action from an HTTP request context
// It extracts the admin username from the JWT claims and IP from request headers
func (l *Logger) LogFromRequest(r *http.Request, action, resourceType, resourceID, status string, details map[string]string) {
	// Extract admin username from context (set by JWT middleware)
	adminUser := "unknown"
	if claims, ok := r.Context().Value("claims").(map[string]interface{}); ok {
		if username, ok := claims["username"].(string); ok {
			adminUser = username
		}
	}

	ipAddress := extractClientIP(r)

	l.Log(Entry{
		Action:       action,
		AdminUser:    adminUser,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPAddress:    ipAddress,
		Status:       status,
		Details:      details,
	})
}

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const auditLoggerKey contextKey = "auditLogger"

// WithLogger adds an audit logger to the request context
func WithLogger(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, auditLoggerKey, logger)
}

// FromContext retrieves the audit logger from the request context
func FromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(auditLoggerKey).(*Logger); ok {
		return logger
	}
	// Return a default logger if not found in context
	return NewLogger()
}
