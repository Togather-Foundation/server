package api

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
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
	"github.com/Togather-Foundation/server/internal/auth/oauth"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/federation"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/domain/provenance"
	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"

	"github.com/Togather-Foundation/server/internal/domain/users"
	"github.com/Togather-Foundation/server/internal/email"
	"github.com/Togather-Foundation/server/internal/geocoding"
	"github.com/Togather-Foundation/server/internal/geocoding/nominatim"
	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/Togather-Foundation/server/internal/jsonld"
	"github.com/Togather-Foundation/server/internal/kg"
	"github.com/Togather-Foundation/server/internal/kg/artsdata"
	"github.com/Togather-Foundation/server/internal/mcp"
	"github.com/Togather-Foundation/server/internal/metrics"
	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/Togather-Foundation/server/internal/sse"
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
	Handler       http.Handler
	RiverClient   *river.Client[pgx.Tx]
	UsageRecorder *developers.UsageRecorder
	Broker        *sse.Broker
}

func NewRouter(cfg config.Config, logger zerolog.Logger, pool *pgxpool.Pool, version, gitCommit, buildDate string, shutdownCtx ...context.Context) *RouterWithClient {
	repo, err := postgres.NewRepository(pool)
	if err != nil {
		logger.Error().Err(err).Msg("repository init failed")
		return &RouterWithClient{
			Handler:     http.NewServeMux(),
			RiverClient: nil,
			Broker:      sse.NewBroker(),
		}
	}

	eventsService := events.NewService(repo.Events())
	ingestService := events.NewIngestService(repo.Events(), cfg.Server.BaseURL, cfg.DefaultTimezone, cfg.Validation).WithDedupConfig(cfg.Dedup)
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())
	provenanceService := provenance.NewService(repo.Provenance())

	// Initialize geocoding service (srv-28gtj)
	// Create HTTP client with configured timeout (srv-v89f4)
	httpClient := &http.Client{
		Timeout: time.Duration(cfg.Geocoding.NominatimTimeoutSeconds) * time.Second,
	}
	nominatimClient := nominatim.NewClient(
		cfg.Geocoding.NominatimAPIURL,
		cfg.Geocoding.NominatimUserEmail,
		nominatim.WithHTTPClient(httpClient),
		nominatim.WithRateLimit(cfg.Geocoding.NominatimRateLimitPerSec),
	)
	geocodingCache := postgres.NewGeocodingCacheRepository(pool)
	geocodingService := geocoding.NewGeocodingService(nominatimClient, geocodingCache, logger, cfg.Geocoding.DefaultCountry)

	// Create SQLc queries instance for direct database access
	queries := postgres.New(pool)

	// Derive separate JWT signing keys for admin and developer tokens (srv-yuyg9)
	// IMPORTANT: This is a breaking change - existing tokens will be invalidated when deployed
	// Using HKDF-SHA256 to derive cryptographically independent keys prevents token confusion attacks
	masterSecret := []byte(cfg.Auth.JWTSecret)
	adminJWTKey, err := auth.DeriveAdminJWTKey(masterSecret)
	if err != nil {
		logger.Error().Err(err).Msg("failed to derive admin JWT key")
		return &RouterWithClient{
			Handler:     http.NewServeMux(),
			RiverClient: nil,
			Broker:      sse.NewBroker(),
		}
	}
	developerJWTKey, err := auth.DeriveDeveloperJWTKey(masterSecret)
	if err != nil {
		logger.Error().Err(err).Msg("failed to derive developer JWT key")
		return &RouterWithClient{
			Handler:     http.NewServeMux(),
			RiverClient: nil,
			Broker:      sse.NewBroker(),
		}
	}

	// Get deployment slot for metrics labeling (blue/green)
	slot := os.Getenv("SLOT")
	if slot == "" {
		slot = "unknown"
	}

	// Initialize River job queue for batch processing
	slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Knowledge graph reconciliation service (srv-titkr)
	// Use a KGService interface variable (not a typed *kg.ReconciliationService pointer)
	// to avoid the Go nil-interface trap: a typed nil pointer converts to a non-nil
	// interface value, causing NewWorkersWithPool to register workers with a nil service.
	var reconciliationService jobs.KGService
	if cfg.Artsdata.Enabled {
		// Create HTTP client with configured timeout
		artsdataHTTPClient := &http.Client{
			Timeout: time.Duration(cfg.Artsdata.TimeoutSeconds) * time.Second,
		}
		artsdataClient := artsdata.NewClient(
			cfg.Artsdata.Endpoint,
			artsdata.WithHTTPClient(artsdataHTTPClient),
			artsdata.WithRateLimit(cfg.Artsdata.RateLimitPerSec),
		)
		reconciliationService = kg.NewReconciliationService(
			artsdataClient,
			queries,
			slogLogger,
			time.Duration(cfg.Artsdata.CacheTTLDays)*24*time.Hour,
			time.Duration(cfg.Artsdata.FailureTTLDays)*24*time.Hour,
		)
	}

	// Create a single ScraperSubmissionRepository shared by workers and handlers (srv-rgmjl).
	submissionRepo := postgres.NewScraperSubmissionRepository(pool)

	workers := jobs.NewWorkersWithPool(pool, ingestService, repo.Events(), geocodingService, reconciliationService, placesService, orgService, slogLogger, slot, submissionRepo)

	// Create River metrics hook for Prometheus monitoring
	riverHooks := []rivertype.Hook{
		metrics.NewRiverMetricsHook(slot),
	}

	// Build scraper for periodic jobs and admin trigger (srv-pfeud).
	// Use the server's own base URL and an API key from env for self-ingestion.
	var scraperSvc *scraper.Scraper
	ingestAPIKey := os.Getenv("SEL_API_KEY")
	if ingestAPIKey == "" {
		ingestAPIKey = os.Getenv("SEL_INGEST_KEY")
	}
	if ingestAPIKey != "" {
		scraperSourceRepo := postgres.NewScraperSourceRepository(pool)
		ingestClient := scraper.NewIngestClient(cfg.Server.BaseURL, ingestAPIKey)
		scraperSvc = scraper.NewScraperWithSourceRepoAndSlot(ingestClient, queries, scraperSourceRepo, logger, slot)
		logger.Info().Msg("router: scraper configured for periodic jobs and admin trigger")

		// Wire Tier 2 headless browser extractor when enabled (srv-1zrrt).
		if cfg.Scraper.HeadlessEnabled {
			rodExt := scraper.NewRodExtractor(
				logger.With().Str("component", "rod").Logger(),
				cfg.Scraper.HeadlessMaxConc,
				cfg.Scraper.ChromePath,
				true,
			)
			scraperSvc.SetRodExtractor(rodExt)
			logger.Info().Msg("scraper: Tier 2 headless browser enabled")
		}
	} else {
		logger.Warn().Msg("router: SEL_API_KEY/SEL_INGEST_KEY not set — periodic scrape jobs and admin trigger disabled")
	}

	// Load source configs for periodic job registration.
	// TODO(srv-ephoo): Load sources from DB (with YAML fallback) so dynamically
	// added/removed sources are picked up without a server restart.
	var sourceCfgs []scraper.SourceConfig
	if scraperSvc != nil {
		var loadErr error
		sourceCfgs, loadErr = scraper.LoadSourceConfigs("configs/sources")
		if loadErr != nil {
			logger.Warn().Err(loadErr).Msg("router: failed to load source configs for periodic jobs (non-fatal)")
		}
	}

	// Register scrape worker when scraper is available.
	if scraperSvc != nil {
		workers = jobs.NewWorkersWithScraper(pool, ingestService, repo.Events(), geocodingService, reconciliationService, placesService, orgService, slogLogger, slot, scraperSvc, queries, submissionRepo)
	}

	// Configure periodic cleanup jobs and per-source scrape jobs.
	periodicJobs := jobs.NewPeriodicJobsFromSources(sourceCfgs)

	riverClient, err := jobs.NewClient(pool, workers, slogLogger, riverHooks, periodicJobs)
	if err != nil {
		logger.Error().Err(err).Msg("river client init failed")
		return &RouterWithClient{
			Handler:     http.NewServeMux(),
			RiverClient: nil,
			Broker:      sse.NewBroker(),
		}
	}
	// Note: River workers are started in cmd/server/cmd/serve.go with proper lifecycle management
	// DO NOT call riverClient.Start() here - it's handled during server initialization for proper graceful shutdown

	// Set up SSE broker to fan River job events to connected admin clients.
	// Subscribe must be called before riverClient.Start (called in serve.go).
	var sseShutdownCtx context.Context
	if len(shutdownCtx) > 0 && shutdownCtx[0] != nil {
		sseShutdownCtx = shutdownCtx[0]
	} else {
		sseShutdownCtx = context.Background()
	}
	broker := sse.NewBroker()
	subCh, sseUnsub := riverClient.Subscribe(
		river.EventKindJobCompleted,
		river.EventKindJobFailed,
		river.EventKindJobCancelled,
	)
	broker.Start(sseShutdownCtx, subCh)
	// Release the River subscription when the server shuts down.
	go func() { <-sseShutdownCtx.Done(); sseUnsub() }()

	// Load configured timezone for default date filtering (srv-h7j38)
	loc, err := time.LoadLocation(cfg.DefaultTimezone)
	if err != nil {
		logger.Warn().Err(err).Str("timezone", cfg.DefaultTimezone).Msg("invalid DefaultTimezone, falling back to UTC")
		loc = time.UTC
	}

	eventsHandler := handlers.NewEventsHandler(eventsService, ingestService, provenanceService, riverClient, queries, cfg.Environment, cfg.Server.BaseURL).
		WithPlaceResolver(placesService).
		WithOrgResolver(orgService)
	eventsHandler.Loc = loc
	placesHandler := handlers.NewPlacesHandler(placesService, cfg.Environment, cfg.Server.BaseURL).WithGeocodingService(geocodingService)
	placesHandler.Loc = loc
	orgHandler := handlers.NewOrganizationsHandler(orgService, cfg.Environment, cfg.Server.BaseURL)
	orgHandler.Loc = loc

	// Wire scraper source repo into org/place handlers for sel:scraperSource linkage (best-effort).
	scraperSourceRepo := postgres.NewScraperSourceRepository(pool)
	placesHandler = placesHandler.WithScraperSourceRepo(scraperSourceRepo)
	orgHandler = orgHandler.WithScraperSourceRepo(scraperSourceRepo)

	// Create geocoding handler (srv-28gtj)
	geocodingHandler := handlers.NewGeocodingHandler(geocodingService, cfg.Environment)

	// Create audit logger for admin operations
	auditLogger := audit.NewLoggerWithZerolog(logger)

	// Initialize email service
	// Resolve template directory path (config may be relative, make it absolute if needed)
	templateDir := cfg.Email.TemplatesDir
	if !filepath.IsAbs(templateDir) {
		// If relative path, resolve from repo root
		repoRoot := findRepoRoot()
		templateDir = filepath.Join(repoRoot, templateDir)
	}

	emailService, err := email.NewService(cfg.Email, templateDir, logger)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to initialize email service, user invitations will not be sent")
	}

	// Initialize user service
	baseURL := fmt.Sprintf("https://%s", cfg.Server.PublicURL) // For invitation links
	userService := users.NewService(pool, emailService, auditLogger, baseURL, logger, cfg.Users)

	// Initialize developer service (srv-x7vv0)
	developerRepo := postgres.NewDeveloperRepositoryAdapter(pool)
	developerService := developers.NewService(developerRepo, logger, cfg.Developer)

	// Initialize API key usage recorder (srv-8r58k)
	usageRepo := postgres.NewUsageRepository(pool)
	usageRecorder := developers.NewUsageRecorder(usageRepo, logger, cfg.Developer)
	// Note: usageRecorder.Start() is called in cmd/server/cmd/serve.go for proper lifecycle management
	// DO NOT call usageRecorder.Start() here

	// Developer auth and API key handlers (srv-x7vv0)
	devAuthHandler := handlers.NewDeveloperAuthHandler(
		developerService,
		logger,
		developerJWTKey, // Use derived developer JWT key (srv-yuyg9)
		cfg.Auth.JWTExpiry,
		"sel.events",
		cfg.Environment,
		auditLogger,
	)
	devAPIKeyHandler := handlers.NewDeveloperAPIKeyHandler(
		developerService,
		logger,
		cfg.Environment,
		auditLogger,
	)

	// GitHub OAuth handler (srv-idczk) - only initialize if configured
	var devOAuthHandler *handlers.DeveloperOAuthHandler
	if cfg.Auth.GitHub.ClientID != "" {
		githubClient := oauth.NewGitHubClient(oauth.GitHubConfig{
			ClientID:     cfg.Auth.GitHub.ClientID,
			ClientSecret: cfg.Auth.GitHub.ClientSecret,
			CallbackURL:  cfg.Auth.GitHub.CallbackURL,
			AllowedOrgs:  cfg.Auth.GitHub.AllowedOrgs,
		})
		devOAuthHandler = handlers.NewDeveloperOAuthHandler(
			developerService,
			githubClient,
			logger,
			developerJWTKey, // Use derived developer JWT key (srv-yuyg9)
			cfg.Auth.JWTExpiry,
			"sel.events",
			cfg.Environment,
			auditLogger,
		)
	}

	// Load admin templates
	adminTemplates, err := loadAdminTemplates(gitCommit)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load admin templates, admin UI will be unavailable")
	}

	// Load developer templates (srv-7m0cf)
	devTemplates, err := loadDevTemplates(gitCommit)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load developer templates, developer portal UI will be unavailable")
	}

	// Admin handlers
	jwtManager := auth.NewJWTManagerFromKey(adminJWTKey, cfg.Auth.JWTExpiry, "sel.events") // Use derived admin JWT key (srv-yuyg9)
	adminAuthHandler := handlers.NewAdminAuthHandler(queries, jwtManager, auditLogger, cfg.Environment, adminTemplates, cfg.Auth.JWTExpiry)
	adminHTMLHandler := handlers.NewAdminHTMLHandler(adminTemplates, cfg.Environment, slogLogger, cfg.Email.Enabled)

	// Developer HTML handler (srv-7m0cf)
	devHTMLHandler := handlers.NewDevHTMLHandler(devTemplates, cfg.Environment, slogLogger, developerService, devOAuthHandler != nil)

	// Admin user management handlers
	adminUsersHandler := handlers.NewAdminUsersHandler(userService, auditLogger, cfg.Environment)
	invitationsHandler := handlers.NewInvitationsHandler(userService, auditLogger, cfg.Environment)

	// Admin developer management handlers (srv-2q4ic)
	adminDevelopersHandler := handlers.NewAdminDevelopersHandler(developerService, developerRepo, auditLogger, cfg.Environment)

	// Create AdminService
	requireHTTPS := cfg.Environment == "production"
	adminService := events.NewAdminService(repo.Events(), requireHTTPS, cfg.DefaultTimezone, cfg.Validation)
	adminHandler := handlers.NewAdminHandler(eventsService, adminService, placesService, orgService, auditLogger, queries, cfg.Environment, cfg.Server.BaseURL)
	adminHandler.Loc = loc

	// Create API Key handler
	apiKeyHandler := handlers.NewAPIKeyHandler(queries, cfg.Environment)

	// Create Admin Review Queue handler (srv-bjo)
	adminReviewQueueHandler := handlers.NewAdminReviewQueueHandler(repo.Events(), adminService, auditLogger, cfg.Environment, cfg.Server.BaseURL)

	// Create scraper submission handlers (srv-1cxmi); uses the shared submissionRepo declared above.
	submissionService := domainScraper.NewSubmissionService(submissionRepo, cfg.RateLimit.SubmissionsPerIPPer24h)
	submissionHandler := handlers.NewScraperSubmissionHandler(submissionService, cfg.Environment)
	adminSubmissionHandler := handlers.NewAdminScraperSubmissionHandler(submissionRepo, cfg.Environment)

	// Create Admin Scraper handler (srv-5127b)
	// TriggerScrape enqueues a River ScrapeSourceJob — same path as scheduled scrapes.
	adminScraperHandler := &handlers.AdminScraperHandler{
		Queries:     queries,
		Logger:      logger.With().Str("component", "scraper").Logger(),
		Env:         cfg.Environment,
		RiverClient: riverClient,
	}

	// Create Admin Geocoding handler (srv-qq7o1)
	adminGeocodingHandler := handlers.NewAdminGeocodingHandler(pool, riverClient, slogLogger, cfg.Environment)

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

	// Stats endpoint for public server statistics (server-71ua)
	statsHandler := handlers.NewStatsHandler(queries, version, gitCommit, time.Now(), cfg.Environment)

	mux := http.NewServeMux()
	// Landing page at web root (server-4i67)
	mux.Handle("/", web.IndexHandler())
	mux.Handle("/robots.txt", web.RobotsTxtHandler())
	mux.Handle("/sitemap.xml", web.SitemapHandler())
	mux.Handle("/llms.txt", web.LLMsTxtHandler())
	mux.Handle("/health", healthChecker.Health()) // Comprehensive health check (T011)
	mux.Handle("/healthz", handlers.Healthz())    // Legacy liveness check
	mux.Handle("/readyz", healthChecker.Readyz()) // Readiness check with dependency verification
	mux.Handle("/version", VersionHandler(version, gitCommit, buildDate))
	mux.Handle("/api/v1/stats", http.HandlerFunc(statsHandler.GetStats)) // Public server statistics (server-71ua)
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

	// MCP endpoint (optional, disabled by default)
	if os.Getenv("MCP_HTTP_ENABLED") == "true" {
		transportCfg, err := mcp.LoadTransportConfig()
		if err != nil {
			logger.Warn().Err(err).Msg("invalid MCP transport config; MCP endpoint disabled")
		} else {
			transportCfg.Type = mcp.TransportHTTP
			mcpServer := mcp.NewServer(
				mcp.Config{
					Name:      "Togather SEL MCP Server",
					Version:   version,
					Transport: string(transportCfg.Type),
				},
				eventsService,
				ingestService,
				placesService,
				orgService,
				developerService,
				geocodingService,
				loc,
				cfg.Server.BaseURL,
			)

			mcpHandler, err := mcp.WrapHandler(
				mcp.NewStreamableHTTPHandler(mcpServer.MCPServer()),
				repo.Auth().APIKeys(),
				cfg.RateLimit,
			)
			if err != nil {
				logger.Warn().Err(err).Msg("failed to wrap MCP handler; MCP endpoint disabled")
			} else {
				mux.Handle("/mcp", mcpHandler)
			}
		}
	}

	// Middleware setup
	apiKeyRepo := repo.Auth().APIKeys()
	apiKeyAuth := middleware.AgentAuth(apiKeyRepo)
	rateLimitPublic := middleware.WithRateLimitTierHandler(middleware.TierPublic)
	rateLimitAgent := middleware.WithRateLimitTierHandler(middleware.TierAgent)
	usageTracking := middleware.UsageTracking(usageRecorder, logger)

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
	createEvents := apiKeyAuth(usageTracking(rateLimitAgent(middleware.EventRequestSize()(http.HandlerFunc(eventsHandler.Create)))))
	createBatch := apiKeyAuth(usageTracking(rateLimitAgent(middleware.FederationRequestSize()(http.HandlerFunc(eventsHandler.CreateBatch)))))
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

	// Geocoding endpoints (srv-28gtj, srv-4xnt8)
	publicGeocode := rateLimitPublic(http.HandlerFunc(geocodingHandler.Geocode))
	publicReverseGeocode := rateLimitPublic(http.HandlerFunc(geocodingHandler.ReverseGeocode))
	mux.Handle("/api/v1/geocode", publicGeocode)
	mux.Handle("/api/v1/reverse-geocode", publicReverseGeocode)

	// Public URL submission endpoint (srv-1cxmi)
	publicSubmitURLs := rateLimitPublic(http.HandlerFunc(submissionHandler.SubmitURLs))
	mux.Handle("POST /api/v1/scraper/submissions", publicSubmitURLs)

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

	// Admin stats endpoint for dashboard (server-m11c)
	adminStats := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.GetStats))))
	mux.Handle("GET /api/v1/admin/stats", adminStats)

	adminPendingEvents := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.ListPendingEvents))))
	mux.Handle("/api/v1/admin/events/pending", adminPendingEvents)

	adminListEvents := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.ListEvents))))
	mux.Handle("GET /api/v1/admin/events", adminListEvents)

	adminUpdateEvent := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.UpdateEvent))))
	adminDeleteEvent := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.DeleteEvent))))
	mux.Handle("/api/v1/admin/events/{id}", methodMux(map[string]http.Handler{
		http.MethodPut:    adminUpdateEvent,
		http.MethodDelete: adminDeleteEvent,
	}))

	adminPublishEvent := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.PublishEvent))))
	mux.Handle("POST /api/v1/admin/events/{id}/publish", adminPublishEvent)

	adminUnpublishEvent := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.UnpublishEvent))))
	mux.Handle("POST /api/v1/admin/events/{id}/unpublish", adminUnpublishEvent)

	adminMergeEvents := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.MergeEvents))))
	mux.Handle("POST /api/v1/admin/events/merge", adminMergeEvents)

	adminListDuplicates := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.ListDuplicates))))
	mux.Handle("GET /api/v1/admin/duplicates", adminListDuplicates)

	adminDeletePlace := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.DeletePlace))))
	adminUpdatePlace := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.UpdatePlace))))
	adminGetPlace := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.GetPlace))))
	mux.Handle("/api/v1/admin/places/{id}", methodMux(map[string]http.Handler{
		http.MethodDelete: adminDeletePlace,
		http.MethodPut:    adminUpdatePlace,
		http.MethodGet:    adminGetPlace,
	}))

	adminDeleteOrganization := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.DeleteOrganization))))
	adminUpdateOrganization := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.UpdateOrganization))))
	adminGetOrganization := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.GetOrganization))))
	mux.Handle("/api/v1/admin/organizations/{id}", methodMux(map[string]http.Handler{
		http.MethodDelete: adminDeleteOrganization,
		http.MethodPut:    adminUpdateOrganization,
		http.MethodGet:    adminGetOrganization,
	}))

	// Admin place similarity and merge endpoints
	adminFindSimilarPlaces := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.FindSimilarPlaces))))
	mux.Handle("GET /api/v1/admin/places/{id}/similar", adminFindSimilarPlaces)

	adminMergePlaces := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.MergePlaces))))
	mux.Handle("POST /api/v1/admin/places/merge", adminMergePlaces)

	// Admin organization similarity and merge endpoints
	adminFindSimilarOrgs := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.FindSimilarOrganizations))))
	mux.Handle("GET /api/v1/admin/organizations/{id}/similar", adminFindSimilarOrgs)

	adminMergeOrgs := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminHandler.MergeOrganizations))))
	mux.Handle("POST /api/v1/admin/organizations/merge", adminMergeOrgs)

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

	// Admin review queue management (srv-bjo)
	adminListReviews := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminReviewQueueHandler.ListReviewQueue))))
	adminGetReview := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminReviewQueueHandler.GetReviewQueueEntry))))
	adminApproveReview := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminReviewQueueHandler.ApproveReview))))
	adminRejectReview := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminReviewQueueHandler.RejectReview))))
	adminFixReview := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminReviewQueueHandler.FixReview))))
	adminMergeReview := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminReviewQueueHandler.MergeReview))))
	adminAddOccurrenceReview := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminReviewQueueHandler.AddOccurrenceReview))))

	mux.Handle("GET /api/v1/admin/review-queue", adminListReviews)
	mux.Handle("GET /api/v1/admin/review-queue/{id}", adminGetReview)
	mux.Handle("POST /api/v1/admin/review-queue/{id}/approve", adminApproveReview)
	mux.Handle("POST /api/v1/admin/review-queue/{id}/reject", adminRejectReview)
	mux.Handle("POST /api/v1/admin/review-queue/{id}/fix", adminFixReview)
	mux.Handle("POST /api/v1/admin/review-queue/{id}/merge", adminMergeReview)
	mux.Handle("POST /api/v1/admin/review-queue/{id}/add-occurrence", adminAddOccurrenceReview)

	// Admin geocoding backfill (srv-qq7o1)
	adminGeocodingBackfill := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminGeocodingHandler.Backfill))))
	mux.Handle("POST /api/v1/admin/geocoding/backfill", adminGeocodingBackfill)

	// Admin user management (user administration system)
	adminCreateUser := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminUsersHandler.CreateUser))))
	adminListUsers := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminUsersHandler.ListUsers))))
	adminGetUser := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminUsersHandler.GetUser))))
	adminUpdateUser := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminUsersHandler.UpdateUser))))
	adminDeleteUser := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminUsersHandler.DeleteUser))))
	adminActivateUser := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminUsersHandler.ActivateUser))))
	adminDeactivateUser := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminUsersHandler.DeactivateUser))))
	adminResendInvitation := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminUsersHandler.ResendInvitation))))
	adminGetUserActivity := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminUsersHandler.GetUserActivity))))

	mux.Handle("/api/v1/admin/users", methodMux(map[string]http.Handler{
		http.MethodPost: adminCreateUser,
		http.MethodGet:  adminListUsers,
	}))
	mux.Handle("/api/v1/admin/users/{id}", methodMux(map[string]http.Handler{
		http.MethodGet:    adminGetUser,
		http.MethodPut:    adminUpdateUser,
		http.MethodDelete: adminDeleteUser,
	}))
	mux.Handle("POST /api/v1/admin/users/{id}/activate", adminActivateUser)
	mux.Handle("POST /api/v1/admin/users/{id}/deactivate", adminDeactivateUser)
	mux.Handle("POST /api/v1/admin/users/{id}/resend-invitation", adminResendInvitation)
	mux.Handle("GET /api/v1/admin/users/{id}/activity", adminGetUserActivity)

	// Admin developer management (srv-2q4ic)
	adminInviteDeveloper := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminDevelopersHandler.InviteDeveloper))))
	adminListDevelopers := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminDevelopersHandler.ListDevelopers))))
	adminGetDeveloper := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminDevelopersHandler.GetDeveloper))))
	adminUpdateDeveloper := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminDevelopersHandler.UpdateDeveloper))))
	adminDeactivateDeveloper := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminDevelopersHandler.DeactivateDeveloper))))

	mux.Handle("POST /api/v1/admin/developers/invite", adminInviteDeveloper)
	mux.Handle("/api/v1/admin/developers", methodMux(map[string]http.Handler{
		http.MethodGet: adminListDevelopers,
	}))
	mux.Handle("/api/v1/admin/developers/{id}", methodMux(map[string]http.Handler{
		http.MethodGet:    adminGetDeveloper,
		http.MethodPut:    adminUpdateDeveloper,
		http.MethodDelete: adminDeactivateDeveloper,
	}))

	// Admin scraper source management (srv-5127b)
	adminScraperListSources := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminScraperHandler.ListSources))))
	adminScraperListRuns := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminScraperHandler.ListSourceRuns))))
	adminScraperTrigger := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminScraperHandler.TriggerScrape))))
	adminScraperSetEnabled := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminScraperHandler.SetSourceEnabled))))
	adminScraperGetConfig := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminScraperHandler.GetConfig))))
	adminScraperPatchConfig := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminScraperHandler.PatchConfig))))

	mux.Handle("GET /api/v1/admin/scraper/sources", adminScraperListSources)
	mux.Handle("GET /api/v1/admin/scraper/sources/{name}/runs", adminScraperListRuns)
	mux.Handle("POST /api/v1/admin/scraper/sources/{name}/trigger", adminScraperTrigger)
	mux.Handle("PATCH /api/v1/admin/scraper/sources/{name}", adminScraperSetEnabled)
	mux.Handle("GET /api/v1/admin/scraper/config", adminScraperGetConfig)
	mux.Handle("PATCH /api/v1/admin/scraper/config", adminScraperPatchConfig)

	// Admin scraper submission management (srv-1cxmi)
	adminListSubmissions := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminSubmissionHandler.ListSubmissions))))
	adminUpdateSubmission := jwtAuth(adminRateLimit(middleware.AdminRequestSize()(http.HandlerFunc(adminSubmissionHandler.UpdateSubmission))))
	mux.Handle("GET /api/v1/admin/scraper/submissions", adminListSubmissions)
	mux.Handle("PATCH /api/v1/admin/scraper/submissions/{id}", adminUpdateSubmission)

	// Public invitation acceptance endpoint (NO AUTH)
	publicAcceptInvitation := rateLimitPublic(middleware.AdminRequestSize()(http.HandlerFunc(invitationsHandler.AcceptInvitation)))
	mux.Handle("POST /api/v1/accept-invitation", publicAcceptInvitation)

	// Developer API routes (srv-x7vv0)
	// Developer auth endpoints with rate limiting and body size limits
	devLogin := rateLimitLogin(middleware.AdminRequestSize()(http.HandlerFunc(devAuthHandler.Login)))
	devAcceptInvitation := rateLimitPublic(middleware.AdminRequestSize()(http.HandlerFunc(devAuthHandler.AcceptInvitation)))

	mux.Handle("POST /api/v1/dev/login", devLogin)
	mux.Handle("POST /api/v1/dev/accept-invitation", devAcceptInvitation)

	// GitHub OAuth routes (srv-idczk) - only register if GitHub OAuth is configured
	if devOAuthHandler != nil {
		mux.Handle("GET /auth/github", rateLimitPublic(http.HandlerFunc(devOAuthHandler.GitHubLogin)))
		mux.Handle("GET /auth/github/callback", rateLimitPublic(http.HandlerFunc(devOAuthHandler.GitHubCallback)))
	}

	// Developer API key management endpoints with DevCookieAuth middleware
	devCookieAuth := middleware.DevCookieAuth(developerJWTKey) // Use derived developer JWT key (srv-yuyg9)
	devLogout := devCookieAuth(http.HandlerFunc(devAuthHandler.Logout))
	devListKeys := devCookieAuth(http.HandlerFunc(devAPIKeyHandler.ListAPIKeys))
	devCreateKey := devCookieAuth(http.HandlerFunc(devAPIKeyHandler.CreateAPIKey))
	devRevokeKey := devCookieAuth(http.HandlerFunc(devAPIKeyHandler.RevokeAPIKey))
	devGetUsage := devCookieAuth(http.HandlerFunc(devAPIKeyHandler.GetAPIKeyUsage))

	mux.Handle("POST /api/v1/dev/logout", devLogout)
	mux.Handle("GET /api/v1/dev/api-keys", devListKeys)
	mux.Handle("POST /api/v1/dev/api-keys", devCreateKey)
	mux.Handle("DELETE /api/v1/dev/api-keys/{id}", devRevokeKey)
	mux.Handle("GET /api/v1/dev/api-keys/{id}/usage", devGetUsage)

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
	federationSync := apiKeyAuth(usageTracking(rateLimitFederation(middleware.Idempotency(middleware.FederationRequestSize()(http.HandlerFunc(federationHandler.Sync))))))
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

	// Admin HTML routes with CSRF protection and cookie auth
	mux.Handle("/admin/dashboard", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeDashboard))))
	mux.Handle("/admin/events", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeEventsList))))
	mux.Handle("/admin/events/{id}", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeEventEdit))))
	mux.Handle("/admin/duplicates", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeDuplicates))))
	mux.Handle("/admin/review-queue", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeReviewQueue))))
	mux.Handle("/admin/api-keys", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeAPIKeys))))
	mux.Handle("/admin/federation", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeFederation))))
	mux.Handle("/admin/users", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeUsersList))))
	mux.Handle("/admin/users/{id}/activity", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeUserActivity))))
	mux.Handle("/admin/places", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServePlacesList))))
	mux.Handle("/admin/organizations", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeOrganizationsList))))
	mux.Handle("/admin/developers", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeDevelopersList))))
	mux.Handle("/admin/scraper", csrfMiddleware(adminCookieAuth(http.HandlerFunc(adminHTMLHandler.ServeScraperSources))))

	// SSE stream of River job events for the scraper admin page (srv-lahwo).
	// Uses cookie auth (no CSRF needed — GET, no state mutation, same-origin EventSource).
	adminEventsSSEHandler := handlers.NewAdminEventsSSEHandler(broker, cfg.Environment, logger.With().Str("component", "sse").Logger())
	adminScraperEvents := adminCookieAuth(http.HandlerFunc(adminEventsSSEHandler.ServeHTTP))
	mux.Handle("GET /api/v1/admin/scraper/events", adminScraperEvents)

	// Redirect /admin and /admin/ to dashboard
	adminRoot := csrfMiddleware(adminCookieAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/dashboard", http.StatusFound)
	})))
	mux.Handle("/admin", adminRoot)
	mux.Handle("/admin/", adminRoot)

	// Login page doesn't need CSRF (no auth required, no state-changing action on GET)
	mux.Handle("/admin/login", http.HandlerFunc(adminAuthHandler.LoginPage))

	// Public invitation acceptance page (NO AUTH)
	mux.Handle("/accept-invitation", http.HandlerFunc(adminHTMLHandler.ServeAcceptInvitation))

	// Serve admin static files (CSS, JS, images)
	// The embed.FS contains admin/static/..., so we need to create a sub-filesystem
	adminStaticSubFS, err := fs.Sub(web.AdminStaticFiles, "admin/static")
	if err != nil {
		logger.Error().Err(err).Msg("failed to create admin static sub-filesystem")
	} else {
		adminStaticFS := http.FileServer(http.FS(adminStaticSubFS))
		mux.Handle("/admin/static/", http.StripPrefix("/admin/static/", adminStaticFS))
	}

	// Developer HTML routes (srv-7m0cf)
	// Public pages (no auth required)
	mux.Handle("/dev/login", http.HandlerFunc(devHTMLHandler.ServeLogin))
	mux.Handle("/dev/accept-invitation", http.HandlerFunc(devHTMLHandler.ServeAcceptInvitation))

	// Authenticated pages with CSRF protection (srv-w2isc)
	mux.Handle("/dev/dashboard", csrfMiddleware(devCookieAuth(http.HandlerFunc(devHTMLHandler.ServeDashboard))))
	mux.Handle("/dev/api-keys", csrfMiddleware(devCookieAuth(http.HandlerFunc(devHTMLHandler.ServeAPIKeys))))

	// Redirect /dev and /dev/ to dashboard
	devRoot := csrfMiddleware(devCookieAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dev/dashboard", http.StatusFound)
	})))
	mux.Handle("/dev", devRoot)
	mux.Handle("/dev/", devRoot)

	// Serve developer static files (JS only - reuses admin CSS/images via CDN)
	devStaticSubFS, err := fs.Sub(web.DevStaticFiles, "dev/static")
	if err != nil {
		logger.Error().Err(err).Msg("failed to create dev static sub-filesystem")
	} else {
		devStaticFS := http.FileServer(http.FS(devStaticSubFS))
		mux.Handle("/dev/static/", http.StripPrefix("/dev/static/", devStaticFS))
	}

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
		Handler:       handler,
		RiverClient:   riverClient,
		UsageRecorder: usageRecorder,
		Broker:        broker,
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

