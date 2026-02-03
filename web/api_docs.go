package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed api-docs/dist/*
var ApiDocsFS embed.FS

// APIDocsHandler serves the embedded Scalar API documentation UI.
// The UI loads the OpenAPI spec from /api/v1/openapi.json.
//
// Files are embedded at build time from web/api-docs/dist/ and served
// with appropriate cache headers:
//   - JS files: long-term caching (1 year, immutable)
//   - HTML files: no caching (allows spec updates to be reflected)
//
// Security: The HTML page uses a relaxed CSP to support Scalar features:
//   - 'unsafe-inline' + 'unsafe-eval': Vue.js runtime requires eval
//   - fonts.scalar.com: Inter and JetBrains Mono fonts
//   - proxy.scalar.com: AI-powered search feature
//   - api.scalar.com: Registry and search API
//
// This is safe because the HTML is controlled content from our embedded filesystem
func APIDocsHandler() http.Handler {
	// Strip the "api-docs/dist" prefix to serve files from the root
	stripped, err := fs.Sub(ApiDocsFS, "api-docs/dist")
	if err != nil {
		// This should never happen since the path is hardcoded
		panic("failed to create sub-filesystem for API docs: " + err.Error())
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET and HEAD requests
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Strip the /api/docs prefix from the URL path
		path := strings.TrimPrefix(r.URL.Path, "/api/docs")

		// If path is empty or just "/", serve index.html
		if path == "" || path == "/" {
			path = "index.html"
		} else {
			// Remove leading slash from path for file server
			path = strings.TrimPrefix(path, "/")
		}

		// Set appropriate cache headers for static assets
		// JS files can be cached longer since they're versioned
		if strings.HasSuffix(path, ".js") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if strings.HasSuffix(path, ".html") {
			// HTML should not be cached to allow spec updates
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
			// Override CSP for API docs HTML to allow Scalar requirements:
			// - 'unsafe-inline' + 'unsafe-eval': Scalar uses Vue.js which needs eval
			// - fonts.scalar.com: External font loading (Inter, JetBrains Mono)
			// - proxy.scalar.com: AI search feature
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; "+
					"style-src 'self' 'unsafe-inline'; "+
					"script-src 'self' 'unsafe-inline' 'unsafe-eval'; "+
					"font-src 'self' https://fonts.scalar.com; "+
					"connect-src 'self' https://proxy.scalar.com https://api.scalar.com")
		}

		// Open the file from embedded filesystem
		file, err := stripped.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()

		// Get file info for serving
		stat, err := file.Stat()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Serve the file content
		http.ServeContent(w, r, path, stat.ModTime(), file.(io.ReadSeeker))
	})
}
