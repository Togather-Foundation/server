package api

import (
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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
	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/Togather-Foundation/server/internal/jsonld"
	"github.com/Togather-Foundation/server/internal/metrics"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/Togather-Foundation/server/web"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
)

const (
	// maxRouterRetries is the maximum number of retries for router operations
	maxRouterRetries = 10
)

// RouterWithClient bundles the HTTP handler with the River client for graceful shutdown.
type RouterWithClient struct {
	Handler     http.Handler
	RiverClient *river.Client[pgx.Tx]
}

func NewRouter(cfg config.Config, logger zerolog.Logger, pool *pgxpool.Pool, version, gitCommit, buildDate string) *RouterWithClient {
	repo, err := postgres.NewRepository(pool)
	if err != nil {
		logger.Error().Err(err).Msg("repository init failed")
		return &RouterWithClient{
			Handler:     http.NewServeMux(),
			RiverClient: nil,
		}
	}

	eventsService := events.NewService(repo.Events())
	ingestService := events.NewIngestService(repo.Events(), cfg.Server.BaseURL)
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())
	provenanceService := provenance.NewService(repo.Provenance())

	// Create SQLc queries instance for direct database access
	queries := postgres.New(pool)

	// Get deployment slot for metrics labeling (blue/green)
	slot := os.Getenv("SLOT")
	if slot == "" {
		slot = "unknown"
	}

	// Initialize River job queue for batch processing
	slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	workers := jobs.NewWorkersWithPool(pool, ingestService, slogLogger, slot)

	// Create River metrics hook for Prometheus monitoring
	riverHooks := []rivertype.Hook{
		metrics.NewRiverMetricsHook(slot),
	}

	riverClient, err := jobs.NewClient(pool, workers, slogLogger, riverHooks)
	if err != nil {
		logger.Error().Err(err).Msg("river client init failed")
		return &RouterWithClient{
			Handler:     http.NewServeMux(),
			RiverClient: nil,
		}
	}
	// Note: River workers are started with a shutdown-aware context in serve.go
	// DO NOT call riverClient.Start() here - it's handled during server initialization

	eventsHandler := handlers.NewEventsHandler(eventsService, ingestService, provenanceService, riverClient, queries, cfg.Environment, cfg.Server.BaseURL)
	placesHandler := handlers.NewPlacesHandler(placesService, cfg.Environment, cfg.Server.BaseURL)
	orgHandler := handlers.NewOrganizationsHandler(orgService, cfg.Environment, cfg.Server.BaseURL)

	// Create audit logger for admin operations
	auditLogger := audit.NewLoggerWithZerolog(logger)

	// Load admin templates
	templates, err := loadAdminTemplates()
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load admin templates, admin UI will be unavailable")
	}

	// Admin handlers
	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.JWTExpiry, "sel.events")
	adminAuthHandler := handlers.NewAdminAuthHandler(queries, jwtManager, auditLogger, cfg.Environment, templates, cfg.Auth.JWTExpiry)

	// Create AdminService
	requireHTTPS := cfg.Environment == "production"
	adminService := events.NewAdminService(repo.Events(), requireHTTPS)
	adminHandler := handlers.NewAdminHandler(eventsService, adminService, placesService, orgService, auditLogger, cfg.Environment, cfg.Server.BaseURL)

	// Create API Key handler
	apiKeyHandler := handlers.NewAPIKeyHandler(queries, cfg.Environment)

	// Create Federation handlers (T111)
	changeFeedRepo := postgres.NewChangeFeedRepository(queries)
	changeFeedService := federation.NewChangeFeedService(changeFeedRepo, logger, cfg.Server.BaseURL)
	feedsHandler := handlers.NewFeedsHandler(changeFeedService, cfg.Environment, cfg.Server.BaseURL)

	// Initialize SHACL validator (DISABLED by default - use in dev/CI only)
	// SHACL validation spawns Python processes (~150-200ms overhead per event)
	shaclEnabled := os.Getenv("SHACL_VALIDATION_ENABLED") == "true" // Default: DISABLED
	shapesDir := findShapesDirectory()
	validator, err := jsonld.NewValidator(shapesDir, shaclEnabled)
	if err != nil {
		logger.Warn().Err(err).Msg("SHACL validator initialization failed, validation disabled")
		validator = nil
	} else if shaclEnabled {
		logger.Warn().Str("shapes_dir", shapesDir).Msg("SHACL validation enabled (spawns Python processes - not recommended for production)")
	}

	syncRepo := postgres.NewSyncRepository(pool, queries)
	syncService := federation.NewSyncService(syncRepo, validator, logger)
	federationService := federation.NewService(repo.Federation(), requireHTTPS)
	federationHandler := handlers.NewFederationHandler(federationService, syncService, cfg.Environment)

	// Well-known endpoints (Interoperability Profile §1.7)
	wellKnownHandler := handlers.NewWellKnownHandler(cfg.Server.BaseURL, "0.1.0", time.Now())

	// Health check endpoints (T011)
	healthChecker := handlers.NewHealthChecker(pool, riverClient, version, gitCommit)

	mux := http.NewServeMux()
	// Landing page at web root (server-4i67)
	mux.Handle("/", web.IndexHandler())
	mux.Handle("/robots.txt", web.RobotsTxtHandler())
	mux.Handle("/sitemap.xml", web.SitemapHandler())
	mux.Handle("/health", healthChecker.Health()) // Comprehensive health check (T011)
	mux.Handle("/healthz", handlers.Healthz())    // Legacy liveness check
	mux.Handle("/readyz", healthChecker.Readyz()) // Readiness check with dependency verification
	mux.Handle("/version", VersionHandler(version, gitCommit, buildDate))
	mux.Handle("/api/v1/openapi.json", OpenAPIHandler())
	mux.Handle("/api/v1/openapi.yaml", OpenAPIYAMLHandler()) // YAML format (server-v7yn)
	mux.Handle("/api/docs/", web.APIDocsHandler())           // Scalar API documentation UI (server-6lnc)
	mux.Handle("/api/docs", web.APIDocsHandler())            // Scalar API documentation UI (server-6lnc)
	mux.Handle("/.well-known/sel-profile", http.HandlerFunc(wellKnownHandler.SELProfile))

	// Prometheus metrics endpoint (FR-022)
	mux.Handle("/metrics", promhttp.HandlerFor(
		metrics.Registry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true, // Support OpenMetrics format
		},
	))

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
	createEvents := apiKeyAuth(rateLimitAgent(middleware.EventRequestSize()(http.HandlerFunc(eventsHandler.Create))))
	createBatch := apiKeyAuth(rateLimitAgent(middleware.FederationRequestSize()(http.HandlerFunc(eventsHandler.CreateBatch))))
	batchStatus := rateLimitPublic(http.HandlerFunc(eventsHandler.GetBatchStatus))

	mux.Handle("/api/v1/events", methodMux(map[string]http.Handler{
		http.MethodGet:  publicEvents,
		http.MethodPost: createEvents,
	}))
	mux.Handle("/api/v1/events:batch", methodMux(map[string]http.Handler{
		http.MethodPost: createBatch,
	}))
	mux.Handle("/api/v1/batch-status/{id}", batchStatus)
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
	mux.Handle("/api/v1/admin/login", rateLimitLogin(middleware.AdminRequestSize()(http.HandlerFunc(adminAuthHandler.Login))))
	mux.Handle("/api/v1/admin/logout", http.HandlerFunc(adminAuthHandler.Logout))

	// Admin event management endpoints (requires JWT auth)
	jwtAuth := middleware.JWTAuth(jwtManager, cfg.Environment)
	adminRateLimit := middleware.WithRateLimitTierHandler(middleware.TierAdmin)

	adminPendingEvents := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.ListPendingEvents))))
	mux.Handle("/api/v1/admin/events/pending", adminPendingEvents)

	adminListEvents := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.ListEvents))))
	mux.Handle("/api/v1/admin/events", methodMux(map[string]http.Handler{
		http.MethodGet: adminListEvents,
	}))

	adminUpdateEvent := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.UpdateEvent))))
	adminDeleteEvent := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.DeleteEvent))))
	mux.Handle("/api/v1/admin/events/{id}", methodMux(map[string]http.Handler{
		http.MethodPut:    adminUpdateEvent,
		http.MethodDelete: adminDeleteEvent,
	}))

	adminPublishEvent := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.PublishEvent))))
	mux.Handle("/api/v1/admin/events/{id}/publish", methodMux(map[string]http.Handler{
		http.MethodPost: adminPublishEvent,
	}))

	adminUnpublishEvent := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.UnpublishEvent))))
	mux.Handle("/api/v1/admin/events/{id}/unpublish", methodMux(map[string]http.Handler{
		http.MethodPost: adminUnpublishEvent,
	}))

	adminMergeEvents := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.MergeEvents))))
	mux.Handle("/api/v1/admin/events/merge", methodMux(map[string]http.Handler{
		http.MethodPost: adminMergeEvents,
	}))

	adminDeletePlace := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.DeletePlace))))
	mux.Handle("/api/v1/admin/places/{id}", methodMux(map[string]http.Handler{
		http.MethodDelete: adminDeletePlace,
	}))

	adminDeleteOrganization := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.DeleteOrganization))))
	mux.Handle("/api/v1/admin/organizations/{id}", methodMux(map[string]http.Handler{
		http.MethodDelete: adminDeleteOrganization,
	}))

	// Admin API key management (T078)
	adminCreateAPIKey := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(apiKeyHandler.CreateAPIKey))))
	adminListAPIKeys := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(apiKeyHandler.ListAPIKeys))))
	adminRevokeAPIKey := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(apiKeyHandler.RevokeAPIKey))))

	mux.Handle("/api/v1/admin/api-keys", methodMux(map[string]http.Handler{
		http.MethodPost: adminCreateAPIKey,
		http.MethodGet:  adminListAPIKeys,
	}))
	mux.Handle("/api/v1/admin/api-keys/{id}", methodMux(map[string]http.Handler{
		http.MethodDelete: adminRevokeAPIKey,
	}))

	// Admin federation node management (T081b)
	adminCreateNode := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(federationHandler.CreateNode))))
	adminListNodes := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(federationHandler.ListNodes))))
	adminGetNode := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(federationHandler.GetNode))))
	adminUpdateNode := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(federationHandler.UpdateNode))))
	adminDeleteNode := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(federationHandler.DeleteNode))))

	mux.Handle("/api/v1/admin/federation/nodes", methodMux(map[string]http.Handler{
		http.MethodPost: adminCreateNode,
		http.MethodGet:  adminListNodes,
	}))
	mux.Handle("/api/v1/admin/federation/nodes/{id}", methodMux(map[string]http.Handler{
		http.MethodGet:    adminGetNode,
		http.MethodPut:    adminUpdateNode,
		http.MethodDelete: adminDeleteNode,
	}))

	// Federation change feed endpoint (T111 - public, rate limited)
	changeFeedList := rateLimitPublic(http.HandlerFunc(feedsHandler.ListChanges))
	mux.Handle("/api/v1/feeds/changes", changeFeedList)

	// Federation sync endpoint (T111 - requires API key auth, federation rate limit, idempotency support)
	rateLimitFederation := middleware.WithRateLimitTierHandler(middleware.TierFederation)
	federationSync := apiKeyAuth(rateLimitFederation(middleware.Idempotency(middleware.FederationRequestSize()(http.HandlerFunc(federationHandler.Sync)))))
	mux.Handle("/api/v1/federation/sync", methodMux(map[string]http.Handler{
		http.MethodPost: federationSync,
	}))

	// Admin HTML pages (T080) - placeholder routes for future implementation
	// Apply CSRF protection to cookie-based admin routes to prevent cross-site request forgery
	adminCookieAuth := middleware.AdminAuthCookie(jwtManager)

	// CSRF middleware - only initialize if CSRF key is configured
	var csrfMiddleware func(http.Handler) http.Handler
	if cfg.Auth.CSRFKey != "" {
		// Secure flag: enabled for non-development environments
		requireHTTPS := cfg.Environment != "development" && cfg.Environment != "test"
		csrfMiddleware = middleware.CSRFProtection([]byte(cfg.Auth.CSRFKey), requireHTTPS)
	} else {
		// No-op middleware if CSRF key not configured (development mode)
		csrfMiddleware = func(next http.Handler) http.Handler { return next }
		if cfg.Environment != "development" && cfg.Environment != "test" {
			logger.Warn().Msg("CSRF_KEY not configured - admin routes vulnerable to CSRF attacks")
		}
	}

	// Admin routes with CSRF protection
	adminHTML := csrfMiddleware(adminCookieAuth(http.HandlerFunc(handlers.AdminHTMLPlaceholder(cfg.Environment))))
	mux.Handle("/admin", adminHTML)
	mux.Handle("/admin/", adminHTML)

	// Login page doesn't need CSRF (no auth required, no state-changing action on GET)
	mux.Handle("/admin/login", http.HandlerFunc(adminAuthHandler.LoginPage))

	// Wrap entire router with middleware stack
	// Order: SecurityHeaders -> CORS -> CorrelationID -> RequestLogging -> RateLimit -> HTTPMetrics
	// Note: Security headers and CORS must be applied first to ensure they're set on all responses
	// HTTPMetrics is last (innermost) so it captures actual handler latency, not middleware overhead
	handler := middleware.SecurityHeaders(requireHTTPS)(mux)
	handler = middleware.CORS(cfg.CORS, logger)(handler)
	handler = middleware.CorrelationID(logger)(handler)
	handler = middleware.RequestLogging(logger)(handler)
	handler = middleware.RateLimit(cfg.RateLimit)(handler)
	handler = metrics.HTTPMiddleware(handler)

	return &RouterWithClient{
		Handler:     handler,
		RiverClient: riverClient,
	}
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

