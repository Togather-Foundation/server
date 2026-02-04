package handlers

import (
	"html/template"
	"net/http"
	"strings"
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
	if h == nil || h.Templates == nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	data := map[string]interface{}{
		"Title": "Dashboard - SEL Admin",
	}

	if err := h.Templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeEventsList renders the events list page
func (h *AdminHTMLHandler) ServeEventsList(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Templates == nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	data := map[string]interface{}{
		"Title": "Events - SEL Admin",
	}

	if err := h.Templates.ExecuteTemplate(w, "events_list.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeEventEdit renders the event edit page
func (h *AdminHTMLHandler) ServeEventEdit(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Templates == nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Extract event ID from path: /admin/events/{id}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/admin/events/"), "/")
	eventID := pathParts[0]

	if eventID == "" {
		http.Error(w, "Event ID required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	data := map[string]interface{}{
		"Title":   "Edit Event - SEL Admin",
		"EventID": eventID,
	}

	if err := h.Templates.ExecuteTemplate(w, "event_edit.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeDuplicates renders the duplicates management page
func (h *AdminHTMLHandler) ServeDuplicates(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Templates == nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	data := map[string]interface{}{
		"Title": "Duplicates - SEL Admin",
	}

	if err := h.Templates.ExecuteTemplate(w, "duplicates.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeAPIKeys renders the API keys management page
func (h *AdminHTMLHandler) ServeAPIKeys(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Templates == nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	data := map[string]interface{}{
		"Title": "API Keys - SEL Admin",
	}

	if err := h.Templates.ExecuteTemplate(w, "api_keys.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeFederation renders the federation management page
// Note: federation.html template doesn't exist yet, so this returns a basic placeholder
func (h *AdminHTMLHandler) ServeFederation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Federation - SEL Admin</title>
    <link rel="stylesheet" href="/admin/static/css/tabler.min.css">
</head>
<body>
    <div class="page">
        <div class="page-wrapper">
            <div class="container-xl">
                <div class="empty">
                    <p class="empty-title">Federation Management</p>
                    <p class="empty-subtitle text-muted">Coming soon</p>
                </div>
            </div>
        </div>
    </div>
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
