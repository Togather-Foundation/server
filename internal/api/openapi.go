package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"sigs.k8s.io/yaml"
)

const openAPISourcePath = "specs/001-sel-backend/contracts/openapi.yaml"

// maxParentTraversal is the maximum number of parent directories to traverse
// when searching for go.mod to determine the repository root. This prevents
// infinite loops in case the filesystem structure is unusual (e.g., symlinks,
// containers) while still allowing reasonable project nesting depth.
const maxParentTraversal = 10

var (
	openAPIJSON    []byte
	openAPIJSONErr error
	openAPIYAML    []byte
	openAPIYAMLErr error
	openAPIOnce    sync.Once
)

func OpenAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		openAPIOnce.Do(func() {
			data, err := os.ReadFile(openAPISourcePath)
			if err != nil {
				data, err = os.ReadFile(resolveOpenAPIPath())
				if err != nil {
					openAPIJSONErr = err
					openAPIYAMLErr = err
					return
				}
			}
			// Store raw YAML for YAML endpoint
			openAPIYAML = data
			// Convert to JSON for JSON endpoint
			openAPIJSON, openAPIJSONErr = yaml.YAMLToJSON(data)
		})

		if openAPIJSONErr != nil {
			http.Error(w, "openapi unavailable", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openAPIJSON)
	}
}

// OpenAPIYAMLHandler returns the OpenAPI specification in YAML format.
// Serves the raw YAML file without conversion.
func OpenAPIYAMLHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		openAPIOnce.Do(func() {
			data, err := os.ReadFile(openAPISourcePath)
			if err != nil {
				data, err = os.ReadFile(resolveOpenAPIPath())
				if err != nil {
					openAPIJSONErr = err
					openAPIYAMLErr = err
					return
				}
			}
			// Store raw YAML for YAML endpoint
			openAPIYAML = data
			// Convert to JSON for JSON endpoint
			openAPIJSON, openAPIJSONErr = yaml.YAMLToJSON(data)
		})

		if openAPIYAMLErr != nil {
			http.Error(w, "openapi unavailable", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openAPIYAML)
	}
}

func resolveOpenAPIPath() string {
	root, err := repoRoot()
	if err != nil {
		return openAPISourcePath
	}
	return filepath.Join(root, openAPISourcePath)
}

func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < maxParentTraversal; i++ {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	base := filepath.Dir(file)
	return filepath.Join(base, "..", "..", ".."), nil
}
