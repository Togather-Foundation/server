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
	// Note: Connection pool is intentionally not closed here as it's used by the router
	// throughout the application lifecycle. It will be cleaned up on process exit.

	repo, err := postgres.NewRepository(pool)
	if err != nil {
		logger.Error().Err(err).Msg("repository init failed")
		pool.Close() // Clean up pool if repository init fails
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

	// Middleware setup
	apiKeyRepo := repo.Auth().APIKeys()
	apiKeyAuth := middleware.AgentAuth(apiKeyRepo)
	rateLimitPublic := middleware.WithRateLimitTierHandler(middleware.TierPublic)
	rateLimitAgent := middleware.WithRateLimitTierHandler(middleware.TierAgent)

	// Apply rate limiting to public read endpoints
	publicEvents := rateLimitPublic(http.HandlerFunc(eventsHandler.List))
	publicEventGet := rateLimitPublic(http.HandlerFunc(eventsHandler.Get))
	publicPlaces := rateLimitPublic(http.HandlerFunc(placesHandler.List))
	publicPlaceGet := rateLimitPublic(http.HandlerFunc(placesHandler.Get))
	publicOrgs := rateLimitPublic(http.HandlerFunc(orgHandler.List))
	publicOrgGet := rateLimitPublic(http.HandlerFunc(orgHandler.Get))

	// Authenticated write endpoints with agent rate limiting
	createEvents := apiKeyAuth(rateLimitAgent(http.HandlerFunc(eventsHandler.Create)))

	mux.Handle("/api/v1/events", methodMux(map[string]http.Handler{
		http.MethodGet:  publicEvents,
		http.MethodPost: createEvents,
	}))
	mux.Handle("/api/v1/events/{id}", publicEventGet)
	mux.Handle("/api/v1/places", publicPlaces)
	mux.Handle("/api/v1/places/{id}", publicPlaceGet)
	mux.Handle("/api/v1/organizations", publicOrgs)
	mux.Handle("/api/v1/organizations/{id}", publicOrgGet)

	// Wrap entire router with rate limiting middleware
	return middleware.RateLimit(cfg.RateLimit)(mux)
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
