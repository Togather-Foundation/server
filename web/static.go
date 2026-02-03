package web

import (
	_ "embed"
	"net/http"
)

//go:embed robots.txt
var robotsTxt []byte

//go:embed sitemap.xml
var sitemapXML []byte

// RobotsTxtHandler serves the robots.txt file.
func RobotsTxtHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(robotsTxt) // Error is ignored as WriteHeader already sent status
	})
}

// SitemapHandler serves the sitemap.xml file.
func SitemapHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(sitemapXML) // Error is ignored as WriteHeader already sent status
	})
}
