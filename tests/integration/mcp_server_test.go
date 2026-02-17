package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/mcp"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/mark3labs/mcp-go/client"
	mcpTypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
)

func TestMCPServerInitializeAndTools(t *testing.T) {
	env := setupTestEnv(t)

	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	eventsService := events.NewService(repo.Events())
	ingestService := eventsIngestService(t, repo, env)
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())

	mcpServer := mcp.NewServer(
		mcp.Config{
			Name:      "Test MCP Server",
			Version:   "test",
			Transport: "inprocess",
		},
		eventsService,
		ingestService,
		placesService,
		orgService,
		nil, // developerService
		nil, // geocodingService
		env.Config.Server.BaseURL,
	)

	cli, err := client.NewInProcessClient(mcpServer.MCPServer())
	require.NoError(t, err)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()
	_, err = cli.Initialize(ctx, mcpTypes.InitializeRequest{
		Params: mcpTypes.InitializeParams{
			ProtocolVersion: "2024-11-05",
			Capabilities:    mcpTypes.ClientCapabilities{},
			ClientInfo: mcpTypes.Implementation{
				Name:    "mcp-test-client",
				Version: "1.0.0",
			},
		},
	})
	require.NoError(t, err)

	result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
		Params: mcpTypes.CallToolParams{
			Name:      "events",
			Arguments: map[string]any{},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
}

func TestMCPResources(t *testing.T) {
	env := setupTestEnv(t)

	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	eventsService := events.NewService(repo.Events())
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())
	ingestService := eventsIngestService(t, repo, env)

	mcpServer := mcp.NewServer(
		mcp.Config{
			Name:      "Test MCP Server",
			Version:   "test",
			Transport: "inprocess",
		},
		eventsService,
		ingestService,
		placesService,
		orgService,
		nil, // developerService
		nil, // geocodingService
		env.Config.Server.BaseURL,
	)

	cli, err := client.NewInProcessClient(mcpServer.MCPServer())
	require.NoError(t, err)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()
	_, err = cli.Initialize(ctx, mcpTypes.InitializeRequest{
		Params: mcpTypes.InitializeParams{
			ProtocolVersion: "2024-11-05",
			Capabilities:    mcpTypes.ClientCapabilities{},
			ClientInfo: mcpTypes.Implementation{
				Name:    "mcp-test-client",
				Version: "1.0.0",
			},
		},
	})
	require.NoError(t, err)

	resourceResult, err := cli.ReadResource(ctx, mcpTypes.ReadResourceRequest{
		Params: mcpTypes.ReadResourceParams{
			URI: "schema://openapi",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resourceResult)
	require.NotEmpty(t, resourceResult.Contents)
}

func eventsIngestService(t *testing.T, repo *postgres.Repository, env *testEnv) *events.IngestService {
	t.Helper()
	return events.NewIngestService(repo.Events(), env.Config.Server.BaseURL, "America/Toronto", env.Config.Validation)
}

func decodeToolText(t *testing.T, result *mcpTypes.CallToolResult) map[string]any {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)

	textContent, ok := mcpTypes.AsTextContent(result.Content[0])
	require.True(t, ok)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &payload))
	return payload
}