// findShapesDirectory locates the shapes/ directory relative to the project root.
func findShapesDirectory() string {
	// Try common locations relative to the executable
	candidates := []string{
		"shapes",                      // Same directory as executable
		"../shapes",                   // One level up (development)
		"../../shapes",                // Two levels up
		"/app/shapes",                 // Container deployment
		"/usr/local/share/sel/shapes", // System-wide installation
	}

	for _, candidate := range candidates {
		if absPath, err := filepath.Abs(candidate); err == nil {
			if info, err := os.Stat(absPath); err == nil && info.IsDir() {
				return absPath
			}
		}
	}

	// Fallback: try to find via working directory
	if wd, err := os.Getwd(); err == nil {
		// Walk up from working directory looking for go.mod (repo root)
		dir := wd
		for i := 0; i < maxRouterRetries; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				shapesPath := filepath.Join(dir, "shapes")
				if info, err := os.Stat(shapesPath); err == nil && info.IsDir() {
					return shapesPath
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// Last resort: return relative path and let validator handle error
	return "shapes"
}

// loadAdminTemplates loads HTML templates for the admin UI.
// Returns nil if templates cannot be found (admin UI will gracefully degrade).
func loadAdminTemplates() (*template.Template, error) {
	// Try common locations for the templates directory
	candidates := []string{
		"web/admin/templates",                      // From project root
		"../web/admin/templates",                   // One level up
		"../../web/admin/templates",                // Two levels up
		"/app/web/admin/templates",                 // Container deployment
		"/usr/local/share/sel/web/admin/templates", // System-wide installation
	}

	for _, candidate := range candidates {
		if absPath, err := filepath.Abs(candidate); err == nil {
			if info, err := os.Stat(absPath); err == nil && info.IsDir() {
				// Found templates directory, parse all .html files
				pattern := filepath.Join(absPath, "*.html")
				if tmpl, err := template.ParseGlob(pattern); err == nil {
					return tmpl, nil
				}
			}
		}
	}

	// Try to find via working directory
	if wd, err := os.Getwd(); err == nil {
		// Walk up from working directory looking for go.mod (repo root)
		dir := wd
		for i := 0; i < maxRouterRetries; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				templatesPath := filepath.Join(dir, "web", "admin", "templates")
				if info, err := os.Stat(templatesPath); err == nil && info.IsDir() {
					pattern := filepath.Join(templatesPath, "*.html")
					if tmpl, err := template.ParseGlob(pattern); err == nil {
						return tmpl, nil
					}
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return nil, os.ErrNotExist
}
