// Package mcp provides MCP (Model Context Protocol) server configuration and transport support.
package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"
)

// TransportType represents the available MCP transport protocols.
type TransportType string

const (
	// TransportStdio uses standard input/output for MCP communication.
	// Best for: Claude Desktop, CLI tools, local development.
	TransportStdio TransportType = "stdio"

	// TransportSSE uses Server-Sent Events for MCP communication.
	// Best for: Web applications, browser-based clients.
	TransportSSE TransportType = "sse"

	// TransportHTTP uses Streamable HTTP for MCP communication.
	// Best for: Production deployments, scalable web services.
	TransportHTTP TransportType = "http"
)

const (
	// DefaultTransport is used when MCP_TRANSPORT is not set.
	DefaultTransport = TransportStdio

	// DefaultPort is used when PORT is not set for HTTP/SSE transports.
	DefaultPort = 8080

	// GracefulShutdownTimeout is the maximum time to wait for graceful shutdown.
	// Allows in-flight requests to complete before forcing shutdown.
	GracefulShutdownTimeout = 30 * time.Second

	// ForceShutdownTimeout is the additional time to wait before giving up.
	// After this timeout, the server will forcefully close connections.
	ForceShutdownTimeout = 5 * time.Second
)

// TransportConfig holds configuration for MCP transport selection.
type TransportConfig struct {
	// Type specifies which transport to use (stdio, sse, http).
	Type TransportType

	// Port is the HTTP port for SSE and HTTP transports (ignored for stdio).
	Port int

	// Host is the bind address for SSE and HTTP transports (default: "0.0.0.0").
	Host string
}

// LoadTransportConfig reads transport configuration from environment variables.
// Environment variables:
//   - MCP_TRANSPORT: "stdio", "sse", or "http" (default: "stdio")
//   - PORT: HTTP port for SSE/HTTP transports (default: 8080)
//   - HOST: Bind address for SSE/HTTP transports (default: "0.0.0.0")
func LoadTransportConfig() (*TransportConfig, error) {
	cfg := &TransportConfig{
		Type: DefaultTransport,
		Port: DefaultPort,
		Host: "0.0.0.0",
	}

	// Parse MCP_TRANSPORT
	if transportEnv := os.Getenv("MCP_TRANSPORT"); transportEnv != "" {
		transport := TransportType(transportEnv)
		switch transport {
		case TransportStdio, TransportSSE, TransportHTTP:
			cfg.Type = transport
		default:
			return nil, fmt.Errorf("invalid MCP_TRANSPORT value: %s (must be stdio, sse, or http)", transportEnv)
		}
	}

	// Parse PORT
	if portEnv := os.Getenv("PORT"); portEnv != "" {
		port, err := strconv.Atoi(portEnv)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT value: %s (must be a number)", portEnv)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid PORT value: %d (must be between 1 and 65535)", port)
		}
		cfg.Port = port
	}

	// Parse HOST
	if hostEnv := os.Getenv("HOST"); hostEnv != "" {
		cfg.Host = hostEnv
	}

	return cfg, nil
}

// ServeStdio starts the MCP server using stdio transport.
// This is the default transport, suitable for Claude Desktop and CLI tools.
// The server reads requests from stdin and writes responses to stdout.
func ServeStdio(ctx context.Context, mcpServer *server.MCPServer, authStore auth.APIKeyStore, rateLimitCfg config.RateLimitConfig) error {
	log.Info().Msg("Starting MCP server with stdio transport")

	// Run ServeStdio in goroutine to allow context cancellation
	errCh := make(chan error, 1)
	go func() {
		if err := server.ServeStdio(mcpServer); err != nil {
			errCh <- fmt.Errorf("stdio server error: %w", err)
		}
		close(errCh)
	}()

	log.Info().Msg("Stdio server started")

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Info().Msg("Context cancelled, stdio server stopping")
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// ServeSSE starts the MCP server using Server-Sent Events transport.
// This transport is suitable for web applications and browser-based clients.
// The server listens on the configured host and port.
func ServeSSE(ctx context.Context, mcpServer *server.MCPServer, cfg *TransportConfig, authStore auth.APIKeyStore, rateLimitCfg config.RateLimitConfig) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Info().
		Str("transport", "sse").
		Str("addr", addr).
		Msg("Starting MCP server with SSE transport")

	sseServer := server.NewSSEServer(mcpServer)
	wrapped, err := wrapMCPHandler(sseServer, authStore, rateLimitCfg)
	if err != nil {
		return fmt.Errorf("failed to wrap SSE handler: %w", err)
	}

	// Create HTTP server with SSE handler
	httpServer := &http.Server{
		Addr:    addr,
		Handler: wrapped,
	}

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("SSE server error: %w", err)
		}
		close(errCh)
	}()

	log.Info().Str("addr", addr).Msg("SSE server listening")

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Info().Msg("Shutting down SSE server")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), GracefulShutdownTimeout)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("error during graceful shutdown of SSE server")
			return fmt.Errorf("SSE server shutdown error: %w", err)
		}
		log.Info().Msg("SSE server shutdown complete")
		return nil
	case err := <-errCh:
		return err
	}
}