// TestMCPAuthUnauthorized verifies that MCP HTTP requests without an API key are rejected
func TestMCPAuthUnauthorized(t *testing.T) {
	env := setupTestEnv(t)

	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	eventsService := events.NewService(repo.Events())
	ingestService := eventsIngestService(t, repo, env)
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())

	mcpServer := mcp.NewServer(
		mcp.Config{
			Name:      "Test MCP Server",
			Version:   "test",
			Transport: "http",
		},
		eventsService,
		ingestService,
		placesService,
		orgService,
		nil, // developerService
		nil, // geocodingService
		env.Config.Server.BaseURL,
	)

	// Create HTTP handler with auth middleware
	handler := server.NewStreamableHTTPServer(mcpServer.MCPServer())
	wrappedHandler, err := mcp.WrapHandler(handler, repo.Auth().APIKeys(), env.Config.RateLimit)
	require.NoError(t, err)

	testServer := httptest.NewServer(wrappedHandler)
	defer testServer.Close()

	// Test 1: Request with no Authorization header
	req, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(`{"jsonrpc": "2.0", "method": "initialize", "id": 1}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Request without API key should be rejected")

	// Test 2: Request with empty Authorization header
	req2, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(`{"jsonrpc": "2.0", "method": "initialize", "id": 1}`))
	require.NoError(t, err)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "")

	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()

	require.Equal(t, http.StatusUnauthorized, resp2.StatusCode, "Request with empty Authorization header should be rejected")

	// Test 3: Request with malformed Authorization header (no Bearer prefix)
	req3, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(`{"jsonrpc": "2.0", "method": "initialize", "id": 1}`))
	require.NoError(t, err)
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "InvalidFormat abc123")

	resp3, err := http.DefaultClient.Do(req3)
	require.NoError(t, err)
	defer func() { _ = resp3.Body.Close() }()

	require.Equal(t, http.StatusUnauthorized, resp3.StatusCode, "Request with malformed Authorization should be rejected")
}

// TestMCPAuthValidKey verifies that MCP HTTP requests with a valid API key succeed
func TestMCPAuthValidKey(t *testing.T) {
	env := setupTestEnv(t)

	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	// Create a valid API key
	apiKey := insertAPIKey(t, env, "test-mcp-key")

	eventsService := events.NewService(repo.Events())
	ingestService := eventsIngestService(t, repo, env)
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())

	mcpServer := mcp.NewServer(
		mcp.Config{
			Name:      "Test MCP Server",
			Version:   "test",
			Transport: "http",
		},
		eventsService,
		ingestService,
		placesService,
		orgService,
		nil, // developerService
		nil, // geocodingService
		env.Config.Server.BaseURL,
	)

	// Create HTTP handler with auth middleware
	handler := server.NewStreamableHTTPServer(mcpServer.MCPServer())
	wrappedHandler, err := mcp.WrapHandler(handler, repo.Auth().APIKeys(), env.Config.RateLimit)
	require.NoError(t, err)

	testServer := httptest.NewServer(wrappedHandler)
	defer testServer.Close()

	// Request with valid API key
	initPayload := `{"jsonrpc": "2.0", "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test", "version": "1.0"}}, "id": 1}`
	req, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(initPayload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Request with valid API key should succeed")

	// Verify we got a valid MCP response
	var mcpResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&mcpResp)
	require.NoError(t, err)
	require.Equal(t, "2.0", mcpResp["jsonrpc"], "Should return valid JSON-RPC response")
	require.NotNil(t, mcpResp["result"], "Should have result field")
}

// TestMCPAuthInvalidKey verifies that MCP HTTP requests with an invalid API key are rejected
func TestMCPAuthInvalidKey(t *testing.T) {
	env := setupTestEnv(t)

	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	eventsService := events.NewService(repo.Events())
	ingestService := eventsIngestService(t, repo, env)
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())

	mcpServer := mcp.NewServer(
		mcp.Config{
			Name:      "Test MCP Server",
			Version:   "test",
			Transport: "http",
		},
		eventsService,
		ingestService,
		placesService,
		orgService,
		nil, // developerService
		nil, // geocodingService
		env.Config.Server.BaseURL,
	)

	// Create HTTP handler with auth middleware
	handler := server.NewStreamableHTTPServer(mcpServer.MCPServer())
	wrappedHandler, err := mcp.WrapHandler(handler, repo.Auth().APIKeys(), env.Config.RateLimit)
	require.NoError(t, err)

	testServer := httptest.NewServer(wrappedHandler)
	defer testServer.Close()

	// Test 1: Request with non-existent API key
	req, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(`{"jsonrpc": "2.0", "method": "initialize", "id": 1}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer 01234567FAKEKEYTHATDOESNOTEXIST")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Request with invalid API key should be rejected")

	// Test 2: Request with key that's too short
	req2, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(`{"jsonrpc": "2.0", "method": "initialize", "id": 1}`))
	require.NoError(t, err)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer short")

	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()

	require.Equal(t, http.StatusUnauthorized, resp2.StatusCode, "Request with too-short API key should be rejected")
}

