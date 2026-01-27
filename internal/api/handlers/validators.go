package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/ids"
)

// FilterError represents a validation error for a specific field.
type FilterError struct {
	Field   string
	Message string
}

func (e FilterError) Error() string {
	return e.Field + ": " + e.Message
}

// ValidateAndExtractULID extracts and validates a ULID from a request path parameter.
// Returns the validated ULID string and true if valid.
// If invalid, writes an appropriate error response and returns empty string and false.
func ValidateAndExtractULID(w http.ResponseWriter, r *http.Request, paramName, env string) (string, bool) {
	ulidValue := strings.TrimSpace(pathParam(r, paramName))
	if ulidValue == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", FilterError{Field: paramName, Message: "missing"}, env)
		return "", false
	}
	if err := ids.ValidateULID(ulidValue); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", FilterError{Field: paramName, Message: "invalid ULID"}, env)
		return "", false
	}
	return ulidValue, true
}

// ValidateULIDParam validates a ULID path parameter without automatic error writing.
// Returns the validated ULID and any validation error.
// Use this when you need custom error handling.
func ValidateULIDParam(r *http.Request, paramName string) (string, error) {
	ulidValue := strings.TrimSpace(pathParam(r, paramName))
	if ulidValue == "" {
		return "", FilterError{Field: paramName, Message: "missing"}
	}
	if err := ids.ValidateULID(ulidValue); err != nil {
		return "", FilterError{Field: paramName, Message: "invalid ULID"}
	}
	return ulidValue, nil
}

var (
	// ErrMissingID is returned when a required ID parameter is missing
	ErrMissingID = errors.New("missing id")
	// ErrInvalidULID is returned when a ULID parameter has invalid format
	ErrInvalidULID = errors.New("invalid ULID format")
)
