package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

type contextKey string

const contentTypeKey contextKey = "negotiatedContentType"

const (
	contentJSONLD = "application/ld+json"
	contentJSON   = "application/json"
	contentHTML   = "text/html"
	contentTurtle = "text/turtle"
)

func ContentNegotiation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := negotiateContentType(r)
		ctx := context.WithValue(r.Context(), contentTypeKey, contentType)
		w.Header().Set("Vary", "Accept")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func NegotiatedContentType(r *http.Request) string {
	if r == nil {
		return contentJSON
	}
	if value, ok := r.Context().Value(contentTypeKey).(string); ok && value != "" {
		return value
	}
	return negotiateContentType(r)
}

func negotiateContentType(r *http.Request) string {
	if r == nil {
		return contentJSON
	}

	if format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))); format != "" {
		switch format {
		case "jsonld", "ld+json", "application/ld+json":
			return contentJSONLD
		case "json", "application/json":
			return contentJSONLD
		case "html", "text/html":
			return contentHTML
		case "turtle", "text/turtle":
			return contentTurtle
		}
	}

	accept := r.Header.Get("Accept")
	if strings.TrimSpace(accept) == "" {
		return contentJSON
	}

	bestType := ""
	bestQ := -1.0
	for _, part := range strings.Split(accept, ",") {
		mediaType := strings.TrimSpace(part)
		if mediaType == "" {
			continue
		}

		q := 1.0
		if strings.Contains(mediaType, ";") {
			segments := strings.Split(mediaType, ";")
			mediaType = strings.TrimSpace(segments[0])
			for _, seg := range segments[1:] {
				seg = strings.TrimSpace(seg)
				if strings.HasPrefix(seg, "q=") {
					if parsed, err := strconv.ParseFloat(strings.TrimPrefix(seg, "q="), 64); err == nil {
						q = parsed
					}
				}
			}
		}

		candidate := normalizeMediaType(mediaType)
		if candidate == "" {
			continue
		}
		if q > bestQ {
			bestQ = q
			bestType = candidate
		}
	}

	if bestType == "" {
		return contentJSON
	}
	return bestType
}

func normalizeMediaType(mediaType string) string {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "*/*" {
		return contentJSON
	}

	switch mediaType {
	case contentJSONLD, contentJSON:
		return contentJSONLD
	case contentHTML:
		return contentHTML
	case contentTurtle:
		return contentTurtle
	default:
		return ""
	}
}