// TestMCPRateLimitTierAgent verifies that rate limiting middleware is applied to MCP endpoints
// TODO(server-5h1d): Rate limiting in httptest doesn't work as expected because httptest
// doesn't preserve RemoteAddr consistently across requests. Consider testing rate limiting
// with a real HTTP server or mocking the limiter store.
func TestMCPRateLimitTierAgent(t *testing.T) {
	t.Skip("Rate limiting test needs real HTTP server or mock limiter store - httptest RemoteAddr inconsistency")

	env := setupTestEnv(t)

	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	// Create a valid API key
	apiKey := insertAPIKey(t, env, "test-ratelimit-key")

	eventsService := events.NewService(repo.Events())
	ingestService := eventsIngestService(t, repo, env)
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())

	mcpServer := mcp.NewServer(
		mcp.Config{
			Name:      "Test MCP Server",
			Version:   "test",
			Transport: "http",
		},
		eventsService,
		ingestService,
		placesService,
		orgService,
		nil, // developerService
		nil, // geocodingService
		env.Config.Server.BaseURL,
	)

	// Create HTTP handler with VERY LOW rate limit for testing
	rateLimitCfg := config.RateLimitConfig{
		AgentPerMinute: 2, // Only allow 2 requests per minute
	}

	handler := server.NewStreamableHTTPServer(mcpServer.MCPServer())
	wrappedHandler, err := mcp.WrapHandler(handler, repo.Auth().APIKeys(), rateLimitCfg)
	require.NoError(t, err)

	testServer := httptest.NewServer(wrappedHandler)
	defer testServer.Close()

	initPayload := `{"jsonrpc": "2.0", "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test", "version": "1.0"}}, "id": 1}`

	// First two requests should succeed (burst allowance)
	for i := 0; i < 2; i++ {
		req, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(initPayload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "Request %d should succeed (within burst limit)", i+1)
	}

	// Third request should be rate limited
	req3, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(initPayload))
	require.NoError(t, err)
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer "+apiKey)

	resp3, err := http.DefaultClient.Do(req3)
	require.NoError(t, err)
	defer func() { _ = resp3.Body.Close() }()

	require.Equal(t, http.StatusTooManyRequests, resp3.StatusCode, "Request 3 should be rate limited")
	require.NotEmpty(t, resp3.Header.Get("Retry-After"), "Should include Retry-After header")
}

// TestMCPCreateEvent tests the create_event tool with success and error cases.
func TestMCPCreateEvent(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		eventPayload := map[string]any{
			"@type":     "Event",
			"name":      "Test Concert",
			"startDate": "2026-12-01T19:00:00Z",
			"location": map[string]any{
				"@type": "VirtualLocation",
				"name":  "Online Event",
				"url":   "https://example.com/event",
			},
		}

		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "add_event",
				Arguments: map[string]any{
					"event": eventPayload,
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		// Check if it's an error response first
		textContent, isText := mcpTypes.AsTextContent(result.Content[0])
		if isText && (strings.Contains(textContent.Text, "error") || strings.Contains(textContent.Text, "Error") || strings.Contains(textContent.Text, "failed")) {
			t.Fatalf("Create event failed: %s", textContent.Text)
		}

		payload := decodeToolText(t, result)
		require.NotEmpty(t, payload["id"], "should return event ID")

		// Handle both boolean and pointer boolean types
		isDup := payload["is_duplicate"]
		if isDup != nil {
			switch v := isDup.(type) {
			case bool:
				require.False(t, v, "should not be duplicate")
			default:
				t.Logf("is_duplicate type: %T, value: %v", isDup, isDup)
			}
		}
	})

	t.Run("missing_event_param", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name:      "add_event",
				Arguments: map[string]any{},
			},
		})
		require.NoError(t, err)

		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "event parameter is required")
	})

	t.Run("invalid_event_payload", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "add_event",
				Arguments: map[string]any{
					"event": "not-a-json-object",
				},
			},
		})
		require.NoError(t, err)

		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "invalid")
	})
}

