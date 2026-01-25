package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/api/handlers"
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
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())

	eventsHandler := handlers.NewEventsHandler(eventsService, cfg.Environment)
	placesHandler := handlers.NewPlacesHandler(placesService, cfg.Environment)
	orgHandler := handlers.NewOrganizationsHandler(orgService, cfg.Environment)

	mux := http.NewServeMux()
	mux.Handle("/healthz", handlers.Healthz())
	mux.Handle("/readyz", handlers.Readyz())
	mux.Handle("/api/v1/openapi.json", OpenAPIHandler())
	mux.Handle("/api/v1/events", http.HandlerFunc(eventsHandler.List))
	mux.Handle("/api/v1/events/{id}", http.HandlerFunc(eventsHandler.Get))
	mux.Handle("/api/v1/places", http.HandlerFunc(placesHandler.List))
	mux.Handle("/api/v1/places/{id}", http.HandlerFunc(placesHandler.Get))
	mux.Handle("/api/v1/organizations", http.HandlerFunc(orgHandler.List))
	mux.Handle("/api/v1/organizations/{id}", http.HandlerFunc(orgHandler.Get))
	return mux
}
