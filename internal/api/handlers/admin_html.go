package handlers

import (
	"html/template"
	"log/slog"
	"net/http"
)

// AdminHTMLHandler handles admin HTML page rendering
type AdminHTMLHandler struct {
	Templates *template.Template
	Env       string
	Logger    *slog.Logger
}

// NewAdminHTMLHandler creates a new admin HTML handler
func NewAdminHTMLHandler(templates *template.Template, env string, logger *slog.Logger) *AdminHTMLHandler {
	return &AdminHTMLHandler{
		Templates: templates,
		Env:       env,
		Logger:    logger,
	}
}

// ServeDashboard renders the admin dashboard page
func (h *AdminHTMLHandler) ServeDashboard(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "dashboard"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Dashboard - SEL Admin",
		"ActivePage": "dashboard",
	}

	if err := h.Templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "dashboard.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeEventsList renders the events list page
func (h *AdminHTMLHandler) ServeEventsList(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "events_list"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Events - SEL Admin",
		"ActivePage": "events",
	}

	if err := h.Templates.ExecuteTemplate(w, "events_list.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "events_list.html"), slog.Any("error", err))
		}
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

	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "event_edit"),
			slog.String("method", r.Method),
			slog.String("event_id", eventID),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":   "Edit Event - SEL Admin",
		"EventID": eventID,
	}

	if err := h.Templates.ExecuteTemplate(w, "event_edit.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "event_edit.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeDuplicates renders the duplicates review page
func (h *AdminHTMLHandler) ServeDuplicates(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "duplicates"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Review Duplicates - SEL Admin",
		"ActivePage": "duplicates",
	}

	if err := h.Templates.ExecuteTemplate(w, "duplicates.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "duplicates.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeAPIKeys renders the API keys management page
func (h *AdminHTMLHandler) ServeAPIKeys(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "api_keys"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "API Keys - SEL Admin",
		"ActivePage": "api-keys",
	}

	if err := h.Templates.ExecuteTemplate(w, "api_keys.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "api_keys.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeFederation renders the federation nodes management page
func (h *AdminHTMLHandler) ServeFederation(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "federation"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Federation Nodes - SEL Admin",
		"ActivePage": "federation",
	}

	if err := h.Templates.ExecuteTemplate(w, "federation.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "federation.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeUsersList renders the users list page
func (h *AdminHTMLHandler) ServeUsersList(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "users_list"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Users - SEL Admin",
		"ActivePage": "users",
	}

	if err := h.Templates.ExecuteTemplate(w, "users_list.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "users_list.html"), slog.Any("error", err))
		}
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

	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "user_activity"),
			slog.String("method", r.Method),
			slog.String("user_id", userID),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "User Activity - SEL Admin",
		"ActivePage": "users",
		"UserID":     userID,
	}

	if err := h.Templates.ExecuteTemplate(w, "user_activity.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "user_activity.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeReviewQueue renders the review queue page
func (h *AdminHTMLHandler) ServeReviewQueue(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "review-queue"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Review Queue - SEL Admin",
		"ActivePage": "review-queue",
	}

	if err := h.Templates.ExecuteTemplate(w, "review_queue.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "review_queue.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeDevelopersList renders the developers list page
func (h *AdminHTMLHandler) ServeDevelopersList(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "developers_list"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Developers - SEL Admin",
		"ActivePage": "developers",
	}

	if err := h.Templates.ExecuteTemplate(w, "developers_list.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "developers_list.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeAcceptInvitation renders the public invitation acceptance page
func (h *AdminHTMLHandler) ServeAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "accept_invitation"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title": "Accept Invitation - SEL Admin",
	}

	if err := h.Templates.ExecuteTemplate(w, "accept_invitation.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "accept_invitation.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServePlacesList renders the places list page
func (h *AdminHTMLHandler) ServePlacesList(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "places_list"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Places - SEL Admin",
		"ActivePage": "places",
	}

	if err := h.Templates.ExecuteTemplate(w, "places_list.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "places_list.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeOrganizationsList renders the organizations list page
func (h *AdminHTMLHandler) ServeOrganizationsList(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("admin HTML request",
			slog.String("page", "organizations_list"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Organizations - SEL Admin",
		"ActivePage": "organizations",
	}

	if err := h.Templates.ExecuteTemplate(w, "organizations_list.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "organizations_list.html"), slog.Any("error", err))
		}
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