// TestMCPGetEvent tests the get_event tool with success and error cases.
func TestMCPGetEvent(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Create a test event first
	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	ingestService := eventsIngestService(t, repo, env)
	eventInput := events.EventInput{
		Name:      "Test Event for Get",
		StartDate: "2026-12-15T18:00:00Z",
		VirtualLocation: &events.VirtualLocationInput{
			URL: "https://example.com/test-event",
		},
	}
	result, err := ingestService.Ingest(ctx, eventInput)
	require.NoError(t, err)
	eventID := result.Event.ULID

	t.Run("success", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "events",
				Arguments: map[string]any{
					"id": eventID,
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		require.Equal(t, "Event", payload["@type"])
		require.Equal(t, "Test Event for Get", payload["name"])
	})

	t.Run("no_id_param_lists_events", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name:      "events",
				Arguments: map[string]any{},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		require.NotNil(t, payload["items"])
	})

	t.Run("invalid_ulid", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "events",
				Arguments: map[string]any{
					"id": "invalid-ulid",
				},
			},
		})
		require.NoError(t, err)

		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "invalid ULID")
	})

	t.Run("not_found", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "events",
				Arguments: map[string]any{
					"id": "01KGSV7H8ZDHTYTV6QKFGMFFMZ",
				},
			},
		})
		require.NoError(t, err)

		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "not found")
	})

	t.Run("whitespace_only_id_lists_events", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "events",
				Arguments: map[string]any{
					"id": "   ",
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		require.NotNil(t, payload["items"], "whitespace-only id should list events")
	})
}

// TestMCPListPlaces tests the list_places tool.
func TestMCPListPlaces(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Create test places via direct SQL insert (Create was removed from places.Service)
	testPlace := insertPlace(t, env, "Test Venue", "Toronto")
	_ = testPlace

	t.Run("success", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name:      "places",
				Arguments: map[string]any{},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		require.Contains(t, payload, "items")
		items := payload["items"].([]any)
		require.Greater(t, len(items), 0)
	})

	t.Run("with_query", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "places",
				Arguments: map[string]any{
					"query": "Test Venue",
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		items := payload["items"].([]any)
		require.Greater(t, len(items), 0)
	})

	t.Run("with_limit", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "places",
				Arguments: map[string]any{
					"limit": 1,
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		items := payload["items"].([]any)
		require.LessOrEqual(t, len(items), 1)
	})
}

// TestMCPGetPlace tests the get_place tool.
func TestMCPGetPlace(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Create a test place via direct SQL insert (Create was removed from places.Service)
	place := insertPlace(t, env, "Get Test Place", "Toronto")

	t.Run("success", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "places",
				Arguments: map[string]any{
					"id": place.ULID,
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		require.Equal(t, "Place", payload["@type"])
		require.Equal(t, "Get Test Place", payload["name"])
	})

	t.Run("not_found", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "places",
				Arguments: map[string]any{
					"id": "01KGSV7H8ZDDGC0HRAE8SSDK4Z",
				},
			},
		})
		require.NoError(t, err)

		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "not found")
	})

	t.Run("whitespace_only_id_lists_places", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "places",
				Arguments: map[string]any{
					"id": "   ",
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		require.NotNil(t, payload["items"], "whitespace-only id should list places")
	})
}

// TestMCPCreatePlace verifies that the add_place tool is currently disabled.
// Place creation was removed from the repository during the schema.org refactor.
// TODO(srv-d7cnu): Re-enable when place creation is restored.
func TestMCPCreatePlace(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	t.Run("tool_not_registered", func(t *testing.T) {
		_, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "add_place",
				Arguments: map[string]any{
					"place": map[string]any{
						"@type": "Place",
						"name":  "New Test Venue",
					},
				},
			},
		})
		require.Error(t, err, "add_place tool should not be registered (place creation disabled)")
	})
}

// TestMCPListOrganizations tests the list_organizations tool.
func TestMCPListOrganizations(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Create test organization via direct SQL insert (Create was removed from organizations.Service)
	_ = insertOrganization(t, env, "Test Org")

	t.Run("success", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name:      "organizations",
				Arguments: map[string]any{},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		require.Contains(t, payload, "items")
		items := payload["items"].([]any)
		require.Greater(t, len(items), 0)
	})

	t.Run("with_query", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "organizations",
				Arguments: map[string]any{
					"query": "Test Org",
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		items := payload["items"].([]any)
		require.Greater(t, len(items), 0)
	})
}

