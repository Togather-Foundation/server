package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/mcp"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestMCPServerInitializeAndTools(t *testing.T) {
	env := setupTestEnv(t)

	repo, err := postgres.NewRepository(env.Pool)
	require.NoError(t, err)

	eventsService := events.NewService(repo.Events())
	ingestService := eventsIngestService(t, repo, env)
	placesService := repo.Places().Service()
	orgService := repo.Organizations().Service()

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

	cli := client.NewInProcessClient(mcpServer.MCPServer())
	defer cli.Close()

	ctx := context.Background()
	_, err = cli.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeRequestParams{
			ProtocolVersion: "2024-11-05",
			Capabilities: mcp.ClientCapabilities{
				Tools:     &mcp.ToolsCapability{},
				Resources: &mcp.ResourcesCapability{},
				Prompts:   &mcp.PromptsCapability{},
			},
			ClientInfo: mcp.Implementation{
				Name:    "mcp-test-client",
				Version: "1.0.0",
			},
		},
	})
	require.NoError(t, err)

	result, err := cli.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
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
	placesService := repo.Places().Service()
	orgService := repo.Organizations().Service()
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

	cli := client.NewInProcessClient(mcpServer.MCPServer())
	defer cli.Close()

	ctx := context.Background()
	_, err = cli.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeRequestParams{
			ProtocolVersion: "2024-11-05",
			Capabilities: mcp.ClientCapabilities{
				Resources: &mcp.ResourcesCapability{},
			},
			ClientInfo: mcp.Implementation{
				Name:    "mcp-test-client",
				Version: "1.0.0",
			},
		},
	})
	require.NoError(t, err)

	resourceResult, err := cli.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceRequestParams{
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

func decodeToolText(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &payload))
	return payload
}
