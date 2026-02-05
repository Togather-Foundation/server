package handlers

import (
	"html/template"
	"net/http"
)

// AdminHTMLHandler handles admin HTML page rendering
type AdminHTMLHandler struct {
	Templates *template.Template
	Env       string
}

// NewAdminHTMLHandler creates a new admin HTML handler
func NewAdminHTMLHandler(templates *template.Template, env string) *AdminHTMLHandler {
	return &AdminHTMLHandler{
		Templates: templates,
		Env:       env,
	}
}

// ServeDashboard renders the admin dashboard page
func (h *AdminHTMLHandler) ServeDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Dashboard - SEL Admin",
		"ActivePage": "dashboard",
	}

	if err := h.Templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeEventsList renders the events list page
func (h *AdminHTMLHandler) ServeEventsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Events - SEL Admin",
		"ActivePage": "events",
	}

	if err := h.Templates.ExecuteTemplate(w, "events_list.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeEventEdit renders the event edit page
func (h *AdminHTMLHandler) ServeEventEdit(w http.ResponseWriter, r *http.Request) {
	// Extract event ID from path parameter
	eventID := r.PathValue("id")
	if eventID == "" {
		http.Error(w, "Event ID required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":   "Edit Event - SEL Admin",
		"EventID": eventID,
	}

	if err := h.Templates.ExecuteTemplate(w, "event_edit.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeDuplicates renders the duplicates review page
func (h *AdminHTMLHandler) ServeDuplicates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Review Duplicates - SEL Admin",
		"ActivePage": "duplicates",
	}

	if err := h.Templates.ExecuteTemplate(w, "duplicates.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeAPIKeys renders the API keys management page
func (h *AdminHTMLHandler) ServeAPIKeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "API Keys - SEL Admin",
		"ActivePage": "api-keys",
	}

	if err := h.Templates.ExecuteTemplate(w, "api_keys.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeFederation renders the federation nodes management page
func (h *AdminHTMLHandler) ServeFederation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Template doesn't exist yet, return basic HTML
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Federation - SEL Admin</title>
</head>
<body>
    <h1>Federation Node Management</h1>
    <p>Coming soon...</p>
</body>
</html>`))
}

// AdminHTMLPlaceholder returns a basic handler for admin HTML routes.
// Deprecated: Use AdminHTMLHandler methods instead
func AdminHTMLPlaceholder(_ string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte("<html><body>Admin UI not implemented.</body></html>"))
	}
}
