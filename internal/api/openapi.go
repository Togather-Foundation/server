package api

import (
	"net/http"
	"os"
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
				openAPIJSONErr = err
				return
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
