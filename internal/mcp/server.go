package mcp

import (
	"context"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/mcp/resources"
	"github.com/Togather-Foundation/server/internal/mcp/tools"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Server wraps the MCP server with Togather domain services.
// It provides access to events, places, and organizations through
// MCP tools, resources, and prompts.
type Server struct {
	mcp           *mcpserver.MCPServer
	eventsService *events.Service
	ingestService *events.IngestService
	placesService *places.Service
	orgService    *organizations.Service
	baseURL       string
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
	ingestService *events.IngestService,
	placesService *places.Service,
	orgService *organizations.Service,
	baseURL string,
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
		ingestService: ingestService,
		placesService: placesService,
		orgService:    orgService,
		baseURL:       baseURL,
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
	// Register event tools (server-gau4, server-wako)
	eventTools := tools.NewEventTools(s.eventsService, s.ingestService, s.baseURL)

	// list_events tool - query events with filters and pagination
	s.mcp.AddTool(eventTools.ListEventsTool(), eventTools.ListEventsHandler)

	// get_event tool - retrieve a specific event by ULID
	s.mcp.AddTool(eventTools.GetEventTool(), eventTools.GetEventHandler)

	// create_event tool - create new events
	s.mcp.AddTool(eventTools.CreateEventTool(), eventTools.CreateEventHandler)

	// Register place tools (server-9185, server-g8q5)
	placeTools := tools.NewPlaceTools(s.placesService, s.baseURL)

	// list_places tool - query places with filters and pagination
	s.mcp.AddTool(placeTools.ListPlacesTool(), placeTools.ListPlacesHandler)

	// get_place tool - retrieve a specific place by ULID
	s.mcp.AddTool(placeTools.GetPlaceTool(), placeTools.GetPlaceHandler)

	// create_place tool - create new places
	s.mcp.AddTool(placeTools.CreatePlaceTool(), placeTools.CreatePlaceHandler)

	// Register organization tools (server-slhh, server-5yr5)
	organizationTools := tools.NewOrganizationTools(s.orgService, s.baseURL)

	// list_organizations tool - query organizations with filters and pagination
	s.mcp.AddTool(organizationTools.ListOrganizationsTool(), organizationTools.ListOrganizationsHandler)

	// get_organization tool - retrieve a specific organization by ULID
	s.mcp.AddTool(organizationTools.GetOrganizationTool(), organizationTools.GetOrganizationHandler)

	// create_organization tool - create new organizations
	s.mcp.AddTool(organizationTools.CreateOrganizationTool(), organizationTools.CreateOrganizationHandler)

	// Register search tools (server-rupi)
	searchTools := tools.NewSearchTools(s.eventsService, s.placesService, s.orgService, s.baseURL)

	// search tool - query events, places, and organizations
	s.mcp.AddTool(searchTools.SearchTool(), searchTools.SearchHandler)
}

// registerResources registers all MCP resources for events, places, and organizations.
// Resources are data sources that can be read and subscribed to.
func (s *Server) registerResources() {
	contextResources := resources.NewContextResources()
	contextPath := resources.ContextPath("sel/v0.1.jsonld")

	contextDefinitions := []struct {
		Name        string
		URI         string
		Description string
	}{
		{
			Name:        "SEL Event Context",
			URI:         resources.ContextURI("sel-event"),
			Description: "JSON-LD context for SEL event resources",
		},
		{
			Name:        "SEL Place Context",
			URI:         resources.ContextURI("sel-place"),
			Description: "JSON-LD context for SEL place resources",
		},
		{
			Name:        "SEL Organization Context",
			URI:         resources.ContextURI("sel-organization"),
			Description: "JSON-LD context for SEL organization resources",
		},
	}

	for _, contextDef := range contextDefinitions {
		resource := contextResources.Resource(contextDef.URI, contextDef.Name, contextDef.Description)
		s.mcp.AddResource(resource, contextResources.ReadHandler(contextPath, contextDef.URI))
	}
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
