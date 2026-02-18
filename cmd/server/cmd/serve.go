package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/metrics"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

var (
	// Server flags (override config/env)
	serverHost string
	serverPort int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the SEL HTTP server",
	Long: `Start the SEL HTTP server and begin accepting API requests.

The server will:
- Load configuration from environment variables (or --config file if provided)
- Bootstrap admin user if ADMIN_* env vars are set
- Start HTTP server with JSON-LD API endpoints
- Handle graceful shutdown on SIGINT/SIGTERM

Examples:
  # Start with default configuration (from env vars)
  server serve

  # Start on a specific host and port
  server serve --host 127.0.0.1 --port 9090

  # Start with debug logging
  server serve --log-level debug

  # Start with custom config file
  server serve --config /etc/togather/config.yaml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer()
	},
}

func init() {
	// Server-specific flags
	serveCmd.Flags().StringVar(&serverHost, "host", "", "server host address (default: 0.0.0.0)")
	serveCmd.Flags().IntVar(&serverPort, "port", 0, "server port (default: 8080)")
}

func runServer() error {
	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	// Override config with flags if provided
	if serverHost != "" {
		cfg.Server.Host = serverHost
	}
	if serverPort != 0 {
		cfg.Server.Port = serverPort
	}

	// Create logger
	logger := config.NewLogger(cfg.Logging)
	logger.Info().Msg("starting SEL server")

	// Initialize Prometheus metrics with version information
	activeSlot := os.Getenv("ACTIVE_SLOT")
	if activeSlot == "" {
		activeSlot = "unknown"
	}
	metrics.Init(Version, GitCommit, BuildDate, activeSlot)
	logger.Info().Str("version", Version).Str("active_slot", activeSlot).Msg("metrics initialized")

	// Bootstrap admin user if configured
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := bootstrapAdminUser(ctx, cfg, logger); err != nil {
		logger.Error().Err(err).Msg("admin bootstrap failed")
	}
	cancel()

	// Create database connection pool
	poolCtx, poolCancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := pgxpool.New(poolCtx, cfg.Database.URL)
	poolCancel()
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer pool.Close()

	// Start database metrics collector (collect every 15 seconds)
	dbCollector := metrics.NewDBCollector(pool)
	collectorCtx, collectorCancel := context.WithCancel(context.Background())
	go dbCollector.Start(collectorCtx, 15*time.Second)
	defer collectorCancel()
	defer dbCollector.Stop()
	logger.Info().Msg("database metrics collector started")

	// Create router with River client
	routerWithClient := api.NewRouter(cfg, logger, pool, Version, GitCommit, BuildDate)

	// Start River background job workers
	// Workers process batch ingestion, deduplication, enrichment, and reconciliation jobs
	riverCtx, riverCancel := context.WithCancel(context.Background())
	defer riverCancel()

	if routerWithClient.RiverClient != nil {
		if err := routerWithClient.RiverClient.Start(riverCtx); err != nil {
			return fmt.Errorf("river workers failed to start: %w", err)
		}
		logger.Info().Msg("river background job workers started")
		defer func() {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer stopCancel()
			if err := routerWithClient.RiverClient.Stop(stopCtx); err != nil {
				logger.Error().Err(err).Msg("river workers shutdown error")
			} else {
				logger.Info().Msg("river workers stopped")
			}
		}()
	} else {
		logger.Warn().Msg("river client not initialized, batch processing will not work")
	}

	// Start API key usage recorder (srv-8r58k)
	// Records developer API key usage metrics (request counts, error counts) and periodically flushes to database
	if routerWithClient.UsageRecorder != nil {
		routerWithClient.UsageRecorder.Start()
		logger.Info().Msg("API key usage recorder started")
		defer func() {
			if err := routerWithClient.UsageRecorder.Close(); err != nil {
				logger.Error().Err(err).Msg("usage recorder shutdown error")
			} else {
				logger.Info().Msg("usage recorder stopped")
			}
		}()
	}

	// Create HTTP server
	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           routerWithClient.Handler,
		ReadTimeout:       10 * time.Second, // Total time to read request
		WriteTimeout:      30 * time.Second, // Total time to write response
		ReadHeaderTimeout: 5 * time.Second,  // Time to read headers
		MaxHeaderBytes:    1 << 20,          // 1 MB max header size
	}

	// Start server in background
	go func() {
		logger.Info().Str("addr", server.Addr).Msg("listening")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("http server error")
		}
	}()

	// Wait for shutdown signal
	return gracefulShutdown(server, logger)
}

func loadConfig() (config.Config, error) {
	// TODO: Support --config file flag in the future
	// For now, just use environment variables via config.Load()
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, err
	}

	// Override logging from flags if provided
	if logLevel != "" {
		cfg.Logging.Level = logLevel
	}
	if logFormat != "" {
		cfg.Logging.Format = logFormat
	}

	return cfg, nil
}

func bootstrapAdminUser(ctx context.Context, cfg config.Config, logger zerolog.Logger) error {
	bootstrap := cfg.AdminBootstrap
	if bootstrap.Username == "" || bootstrap.Password == "" || bootstrap.Email == "" {
		logger.Warn().Msg("admin bootstrap env vars not fully set; skipping")
		return nil
	}

	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	const checkQuery = `SELECT id FROM users WHERE email = $1 OR username = $2 LIMIT 1`
	row := pool.QueryRow(ctx, checkQuery, bootstrap.Email, bootstrap.Username)
	var existingID string
	if err := row.Scan(&existingID); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check admin user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(bootstrap.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	const insertQuery = `
INSERT INTO users (id, username, email, password_hash, role, is_active, created_at)
VALUES (gen_random_uuid(), $1, $2, $3, 'admin', true, now())`
	if _, err := pool.Exec(ctx, insertQuery, bootstrap.Username, bootstrap.Email, string(hash)); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	// Log admin creation - redact email in production to avoid PII leaks
	if cfg.Environment == "production" {
		logger.Info().Str("username", bootstrap.Username).Msg("bootstrapped admin user")
	} else {
		logger.Info().Str("email", bootstrap.Email).Str("username", bootstrap.Username).Msg("bootstrapped admin user")
	}
	return nil
}

func gracefulShutdown(server *http.Server, logger zerolog.Logger) error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop
	logger.Info().Msg("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("shutdown error")
		return err
	}

	logger.Info().Msg("server stopped")
	return nil
}