// loadTemplates loads HTML templates from the specified directory.
// The commitHash parameter should come from ldflags (the authoritative source baked into the binary at build time).
// Falls back to BUILD_COMMIT env var, then "dev" if neither is available.
// The templatesSubPath should be a relative path like "web/admin/templates" or "web/dev/templates".
// Optional additionalFiles can be provided to load extra template files (e.g., shared partials).
func loadTemplates(commitHash string, templatesSubPath string, additionalFiles ...string) (*template.Template, error) {
	// Use ldflags value as the authoritative commit hash source.
	// Fall back to BUILD_COMMIT env var for backward compatibility,
	// then "dev" for local development.
	gitCommit := commitHash
	if gitCommit == "" || gitCommit == "unknown" {
		gitCommit = os.Getenv("BUILD_COMMIT")
	}
	if gitCommit == "" {
		gitCommit = "dev"
	} else if len(gitCommit) > 7 {
		gitCommit = gitCommit[:7] // Use short hash
	}

	// Create template with custom functions for cache-busting
	funcMap := template.FuncMap{
		"assetVersion": func() string {
			return gitCommit
		},
		"gitCommit": func() string {
			return gitCommit
		},
	}

	// Try common locations for the templates directory
	systemInstallPath := filepath.Join("/usr/local/share/sel", templatesSubPath)
	candidates := []string{
		templatesSubPath,                         // From project root
		filepath.Join("..", templatesSubPath),    // One level up
		filepath.Join("../..", templatesSubPath), // Two levels up
		filepath.Join("/app", templatesSubPath),  // Container deployment
		systemInstallPath,                        // System-wide installation
	}

	for _, candidate := range candidates {
		if absPath, err := filepath.Abs(candidate); err == nil {
			if info, err := os.Stat(absPath); err == nil && info.IsDir() {
				// Found templates directory, parse all .html files
				pattern := filepath.Join(absPath, "*.html")
				tmpl := template.New("").Funcs(funcMap)

				// Derive the base directory by stripping the templatesSubPath suffix.
				// For example, if absPath is "/app/web/dev/templates" and
				// templatesSubPath is "web/dev/templates", baseDir is "/app".
				absSubPath, _ := filepath.Abs(templatesSubPath)
				baseDir := filepath.Dir(absPath)
				if rel, err := filepath.Rel(absSubPath, absPath); err == nil && rel == "." {
					// candidate matched templatesSubPath directly — base is cwd
					baseDir, _ = os.Getwd()
				} else {
					// Strip templatesSubPath suffix from absPath to get the base
					suffix := string(filepath.Separator) + filepath.Clean(templatesSubPath)
					if strings.HasSuffix(absPath, suffix) {
						baseDir = strings.TrimSuffix(absPath, suffix)
					}
				}

				// Load any additional files first (e.g., shared partials)
				for _, additionalFile := range additionalFiles {
					additionalPath := filepath.Join(baseDir, additionalFile)
					if _, err := os.Stat(additionalPath); err == nil {
						tmpl, _ = tmpl.ParseFiles(additionalPath)
					}
				}

				if tmpl, err := tmpl.ParseGlob(pattern); err == nil {
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
				templatesPath := filepath.Join(dir, templatesSubPath)
				if info, err := os.Stat(templatesPath); err == nil && info.IsDir() {
					pattern := filepath.Join(templatesPath, "*.html")
					tmpl := template.New("").Funcs(funcMap)

					// Load any additional files first (e.g., shared partials)
					for _, additionalFile := range additionalFiles {
						additionalPath := filepath.Join(dir, additionalFile)
						if _, err := os.Stat(additionalPath); err == nil {
							tmpl, _ = tmpl.ParseFiles(additionalPath)
						}
					}

					if tmpl, err := tmpl.ParseGlob(pattern); err == nil {
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

// loadAdminTemplates loads HTML templates for the admin UI.
// Thin wrapper around loadTemplates for backward compatibility.
func loadAdminTemplates(commitHash string) (*template.Template, error) {
	return loadTemplates(commitHash, "web/admin/templates")
}

// loadDevTemplates loads HTML templates for the developer portal UI.
// Thin wrapper around loadTemplates that also loads the shared _head_meta.html partial.
func loadDevTemplates(commitHash string) (*template.Template, error) {
	return loadTemplates(commitHash, "web/dev/templates", "web/admin/templates/_head_meta.html")
}

// findRepoRoot locates the repository root by looking for go.mod
func findRepoRoot() string {
	// Try working directory first
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < maxRouterRetries; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// Fallback: return current directory
	return "."
}