// ServeHTTP starts the MCP server using Streamable HTTP transport.
// This is the production-grade transport, suitable for scalable web deployments.
// The server listens on the configured host and port.
func ServeHTTP(ctx context.Context, mcpServer *server.MCPServer, cfg *TransportConfig, authStore auth.APIKeyStore, rateLimitCfg config.RateLimitConfig) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Info().
		Str("transport", "http").
		Str("addr", addr).
		Msg("Starting MCP server with Streamable HTTP transport")

	httpTransport := server.NewStreamableHTTPServer(mcpServer)
	wrapped, err := wrapMCPHandler(httpTransport, authStore, rateLimitCfg)
	if err != nil {
		return fmt.Errorf("failed to wrap HTTP handler: %w", err)
	}

	// Create HTTP server with streamable HTTP handler
	httpServer := &http.Server{
		Addr:    addr,
		Handler: wrapped,
	}

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("HTTP server error: %w", err)
		}
		close(errCh)
	}()

	log.Info().Str("addr", addr).Msg("Streamable HTTP server listening")

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Info().Msg("Shutting down HTTP server")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), GracefulShutdownTimeout)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("error during graceful shutdown of HTTP server")
			return fmt.Errorf("HTTP server shutdown error: %w", err)
		}
		log.Info().Msg("HTTP server shutdown complete")
		return nil
	case err := <-errCh:
		return err
	}
}

// Serve starts the MCP server with the configured transport.
// This is the main entry point for serving MCP requests.
func Serve(ctx context.Context, mcpServer *server.MCPServer, cfg *TransportConfig, authStore auth.APIKeyStore, rateLimitCfg config.RateLimitConfig) error {
	switch cfg.Type {
	case TransportStdio:
		return ServeStdio(ctx, mcpServer, authStore, rateLimitCfg)
	case TransportSSE:
		return ServeSSE(ctx, mcpServer, cfg, authStore, rateLimitCfg)
	case TransportHTTP:
		return ServeHTTP(ctx, mcpServer, cfg, authStore, rateLimitCfg)
	default:
		return fmt.Errorf("unsupported transport type: %s", cfg.Type)
	}
}

func wrapMCPHandler(handler http.Handler, authStore auth.APIKeyStore, rateLimitCfg config.RateLimitConfig) (http.Handler, error) {
	if handler == nil {
		return nil, fmt.Errorf("handler cannot be nil")
	}

	wrapped := handler
	if authStore != nil {
		wrapped = middleware.AgentAuth(authStore)(wrapped)
	}

	wrapped = middleware.WithRateLimitTierHandler(middleware.TierAgent)(wrapped)
	wrapped = middleware.RateLimit(rateLimitCfg)(wrapped)
	return wrapped, nil
}

// WrapHandler exposes the MCP middleware wrapper for embedding in existing routers.
// Returns an error if the handler is nil.
func WrapHandler(handler http.Handler, authStore auth.APIKeyStore, rateLimitCfg config.RateLimitConfig) (http.Handler, error) {
	return wrapMCPHandler(handler, authStore, rateLimitCfg)
}

// NewStreamableHTTPHandler creates a streamable HTTP MCP handler for embedding.
func NewStreamableHTTPHandler(mcpServer *server.MCPServer) http.Handler {
	return server.NewStreamableHTTPServer(mcpServer)
}
