package validation

import (
	"fmt"
	"net/url"
	"strings"
)

// URLValidationError represents a URL validation failure
type URLValidationError struct {
	Field   string
	Message string
	URL     string
}

func (e URLValidationError) Error() string {
	return fmt.Sprintf("%s: %s (url: %s)", e.Field, e.Message, e.URL)
}

// ValidateURL validates that a URL is well-formed and optionally requires HTTPS
func ValidateURL(urlString, fieldName string, requireHTTPS bool) error {
	if urlString == "" {
		return nil // Empty URLs are allowed unless field is required
	}

	// Parse the URL
	parsedURL, err := url.Parse(urlString)
	if err != nil {
		return URLValidationError{
			Field:   fieldName,
			Message: "invalid URL format",
			URL:     urlString,
		}
	}

	// Check if URL has a scheme
	if parsedURL.Scheme == "" {
		return URLValidationError{
			Field:   fieldName,
			Message: "URL must include a scheme (http:// or https://)",
			URL:     urlString,
		}
	}

	// Check if URL has a host
	if parsedURL.Host == "" {
		return URLValidationError{
			Field:   fieldName,
			Message: "URL must include a host",
			URL:     urlString,
		}
	}

	// Require HTTPS in production if specified
	if requireHTTPS {
		scheme := strings.ToLower(parsedURL.Scheme)
		if scheme != "https" {
			return URLValidationError{
				Field:   fieldName,
				Message: "URL must use HTTPS in production",
				URL:     urlString,
			}
		}
	}

	// Ensure scheme is http or https
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return URLValidationError{
			Field:   fieldName,
			Message: "URL scheme must be http or https",
			URL:     urlString,
		}
	}

	return nil
}

// ValidateBaseURL validates a base URL for federation nodes
// Base URLs must not have paths, query parameters, or fragments
func ValidateBaseURL(urlString, fieldName string, requireHTTPS bool) error {
	if err := ValidateURL(urlString, fieldName, requireHTTPS); err != nil {
		return err
	}

	if urlString == "" {
		return nil
	}

	parsedURL, _ := url.Parse(urlString) // Already validated above

	// Base URLs should not have paths (except /)
	if parsedURL.Path != "" && parsedURL.Path != "/" {
		return URLValidationError{
			Field:   fieldName,
			Message: "base URL must not contain a path",
			URL:     urlString,
		}
	}

	// Base URLs should not have query parameters
	if parsedURL.RawQuery != "" {
		return URLValidationError{
			Field:   fieldName,
			Message: "base URL must not contain query parameters",
			URL:     urlString,
		}
	}

	// Base URLs should not have fragments
	if parsedURL.Fragment != "" {
		return URLValidationError{
			Field:   fieldName,
			Message: "base URL must not contain a fragment",
			URL:     urlString,
		}
	}

	return nil
}
