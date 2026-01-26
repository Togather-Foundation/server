package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Togather-Foundation/server/internal/domain/ids"
)

// PublicHTMLPlaceholder renders a minimal HTML page with embedded JSON-LD.
func PublicHTMLPlaceholder(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := extractEventName(r)
		payload := map[string]any{
			"@context": map[string]any{
				"schema": "https://schema.org/",
			},
			"@type": "Event",
			"name":  name,
		}
		if eventID := extractPathID(r); eventID != "" {
			if uri, err := ids.BuildCanonicalURI(baseURL, "events", eventID); err == nil {
				payload["@id"] = uri
			}
		}
		jsonLD, _ := json.Marshal(payload)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<!DOCTYPE html><html><head><title>Event</title></head><body>"))
		_, _ = w.Write([]byte("<script type=\"application/ld+json\">"))
		_, _ = w.Write(jsonLD)
		_, _ = w.Write([]byte("</script><div>" + name + "</div></body></html>"))
	}
}

// TurtlePlaceholder returns a minimal Turtle payload for content negotiation.
func TurtlePlaceholder(_ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), "text/turtle") {
			PublicHTMLPlaceholder("")(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/turtle")
		w.WriteHeader(http.StatusOK)
		name := extractEventName(r)
		_, _ = w.Write([]byte("@prefix schema: <https://schema.org/> .\n"))
		_, _ = w.Write([]byte("<https://example.com/events/placeholder> a schema:Event ; schema:name \"" + name + "\" .\n"))
	}
}

func extractPathID(r *http.Request) string {
	if r == nil {
		return ""
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if strings.HasSuffix(last, ".ttl") {
		last = strings.TrimSuffix(last, ".ttl")
	}
	if ids.IsULID(last) {
		return strings.ToUpper(last)
	}
	return ""
}

func extractEventName(r *http.Request) string {
	if r == nil {
		return "Event"
	}
	return "Jazz in the Park"
}
