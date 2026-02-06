package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/mcp"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	// shutdownTimeout is the maximum time to wait for graceful shutdown.
	shutdownTimeout = 30 * time.Second
)

func main() {
	// Exit with non-zero status on error
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run contains the main application logic.
// It's separated from main() to allow deferred cleanup functions to run before os.Exit.
func run() error {
	// Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Setup logging
	// CRITICAL: For stdio transport, NEVER log to stdout - it corrupts MCP protocol messages
	// Always log to stderr for all transports
	logger := setupLogging(cfg.Base.Logging)
	log.Logger = logger

	log.Info().
		Str("transport", string(cfg.Transport.Type)).
		Str("mcp_name", cfg.MCP.Name).
		Str("mcp_version", cfg.MCP.Version).
		Str("environment", cfg.Base.Environment).
		Msg("Starting MCP server")

	// Connect to database
	ctx := context.Background()
	pool, err := connectDatabase(ctx, cfg.Base.Database)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer pool.Close()

	log.Info().Msg("Database connection established")

	// Initialize repository
	repo, err := postgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("repository initialization failed: %w", err)
	}

	// Initialize domain services
	eventsService := events.NewService(repo.Events())
	ingestService := events.NewIngestService(repo.Events(), cfg.Base.Server.BaseURL)
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())

	log.Info().Msg("Domain services initialized")

	// Create MCP server
	mcpServer := mcp.NewServer(
		mcp.Config{
			Name:      cfg.MCP.Name,
			Version:   cfg.MCP.Version,
			Transport: string(cfg.Transport.Type),
		},
		eventsService,
		ingestService,
		placesService,
		orgService,
		cfg.Base.Server.BaseURL,
	)

	log.Info().
		Str("transport", string(cfg.Transport.Type)).
		Msg("MCP server created, starting transport")

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle OS signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := mcp.Serve(ctx, mcpServer.MCPServer(), cfg.Transport, repo.Auth().APIKeys(), cfg.Base.RateLimit); err != nil {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for shutdown signal or server error
	select {
	case sig := <-sigCh:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}

	// Initiate graceful shutdown
	log.Info().Msg("Initiating graceful shutdown")
	cancel() // Cancel context to stop server

	// Wait for server to finish with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	// Shutdown MCP server
	if err := mcpServer.Shutdown(shutdownCtx); err != nil {
		log.Warn().Err(err).Msg("MCP server shutdown error")
	}

	// Wait for server goroutine to exit or timeout
	select {
	case <-shutdownCtx.Done():
		log.Warn().Msg("Shutdown timeout exceeded")
		return fmt.Errorf("shutdown timeout exceeded")
	case err := <-serverErr:
		if err != nil {
			log.Warn().Err(err).Msg("Server error during shutdown")
		}
	}

	log.Info().Msg("Shutdown complete")
	return nil
}

// setupLogging initializes the logger based on configuration.
// IMPORTANT: All logs go to stderr to avoid corrupting MCP protocol on stdout.
func setupLogging(cfg config.LoggingConfig) zerolog.Logger {
	// Parse log level
	level := zerolog.InfoLevel
	switch cfg.Level {
	case "debug":
		level = zerolog.DebugLevel
	case "info":
		level = zerolog.InfoLevel
	case "warn", "warning":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	}

	// Set global log level
	zerolog.SetGlobalLevel(level)

	// Configure output format
	// CRITICAL: Always output to stderr (not stdout) to avoid corrupting MCP stdio protocol
	var logger zerolog.Logger
	if cfg.Format == "console" {
		// Human-readable format with colors
		logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Logger()
	} else {
		// JSON format (default)
		logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}

	return logger
}

// connectDatabase establishes a connection to the PostgreSQL database.
func connectDatabase(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	// Parse pool config
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Set connection pool limits
	poolConfig.MaxConns = int32(cfg.MaxConnections)
	poolConfig.MinConns = int32(cfg.MaxIdle)

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Setup slog logger for database operations (sent to stderr)
	slogLogger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(slogLogger)

	return pool, nil
}
