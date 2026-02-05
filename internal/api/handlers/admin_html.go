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

	data := map[string]interface{}{
		"Title":      "Federation Nodes - SEL Admin",
		"ActivePage": "federation",
	}

	if err := h.Templates.ExecuteTemplate(w, "federation.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeUsersList renders the users list page
func (h *AdminHTMLHandler) ServeUsersList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Users - SEL Admin",
		"ActivePage": "users",
	}

	if err := h.Templates.ExecuteTemplate(w, "users_list.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeUserActivity renders the user activity page
func (h *AdminHTMLHandler) ServeUserActivity(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from path parameter
	userID := r.PathValue("id")
	if userID == "" {
		http.Error(w, "User ID required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "User Activity - SEL Admin",
		"ActivePage": "users",
		"UserID":     userID,
	}

	if err := h.Templates.ExecuteTemplate(w, "user_activity.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeAcceptInvitation renders the public invitation acceptance page
func (h *AdminHTMLHandler) ServeAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title": "Accept Invitation - SEL Admin",
	}

	if err := h.Templates.ExecuteTemplate(w, "accept_invitation.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
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
