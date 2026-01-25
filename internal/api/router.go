package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/handlers"
	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

func NewRouter(cfg config.Config, logger zerolog.Logger) http.Handler {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		logger.Error().Err(err).Msg("database connection failed")
		return http.NewServeMux()
	}

	repo, err := postgres.NewRepository(pool)
	if err != nil {
		logger.Error().Err(err).Msg("repository init failed")
		return http.NewServeMux()
	}

	eventsService := events.NewService(repo.Events())
	ingestService := events.NewIngestService(repo.Events(), cfg.Server.BaseURL)
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())

	eventsHandler := handlers.NewEventsHandler(eventsService, ingestService, cfg.Environment, cfg.Server.BaseURL)
	placesHandler := handlers.NewPlacesHandler(placesService, cfg.Environment)
	orgHandler := handlers.NewOrganizationsHandler(orgService, cfg.Environment)

	mux := http.NewServeMux()
	mux.Handle("/healthz", handlers.Healthz())
	mux.Handle("/readyz", handlers.Readyz())
	mux.Handle("/api/v1/openapi.json", OpenAPIHandler())
	apiKeyRepo := repo.Auth().APIKeys()
	apiKeyAuth := middleware.AgentAuth(apiKeyRepo)
	rateLimitAgent := middleware.WithRateLimitTierHandler(middleware.TierAgent)

	publicEvents := http.HandlerFunc(eventsHandler.List)
	createEvents := apiKeyAuth(rateLimitAgent(http.HandlerFunc(eventsHandler.Create)))

	mux.Handle("/api/v1/events", methodMux(map[string]http.Handler{
		http.MethodGet:  publicEvents,
		http.MethodPost: createEvents,
	}))
	mux.Handle("/api/v1/events/{id}", http.HandlerFunc(eventsHandler.Get))
	mux.Handle("/api/v1/places", http.HandlerFunc(placesHandler.List))
	mux.Handle("/api/v1/places/{id}", http.HandlerFunc(placesHandler.Get))
	mux.Handle("/api/v1/organizations", http.HandlerFunc(orgHandler.List))
	mux.Handle("/api/v1/organizations/{id}", http.HandlerFunc(orgHandler.Get))
	return mux
}

func methodMux(handlers map[string]http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler, ok := handlers[r.Method]; ok {
			handler.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Allow", allowedMethods(handlers))
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
}

func allowedMethods(handlers map[string]http.Handler) string {
	methods := make([]string, 0, len(handlers))
	for method := range handlers {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return strings.Join(methods, ", ")
}
