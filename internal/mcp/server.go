package mcp

import (
	"context"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Server wraps the MCP server with Togather domain services.
// It provides access to events, places, and organizations through
// MCP tools, resources, and prompts.
type Server struct {
	mcp           *mcpserver.MCPServer
	eventsService *events.Service
	placesService *places.Service
	orgService    *organizations.Service
}

// Config holds configuration for the MCP server.
type Config struct {
	Name    string
	Version string
}

// NewServer creates a new MCP server with the given services.
// It initializes the server with all capabilities enabled:
// - Tools: Execute operations on events, places, and organizations
// - Resources: Subscribe to and read data from the Togather system
// - Prompts: Get contextual prompts for working with SEL data
//
// Example usage:
//
//	srv := mcp.NewServer(mcp.Config{
//	    Name:    "Togather MCP Server",
//	    Version: "1.0.0",
//	}, eventsService, placesService, orgService)
func NewServer(
	cfg Config,
	eventsService *events.Service,
	placesService *places.Service,
	orgService *organizations.Service,
) *Server {
	// Initialize MCP server with full capabilities
	mcpServer := mcpserver.NewMCPServer(
		cfg.Name,
		cfg.Version,
		mcpserver.WithToolCapabilities(true), // Enable tools with listChanged notifications
		mcpserver.WithResourceCapabilities(true, true), // Enable resources with subscribe + listChanged
		mcpserver.WithPromptCapabilities(true),         // Enable prompts with listChanged notifications
		mcpserver.WithRecovery(),                       // Add panic recovery middleware
		mcpserver.WithInstructions("MCP server for Togather Shared Events Library (SEL) - query and interact with events, places, and organizations"),
	)

	srv := &Server{
		mcp:           mcpServer,
		eventsService: eventsService,
		placesService: placesService,
		orgService:    orgService,
	}

	// Register tools, resources, and prompts
	srv.registerTools()
	srv.registerResources()
	srv.registerPrompts()

	return srv
}

// MCPServer returns the underlying MCP server for use with transports.
// Use this to pass the server to ServeStdio, ServeSSE, or other transport handlers.
func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcp
}

// registerTools registers all MCP tools for events, places, and organizations.
// Tools are operations that can be executed by the MCP client.
func (s *Server) registerTools() {
	// Tool registration will be implemented in subsequent beads:
	// - server-ncc3: Event query tools (list, get, search)
	// - server-ppp3: Place query tools (list, get, search)
	// - server-ooo3: Organization query tools (list, get, search)
}

// registerResources registers all MCP resources for events, places, and organizations.
// Resources are data sources that can be read and subscribed to.
func (s *Server) registerResources() {
	// Resource registration will be implemented in subsequent beads:
	// - server-rrr3: Event resources (individual events, feeds)
	// - server-sss3: Place resources (individual places)
	// - server-ttt3: Organization resources (individual organizations)
}

// registerPrompts registers all MCP prompts for SEL workflows.
// Prompts provide contextual assistance for working with SEL data.
func (s *Server) registerPrompts() {
	// Prompt registration will be implemented in subsequent beads:
	// - server-uuu3: SEL workflow prompts (event creation, validation, federation)
}

// Shutdown gracefully shuts down the MCP server and cleans up resources.
// This should be called when the server is no longer needed.
func (s *Server) Shutdown(ctx context.Context) error {
	// Currently no cleanup needed, but this provides a hook for future
	// resource cleanup (connections, subscriptions, etc.)
	return nil
}
