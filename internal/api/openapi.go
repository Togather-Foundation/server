package api

import (
	_ "embed"
	"net/http"
	"sync"

	"sigs.k8s.io/yaml"
)

//go:embed ../../specs/001-sel-backend/contracts/openapi.yaml
var openAPISource []byte

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
			openAPIJSON, openAPIJSONErr = yaml.YAMLToJSON(openAPISource)
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
