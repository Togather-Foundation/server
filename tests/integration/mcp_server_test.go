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
		env.Config.Server.BaseURL,
	)

	cli, err := client.NewInProcessClient(mcpServer.MCPServer())
	require.NoError(t, err)
	defer cli.Close()

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
			Name:      "list_events",
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
		env.Config.Server.BaseURL,
	)

	cli, err := client.NewInProcessClient(mcpServer.MCPServer())
	require.NoError(t, err)
	defer cli.Close()

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
	return events.NewIngestService(repo.Events(), env.Config.Server.BaseURL)
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
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Request without API key should be rejected")

	// Test 2: Request with empty Authorization header
	req2, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(`{"jsonrpc": "2.0", "method": "initialize", "id": 1}`))
	require.NoError(t, err)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "")

	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp2.StatusCode, "Request with empty Authorization header should be rejected")

	// Test 3: Request with malformed Authorization header (no Bearer prefix)
	req3, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(`{"jsonrpc": "2.0", "method": "initialize", "id": 1}`))
	require.NoError(t, err)
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "InvalidFormat abc123")

	resp3, err := http.DefaultClient.Do(req3)
	require.NoError(t, err)
	defer resp3.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Request with invalid API key should be rejected")

	// Test 2: Request with key that's too short
	req2, err := http.NewRequest(http.MethodPost, testServer.URL, strings.NewReader(`{"jsonrpc": "2.0", "method": "initialize", "id": 1}`))
	require.NoError(t, err)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer short")

	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

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
	defer resp3.Body.Close()

	require.Equal(t, http.StatusTooManyRequests, resp3.StatusCode, "Request 3 should be rate limited")
	require.NotEmpty(t, resp3.Header.Get("Retry-After"), "Should include Retry-After header")
}