// TestMCPGetOrganization tests the get_organization tool.
func TestMCPGetOrganization(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Create a test organization via direct SQL insert (Create was removed from organizations.Service)
	org := insertOrganization(t, env, "Get Test Org")

	t.Run("success", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "organizations",
				Arguments: map[string]any{
					"id": org.ULID,
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		require.Equal(t, "Organization", payload["@type"])
		require.Equal(t, "Get Test Org", payload["name"])
	})

	t.Run("not_found", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "organizations",
				Arguments: map[string]any{
					"id": "01KGSV7H8Z62JFXE9CGKX3WRAG",
				},
			},
		})
		require.NoError(t, err)

		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "not found")
	})

	t.Run("whitespace_only_id_lists_organizations", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "organizations",
				Arguments: map[string]any{
					"id": "   ",
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		require.NotNil(t, payload["items"], "whitespace-only id should list organizations")
	})
}

// TestMCPCreateOrganization verifies that the add_organization tool is currently disabled.
// Organization creation was disabled during the schema.org rebase.
// TODO(srv-d7cnu): Re-enable when organization creation is restored.
func TestMCPCreateOrganization(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	t.Run("disabled", func(t *testing.T) {
		orgPayload := map[string]any{
			"@type":     "Organization",
			"name":      "New Test Company",
			"legalName": "New Test Company LLC",
			"url":       "https://example.com",
		}

		// add_organization tool is currently unregistered (commented out in server.go)
		// so calling it should return an error from the MCP server
		_, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "add_organization",
				Arguments: map[string]any{
					"organization": orgPayload,
				},
			},
		})
		require.Error(t, err, "expected error calling unregistered add_organization tool")
		require.Contains(t, err.Error(), "add_organization",
			"error should reference the tool name")
	})
}

// TestMCPSearch tests the cross-entity search tool.
func TestMCPSearch(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Create test data
	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	ingestService := eventsIngestService(t, repo, env)
	_, err = ingestService.Ingest(ctx, events.EventInput{
		Name:      "Music Festival",
		StartDate: "2026-06-01T12:00:00Z",
		VirtualLocation: &events.VirtualLocationInput{
			URL: "https://example.com/music-festival",
		},
	})
	require.NoError(t, err)

	// Create test place and org via direct SQL insert (Create was removed from domain services)
	_ = insertPlace(t, env, "Music Hall", "Toronto")
	_ = insertOrganization(t, env, "Music Society")

	t.Run("success_all_types", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "search",
				Arguments: map[string]any{
					"query": "Music",
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		require.Contains(t, payload, "items")
		require.Contains(t, payload, "count")
		items := payload["items"].([]any)
		require.Greater(t, len(items), 0)
	})

	t.Run("success_specific_types", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "search",
				Arguments: map[string]any{
					"query": "Music",
					"types": []string{"event", "place"},
				},
			},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		items := payload["items"].([]any)
		require.Greater(t, len(items), 0)
	})

	t.Run("missing_query", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name:      "search",
				Arguments: map[string]any{},
			},
		})
		require.NoError(t, err)

		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "query parameter is required")
	})

	t.Run("invalid_types", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "search",
				Arguments: map[string]any{
					"query": "test",
					"types": []string{"invalid_type"},
				},
			},
		})
		require.NoError(t, err)

		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "unsupported type")
	})
}

// setupMCPClient creates and initializes an MCP client for testing.
func setupMCPClient(t *testing.T, env *testEnv) *client.Client {
	t.Helper()

	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	eventsService := events.NewService(repo.Events())
	ingestService := eventsIngestService(t, repo, env)
	placesService := places.NewService(repo.Places())
	orgService := organizations.NewService(repo.Organizations())

	mcpServer := mcp.NewServer(
		mcp.Config{
			Name:        "Test MCP Server",
			Version:     "test",
			Transport:   "inprocess",
			ContextDir:  "../../contexts",
			OpenAPIPath: "../../docs/api/openapi.yaml",
		},
		eventsService,
		ingestService,
		placesService,
		orgService,
		nil, // developerService
		nil, // geocodingService
		env.Config.Server.BaseURL,
	)

	cli, err := client.NewInProcessClient(mcpServer.MCPServer())
	require.NoError(t, err)

	ctx := context.Background()
	_, err = cli.Initialize(ctx, mcpTypes.InitializeRequest{
		Params: mcpTypes.InitializeParams{
			ProtocolVersion: "2024-11-05",
			Capabilities:    mcpTypes.ClientCapabilities{},
			ClientInfo: mcpTypes.Implementation{
				Name:    "mcp-test-client",
				Version: "1.0.0",
			},
		},
	})
	require.NoError(t, err)

	return cli
}

