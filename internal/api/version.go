package api

import (
	"encoding/json"
	"net/http"
	"runtime"
)

// versionResponse represents the JSON response for the /version endpoint
type versionResponse struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
}

// VersionHandler returns an HTTP handler for the /version endpoint.
// Returns build metadata including version, git commit, build date, and Go version.
// This is a public endpoint (no authentication required).
//
// Parameters are typically set via ldflags during build (see Dockerfile):
//   - version: Application version (e.g., "0.1.0" or "dev")
//   - gitCommit: Git commit hash (e.g., "abc123def456" or "unknown")
//   - buildDate: Build timestamp (e.g., "2026-01-28T12:00:00Z" or "unknown")
func VersionHandler(version, gitCommit, buildDate string) http.Handler {
	// Set defaults if values not provided
	if version == "" {
		version = "dev"
	}
	if gitCommit == "" {
		gitCommit = "unknown"
	}
	if buildDate == "" {
		buildDate = "unknown"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		response := versionResponse{
			Version:   version,
			GitCommit: gitCommit,
			BuildDate: buildDate,
			GoVersion: runtime.Version(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	})
}
