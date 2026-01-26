package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/handlers"
	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/federation"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/domain/provenance"
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
	provenanceService := provenance.NewService(repo.Provenance())

	eventsHandler := handlers.NewEventsHandler(eventsService, ingestService, provenanceService, cfg.Environment, cfg.Server.BaseURL)
	placesHandler := handlers.NewPlacesHandler(placesService, cfg.Environment, cfg.Server.BaseURL)
	orgHandler := handlers.NewOrganizationsHandler(orgService, cfg.Environment, cfg.Server.BaseURL)

	// Create audit logger for admin operations
	auditLogger := audit.NewLogger()

	// Admin handlers
	queries := postgres.New(pool)
	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.JWTExpiry, "sel.events")
	adminAuthHandler := handlers.NewAdminAuthHandler(queries, jwtManager, auditLogger, cfg.Environment, nil)

	// Create AdminService
	requireHTTPS := cfg.Environment == "production"
	adminService := events.NewAdminService(repo.Events(), requireHTTPS)
	adminHandler := handlers.NewAdminHandler(eventsService, adminService, placesService, orgService, auditLogger, cfg.Environment, cfg.Server.BaseURL)

	// Create API Key handler
	apiKeyHandler := handlers.NewAPIKeyHandler(queries, cfg.Environment)

	// Create Federation handler
	federationService := federation.NewService(repo.Federation(), requireHTTPS)
	federationHandler := handlers.NewFederationHandler(federationService, cfg.Environment)

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
	publicPages := handlers.NewPublicPagesHandler(eventsService, placesService, orgService, cfg.Environment, cfg.Server.BaseURL)
	publicEventPage := rateLimitPublic(http.HandlerFunc(publicPages.GetEvent))
	publicPlacePage := rateLimitPublic(http.HandlerFunc(publicPages.GetPlace))
	publicOrgPage := rateLimitPublic(http.HandlerFunc(publicPages.GetOrganization))

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

	// Public HTML and Turtle placeholders for content negotiation tests
	mux.Handle("/events/{id}", publicEventPage)
	mux.Handle("/places/{id}", publicPlacePage)
	mux.Handle("/organizations/{id}", publicOrgPage)

	// Admin routes (T075, T076)
	// Admin auth endpoints - login has aggressive rate limiting
	rateLimitLogin := middleware.WithRateLimitTierHandler(middleware.TierLogin)
	mux.Handle("/api/v1/admin/login", rateLimitLogin(http.HandlerFunc(adminAuthHandler.Login)))
	mux.Handle("/api/v1/admin/logout", http.HandlerFunc(adminAuthHandler.Logout))

	// Admin event management endpoints (requires JWT auth)
	jwtAuth := middleware.JWTAuth(jwtManager, cfg.Environment)
	adminRateLimit := middleware.WithRateLimitTierHandler(middleware.TierAdmin)

	adminPendingEvents := jwtAuth(adminRateLimit(http.HandlerFunc(adminHandler.ListPendingEvents)))
	mux.Handle("/api/v1/admin/events/pending", adminPendingEvents)

	adminListEvents := jwtAuth(adminRateLimit(http.HandlerFunc(adminHandler.ListEvents)))
	mux.Handle("/api/v1/admin/events", methodMux(map[string]http.Handler{
		http.MethodGet: adminListEvents,
	}))

	adminUpdateEvent := jwtAuth(adminRateLimit(http.HandlerFunc(adminHandler.UpdateEvent)))
	adminDeleteEvent := jwtAuth(adminRateLimit(http.HandlerFunc(adminHandler.DeleteEvent)))
	mux.Handle("/api/v1/admin/events/{id}", methodMux(map[string]http.Handler{
		http.MethodPut:    adminUpdateEvent,
		http.MethodDelete: adminDeleteEvent,
	}))

	adminPublishEvent := jwtAuth(adminRateLimit(http.HandlerFunc(adminHandler.PublishEvent)))
	mux.Handle("/api/v1/admin/events/{id}/publish", methodMux(map[string]http.Handler{
		http.MethodPost: adminPublishEvent,
	}))

	adminUnpublishEvent := jwtAuth(adminRateLimit(http.HandlerFunc(adminHandler.UnpublishEvent)))
	mux.Handle("/api/v1/admin/events/{id}/unpublish", methodMux(map[string]http.Handler{
		http.MethodPost: adminUnpublishEvent,
	}))

	adminMergeEvents := jwtAuth(adminRateLimit(http.HandlerFunc(adminHandler.MergeEvents)))
	mux.Handle("/api/v1/admin/events/merge", methodMux(map[string]http.Handler{
		http.MethodPost: adminMergeEvents,
	}))

	adminDeletePlace := jwtAuth(adminRateLimit(http.HandlerFunc(adminHandler.DeletePlace)))
	mux.Handle("/api/v1/admin/places/{id}", methodMux(map[string]http.Handler{
		http.MethodDelete: adminDeletePlace,
	}))

	adminDeleteOrganization := jwtAuth(adminRateLimit(http.HandlerFunc(adminHandler.DeleteOrganization)))
	mux.Handle("/api/v1/admin/organizations/{id}", methodMux(map[string]http.Handler{
		http.MethodDelete: adminDeleteOrganization,
	}))

	// Admin API key management (T078)
	adminCreateAPIKey := jwtAuth(adminRateLimit(http.HandlerFunc(apiKeyHandler.CreateAPIKey)))
	adminListAPIKeys := jwtAuth(adminRateLimit(http.HandlerFunc(apiKeyHandler.ListAPIKeys)))
	adminRevokeAPIKey := jwtAuth(adminRateLimit(http.HandlerFunc(apiKeyHandler.RevokeAPIKey)))

	mux.Handle("/api/v1/admin/api-keys", methodMux(map[string]http.Handler{
		http.MethodPost: adminCreateAPIKey,
		http.MethodGet:  adminListAPIKeys,
	}))
	mux.Handle("/api/v1/admin/api-keys/{id}", methodMux(map[string]http.Handler{
		http.MethodDelete: adminRevokeAPIKey,
	}))

	// Admin federation node management (T081b)
	adminCreateNode := jwtAuth(adminRateLimit(http.HandlerFunc(federationHandler.CreateNode)))
	adminListNodes := jwtAuth(adminRateLimit(http.HandlerFunc(federationHandler.ListNodes)))
	adminGetNode := jwtAuth(adminRateLimit(http.HandlerFunc(federationHandler.GetNode)))
	adminUpdateNode := jwtAuth(adminRateLimit(http.HandlerFunc(federationHandler.UpdateNode)))
	adminDeleteNode := jwtAuth(adminRateLimit(http.HandlerFunc(federationHandler.DeleteNode)))

	mux.Handle("/api/v1/admin/federation/nodes", methodMux(map[string]http.Handler{
		http.MethodPost: adminCreateNode,
		http.MethodGet:  adminListNodes,
	}))
	mux.Handle("/api/v1/admin/federation/nodes/{id}", methodMux(map[string]http.Handler{
		http.MethodGet:    adminGetNode,
		http.MethodPut:    adminUpdateNode,
		http.MethodDelete: adminDeleteNode,
	}))

	// Admin HTML pages (T080) - placeholder routes for future implementation
	adminCookieAuth := middleware.AdminAuthCookie(jwtManager)
	adminHTML := adminCookieAuth(http.HandlerFunc(handlers.AdminHTMLPlaceholder(cfg.Environment)))
	mux.Handle("/admin", adminHTML)
	mux.Handle("/admin/", adminHTML)
	mux.Handle("/admin/login", http.HandlerFunc(adminAuthHandler.LoginPage))

	// Wrap entire router with middleware stack
	// Order: CorrelationID -> RequestLogging -> RateLimit
	handler := middleware.CorrelationID(logger)(mux)
	handler = middleware.RequestLogging(logger)(handler)
	handler = middleware.RateLimit(cfg.RateLimit)(handler)

	return handler
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
