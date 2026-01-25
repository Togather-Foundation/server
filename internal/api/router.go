package api

import (
	"net/http"

	"github.com/Togather-Foundation/server/internal/api/handlers"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/rs/zerolog"
)

func NewRouter(cfg config.Config, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/healthz", handlers.Healthz())
	mux.Handle("/readyz", handlers.Readyz())
	mux.Handle("/api/v1/openapi.json", OpenAPIHandler())
	return mux
}
