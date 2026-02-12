package handlers

import (
	"html/template"
	"log/slog"
	"net/http"
)

// DevHTMLHandler handles developer portal HTML page rendering
type DevHTMLHandler struct {
	Templates *template.Template
	Env       string
	Logger    *slog.Logger
}

// NewDevHTMLHandler creates a new developer HTML handler
func NewDevHTMLHandler(templates *template.Template, env string, logger *slog.Logger) *DevHTMLHandler {
	return &DevHTMLHandler{
		Templates: templates,
		Env:       env,
		Logger:    logger,
	}
}

// ServeLogin renders the developer login page
func (h *DevHTMLHandler) ServeLogin(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("developer HTML request",
			slog.String("page", "login"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title": "Developer Login - SEL Events",
	}

	if err := h.Templates.ExecuteTemplate(w, "login.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "login.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeAcceptInvitation renders the developer invitation acceptance page
func (h *DevHTMLHandler) ServeAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("developer HTML request",
			slog.String("page", "accept_invitation"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title": "Accept Invitation - SEL Developer Portal",
	}

	if err := h.Templates.ExecuteTemplate(w, "accept_invitation.html", data); err != nil {
		if h.Logger != nil {
			h.Logger.Error("template error", slog.String("template", "accept_invitation.html"), slog.Any("error", err))
		}
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

// ServeDashboard renders the developer dashboard page
func (h *DevHTMLHandler) ServeDashboard(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("developer HTML request",
			slog.String("page", "dashboard"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "Dashboard - SEL Developer Portal",
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

// ServeAPIKeys renders the developer API keys management page
func (h *DevHTMLHandler) ServeAPIKeys(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Info("developer HTML request",
			slog.String("page", "api_keys"),
			slog.String("method", r.Method),
			slog.String("remote_addr", r.RemoteAddr))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := map[string]interface{}{
		"Title":      "API Keys - SEL Developer Portal",
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
