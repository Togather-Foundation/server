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

var (
	openAPIJSON    []byte
	openAPIJSONErr error
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
					return
				}
			}
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
	for i := 0; i < 10; i++ {
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
