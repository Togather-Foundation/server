package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/mcp"
)

// MCPConfig holds MCP-specific configuration.
// It extends the base application config with MCP server settings.
type MCPConfig struct {
	// Base application configuration (database, auth, etc.)
	Base config.Config

	// MCP server configuration
	MCP MCPServerConfig

	// Transport configuration
	Transport *mcp.TransportConfig
}

// MCPServerConfig holds MCP server metadata.
type MCPServerConfig struct {
	Name    string
	Version string
}

// LoadConfig loads configuration from environment variables.
// MCP-specific environment variables:
//   - MCP_SERVER_NAME: Server name for MCP identification (default: "Togather SEL MCP Server")
//   - MCP_SERVER_VERSION: Server version (default: "1.0.0")
//   - MCP_TRANSPORT: Transport type - "stdio", "sse", or "http" (default: "stdio")
//   - PORT: HTTP port for SSE/HTTP transports (default: 8080)
//   - HOST: Bind address for SSE/HTTP transports (default: "0.0.0.0")
//
// All standard application environment variables from config.Load() are also supported.
func LoadConfig() (*MCPConfig, error) {
	// Load base application config
	baseConfig, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load base config: %w", err)
	}

	// Load transport config
	transportConfig, err := mcp.LoadTransportConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load transport config: %w", err)
	}

	// MCP server metadata
	mcpConfig := MCPServerConfig{
		Name:    getEnv("MCP_SERVER_NAME", "Togather SEL MCP Server"),
		Version: getEnv("MCP_SERVER_VERSION", "1.0.0"),
	}

	return &MCPConfig{
		Base:      baseConfig,
		MCP:       mcpConfig,
		Transport: transportConfig,
	}, nil
}

// getEnv returns the value of an environment variable or a fallback value if not set.
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// getEnvInt returns an environment variable as an integer or a fallback value.
func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// getEnvDuration returns an environment variable as a duration or a fallback value.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
