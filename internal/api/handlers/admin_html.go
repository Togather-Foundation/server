package handlers

import (
	"net/http"
)

// AdminHTMLPlaceholder returns a basic handler for admin HTML routes.
// These routes require cookie-based auth and currently return Not Implemented.
func AdminHTMLPlaceholder(_ string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte("<html><body>Admin UI not implemented.</body></html>"))
	}
}