// TestMCPPrompts verifies that all MCP prompts are available and return proper responses
func TestMCPPrompts(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Test create_event_from_text prompt
	t.Run("create_event_from_text", func(t *testing.T) {
		result, err := cli.GetPrompt(ctx, mcpTypes.GetPromptRequest{
			Params: mcpTypes.GetPromptParams{
				Name: "create_event_from_text",
				Arguments: map[string]string{
					"description":      "Community meetup at the library tomorrow at 7pm",
					"default_timezone": "America/Toronto",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEmpty(t, result.Messages)

		msg := result.Messages[0]
		textContent, ok := mcpTypes.AsTextContent(msg.Content)
		require.True(t, ok)
		require.Contains(t, textContent.Text, "Convert the following event description")
		require.Contains(t, textContent.Text, "America/Toronto")
	})

	// Test find_venue prompt
	t.Run("find_venue", func(t *testing.T) {
		result, err := cli.GetPrompt(ctx, mcpTypes.GetPromptRequest{
			Params: mcpTypes.GetPromptParams{
				Name: "find_venue",
				Arguments: map[string]string{
					"requirements": "wheelchair accessible, AV equipment",
					"location":     "downtown Toronto",
					"capacity":     "50",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEmpty(t, result.Messages)

		msg := result.Messages[0]
		textContent, ok := mcpTypes.AsTextContent(msg.Content)
		require.True(t, ok)
		require.Contains(t, textContent.Text, "Find a venue")
		require.Contains(t, textContent.Text, "wheelchair accessible")
		require.Contains(t, textContent.Text, "downtown Toronto")
	})

	// Test duplicate_check prompt
	t.Run("duplicate_check", func(t *testing.T) {
		result, err := cli.GetPrompt(ctx, mcpTypes.GetPromptRequest{
			Params: mcpTypes.GetPromptParams{
				Name: "duplicate_check",
				Arguments: map[string]string{
					"event_description": "Tech Talk: Introduction to Go",
					"date":              "2026-02-15",
					"location":          "Toronto Tech Hub",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEmpty(t, result.Messages)

		msg := result.Messages[0]
		textContent, ok := mcpTypes.AsTextContent(msg.Content)
		require.True(t, ok)
		require.Contains(t, textContent.Text, "potential duplicate events")
		require.Contains(t, textContent.Text, "Tech Talk")
		require.Contains(t, textContent.Text, "2026-02-15")
	})

	// Test list prompts
	t.Run("list_prompts", func(t *testing.T) {
		result, err := cli.ListPrompts(ctx, mcpTypes.ListPromptsRequest{})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Prompts, 3, "Should have exactly 3 prompts")

		promptNames := make(map[string]bool)
		for _, prompt := range result.Prompts {
			promptNames[prompt.Name] = true
		}
		require.True(t, promptNames["create_event_from_text"])
		require.True(t, promptNames["find_venue"])
		require.True(t, promptNames["duplicate_check"])
	})
}

// TestMCPResourcesComplete verifies that all MCP resources are available and readable
func TestMCPResourcesComplete(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Test list resources
	t.Run("list_resources", func(t *testing.T) {
		result, err := cli.ListResources(ctx, mcpTypes.ListResourcesRequest{})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Resources, 5, "Should have exactly 5 resources")

		resourceURIs := make(map[string]bool)
		for _, resource := range result.Resources {
			resourceURIs[resource.URI] = true
		}
		require.True(t, resourceURIs["context://sel-event"], "Should have SEL event context")
		require.True(t, resourceURIs["context://sel-place"], "Should have SEL place context")
		require.True(t, resourceURIs["context://sel-organization"], "Should have SEL organization context")
		require.True(t, resourceURIs["schema://openapi"], "Should have OpenAPI schema")
		require.True(t, resourceURIs["info://server"], "Should have server info")
	})

	// Test read SEL event context
	t.Run("sel_event_context", func(t *testing.T) {
		result, err := cli.ReadResource(ctx, mcpTypes.ReadResourceRequest{
			Params: mcpTypes.ReadResourceParams{
				URI: "context://sel-event",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEmpty(t, result.Contents)

		content := result.Contents[0]
		textContent, ok := mcpTypes.AsTextResourceContents(content)
		require.True(t, ok, "Content should be text resource type, got: %T", content)
		require.Contains(t, textContent.Text, "@context")
	})

	// Test read server info
	t.Run("server_info", func(t *testing.T) {
		result, err := cli.ReadResource(ctx, mcpTypes.ReadResourceRequest{
			Params: mcpTypes.ReadResourceParams{
				URI: "info://server",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEmpty(t, result.Contents)

		content := result.Contents[0]
		textContent, ok := mcpTypes.AsTextResourceContents(content)
		require.True(t, ok, "Content should be text resource type, got: %T", content)

		var info map[string]any
		err = json.Unmarshal([]byte(textContent.Text), &info)
		require.NoError(t, err)
		require.Equal(t, "Test MCP Server", info["name"])
		require.Equal(t, "test", info["version"])
		capabilities := info["capabilities"].(map[string]any)
		require.True(t, capabilities["tools"].(bool))
		require.True(t, capabilities["resources"].(bool))
		require.True(t, capabilities["prompts"].(bool))
	})
}

// TestMCPToolValidation verifies that tools validate their arguments properly
func TestMCPToolValidation(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Test get_event with invalid ULID
	t.Run("events_invalid_ulid", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "events",
				Arguments: map[string]any{
					"id": "not-a-valid-ulid",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "invalid")
	})

	// Test list_events with invalid date format
	t.Run("events_invalid_date", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "events",
				Arguments: map[string]any{
					"start_date": "not-a-date",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "invalid")
	})

	// Test create_event with missing required fields
	t.Run("create_event_missing_fields", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "add_event",
				Arguments: map[string]any{
					"event": map[string]any{
						"description": "Event without required fields",
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "required")
	})
}

// TestMCPPagination verifies that pagination works correctly across list tools
func TestMCPPagination(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Create test data
	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	// Create multiple events for pagination testing
	ingestService := eventsIngestService(t, repo, env)
	for i := 0; i < 5; i++ {
		_, err := ingestService.Ingest(ctx, events.EventInput{
			Name:      "Pagination Test Event",
			StartDate: "2026-06-01T12:00:00Z",
			VirtualLocation: &events.VirtualLocationInput{
				URL: "https://example.com/event",
			},
		})
		require.NoError(t, err)
	}

	// Test list_events pagination
	t.Run("events_pagination", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "events",
				Arguments: map[string]any{
					"limit": 2,
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		payload := decodeToolText(t, result)
		items, ok := payload["items"].([]any)
		require.True(t, ok, "Response should have items array")
		require.LessOrEqual(t, len(items), 2, "Should respect limit")
	})
}

// TestMCPErrorHandling verifies that errors are handled correctly across all tools
func TestMCPErrorHandling(t *testing.T) {
	env := setupTestEnv(t)
	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	// Test calling non-existent tool
	t.Run("nonexistent_tool", func(t *testing.T) {
		_, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name:      "nonexistent_tool",
				Arguments: map[string]any{},
			},
		})
		require.Error(t, err, "Should error when calling non-existent tool")
	})

	// Test search with empty query
	t.Run("search_empty_query", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{
				Name: "search",
				Arguments: map[string]any{
					"query": "",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		textContent, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.Contains(t, textContent.Text, "required")
	})

	// Test reading non-existent resource
	t.Run("nonexistent_resource", func(t *testing.T) {
		_, err := cli.ReadResource(ctx, mcpTypes.ReadResourceRequest{
			Params: mcpTypes.ReadResourceParams{
				URI: "context://nonexistent",
			},
		})
		require.Error(t, err, "Should error when reading non-existent resource")
	})

	// Test getting non-existent prompt
	t.Run("nonexistent_prompt", func(t *testing.T) {
		_, err := cli.GetPrompt(ctx, mcpTypes.GetPromptRequest{
			Params: mcpTypes.GetPromptParams{
				Name:      "nonexistent_prompt",
				Arguments: map[string]string{},
			},
		})
		require.Error(t, err, "Should error when getting non-existent prompt")
	})
}
