package mcp

import (
	"context"

	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/geocoding"
	"github.com/Togather-Foundation/server/internal/mcp/prompts"
	"github.com/Togather-Foundation/server/internal/mcp/resources"
	"github.com/Togather-Foundation/server/internal/mcp/tools"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Server wraps the MCP server with Togather domain services.
//
// Server provides access to events, places, and organizations through
// MCP tools, resources, and prompts. It integrates with Togather's domain
// layer and exposes SEL data through the Model Context Protocol.
//
// The server is created with NewServer and exposes three capability types:
//
//   - Tools: Operations like list_events, create_place, search
//   - Resources: JSON-LD contexts, OpenAPI schemas, server metadata
//   - Prompts: Contextual assistance for SEL workflows
//
// Use MCPServer() to obtain the underlying MCP server for transport handlers.
// The server supports graceful shutdown via Shutdown(ctx).
//
// Example:
//
//	srv := mcp.NewServer(cfg, eventsService, ingestService, placesService, orgService, baseURL)
//	defer srv.Shutdown(context.Background())
//
//	// Serve with configured transport
//	err := mcp.Serve(ctx, srv.MCPServer(), transportCfg, authStore, rateLimitCfg)
type Server struct {
	mcp              *mcpserver.MCPServer
	eventsService    *events.Service
	ingestService    *events.IngestService
	placesService    *places.Service
	orgService       *organizations.Service
	developerService *developers.Service
	geocodingService *geocoding.GeocodingService
	baseURL          string
	name             string
	version          string
	transport        string
	contextDir       string // Directory containing JSON-LD context files
	openAPIPath      string // Path to OpenAPI specification YAML file
}

// Config holds configuration for the MCP server.
//
// Config specifies the server name, version, and transport type used for
// server identification and resource generation. The Name and Version appear
// in server metadata resources, while Transport is stored for informational
// purposes (actual transport selection happens via LoadTransportConfig).
//
// Example:
//
//	cfg := mcp.Config{
//	    Name:        "Togather MCP Server",
//	    Version:     "1.0.0",
//	    Transport:   "stdio", // or "sse", "http"
//	    ContextDir:  "contexts",
//	    OpenAPIPath: "docs/api/openapi.yaml",
//	}
type Config struct {
	// Name is the human-readable server name (e.g., "Togather MCP Server").
	Name string

	// Version is the semantic version of the server (e.g., "1.0.0").
	Version string

	// Transport indicates the transport type ("stdio", "sse", "http").
	// This is informational; actual transport selection uses LoadTransportConfig.
	Transport string

	// ContextDir is the directory containing JSON-LD context files (default: "contexts").
	ContextDir string

	// OpenAPIPath is the path to the OpenAPI specification YAML file
	// (default: "specs/001-sel-backend/contracts/openapi.yaml").
	OpenAPIPath string
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
	developerService *developers.Service,
	geocodingService *geocoding.GeocodingService,
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
		mcp:              mcpServer,
		eventsService:    eventsService,
		ingestService:    ingestService,
		placesService:    placesService,
		orgService:       orgService,
		developerService: developerService,
		geocodingService: geocodingService,
		baseURL:          baseURL,
		name:             cfg.Name,
		version:          cfg.Version,
		transport:        cfg.Transport,
		contextDir:       cfg.ContextDir,
		openAPIPath:      cfg.OpenAPIPath,
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
	eventTools := tools.NewEventTools(s.eventsService, s.ingestService, s.baseURL).
		WithPlaceResolver(s.placesService).
		WithOrgResolver(s.orgService)

	// events tool - list events with filters OR get a specific event by ULID
	s.mcp.AddTool(eventTools.EventsTool(), eventTools.EventsHandler)

	// add_event tool - create new events
	s.mcp.AddTool(eventTools.AddEventTool(), eventTools.AddEventHandler)

	// Register place tools (server-9185, server-g8q5)
	placeTools := tools.NewPlaceTools(s.placesService, s.baseURL)

	// places tool - list places with filters OR get a specific place by ULID
	s.mcp.AddTool(placeTools.PlacesTool(), placeTools.PlacesHandler)

	// TODO: add_place tool disabled - Place creation removed from repository
	// s.mcp.AddTool(placeTools.AddPlaceTool(), placeTools.AddPlaceHandler)

	// Register organization tools (server-slhh, server-5yr5)
	organizationTools := tools.NewOrganizationTools(s.orgService, s.baseURL)

	// organizations tool - list organizations with filters OR get a specific organization by ULID
	s.mcp.AddTool(organizationTools.OrganizationsTool(), organizationTools.OrganizationsHandler)

	// TODO(srv-d7cnu): add_organization tool disabled - Organization creation broken after rebase
	// s.mcp.AddTool(organizationTools.AddOrganizationTool(), organizationTools.AddOrganizationHandler)

	// Register search tools (server-rupi)
	searchTools := tools.NewSearchTools(s.eventsService, s.placesService, s.orgService, s.baseURL)

	// search tool - query events, places, and organizations
	s.mcp.AddTool(searchTools.SearchTool(), searchTools.SearchHandler)

	// Register developer tools (API key management)
	if s.developerService != nil {
		developerTools := tools.NewDeveloperTools(s.developerService, s.baseURL)

		// api_keys tool - list all keys OR get specific key usage with daily breakdown
		s.mcp.AddTool(developerTools.APIKeysTool(), developerTools.APIKeysHandler)

		// manage_api_key tool - create or revoke API keys
		s.mcp.AddTool(developerTools.ManageAPIKeyTool(), developerTools.ManageAPIKeyHandler)
	}

	// Register geocoding tools (srv-28gtj, srv-4xnt8)
	if s.geocodingService != nil {
		geocodingTools := tools.NewGeocodingTools(s.geocodingService)

		// geocode_address tool - convert addresses to coordinates
		s.mcp.AddTool(geocodingTools.GeocodeAddressTool(), geocodingTools.GeocodeAddressHandler)

		// reverse_geocode tool - convert coordinates to addresses
		s.mcp.AddTool(geocodingTools.ReverseGeocodeTool(), geocodingTools.ReverseGeocodeHandler)
	}
}

// registerResources registers all MCP resources for events, places, and organizations.
// Resources are data sources that can be read and subscribed to.
func (s *Server) registerResources() {
	contextResources := resources.NewContextResources(s.contextDir)
	contextPath := contextResources.ContextPath("sel/v0.1.jsonld")

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

	schemaResources := resources.NewSchemaResources(s.openAPIPath)
	serverInfo := resources.ServerInfo{
		Name:    s.name,
		Version: s.version,
		BaseURL: s.baseURL,
		Capabilities: resources.ServerCapabilities{
			Tools:     true,
			Resources: true,
			Prompts:   true,
		},
		Transport: s.transport,
	}

	s.mcp.AddResource(schemaResources.OpenAPIResource(), schemaResources.OpenAPIReadHandler())
	s.mcp.AddResource(schemaResources.InfoResource(), schemaResources.InfoReadHandler(serverInfo))
}

// registerPrompts registers all MCP prompts for SEL workflows.
// Prompts provide contextual assistance for working with SEL data.
func (s *Server) registerPrompts() {
	promptTemplates := prompts.NewPromptTemplates()

	s.mcp.AddPrompt(promptTemplates.CreateEventFromTextPrompt(), promptTemplates.CreateEventFromTextHandler)
	s.mcp.AddPrompt(promptTemplates.FindVenuePrompt(), promptTemplates.FindVenueHandler)
	s.mcp.AddPrompt(promptTemplates.DuplicateCheckPrompt(), promptTemplates.DuplicateCheckHandler)
}

// Shutdown gracefully shuts down the MCP server and cleans up resources.
// This should be called when the server is no longer needed.
func (s *Server) Shutdown(ctx context.Context) error {
	// Currently no cleanup needed, but this provides a hook for future
	// resource cleanup (connections, subscriptions, etc.)
	return nil
}
