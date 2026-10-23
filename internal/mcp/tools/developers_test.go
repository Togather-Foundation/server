package tools

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// fakeDeveloperService implements DeveloperService and records every call so
// tests can assert that rejected calls never reach the service/repository.
type fakeDeveloperService struct {
	calls           []string
	developerIDs    []uuid.UUID
	keys            []*developers.APIKeyWithUsage
	createKeyResult *developers.APIKeyWithUsage
}

func (f *fakeDeveloperService) record(name string, developerID uuid.UUID) {
	f.calls = append(f.calls, name)
	f.developerIDs = append(f.developerIDs, developerID)
}

func (f *fakeDeveloperService) GetDeveloperByID(_ context.Context, id uuid.UUID) (*developers.Developer, error) {
	f.record("GetDeveloperByID", id)
	return &developers.Developer{ID: id, MaxKeys: developers.DefaultMaxKeys, IsActive: true}, nil
}

func (f *fakeDeveloperService) ListOwnKeys(_ context.Context, developerID uuid.UUID) ([]*developers.APIKeyWithUsage, error) {
	f.record("ListOwnKeys", developerID)
	return f.keys, nil
}

func (f *fakeDeveloperService) CheckAPIKeyOwnership(_ context.Context, keyID uuid.UUID, developerID uuid.UUID) (bool, error) {
	f.record("CheckAPIKeyOwnership", developerID)
	return true, nil
}

func (f *fakeDeveloperService) GetAPIKeyUsageStats(_ context.Context, _ uuid.UUID, _, _ time.Time) (int64, int64, []developers.DailyUsage, error) {
	f.record("GetAPIKeyUsageStats", uuid.Nil)
	return 0, 0, nil, nil
}

func (f *fakeDeveloperService) CreateAPIKey(_ context.Context, params developers.CreateAPIKeyParams) (string, *developers.APIKeyWithUsage, error) {
	f.record("CreateAPIKey", params.DeveloperID)
	keyInfo := f.createKeyResult
	if keyInfo == nil {
		keyInfo = &developers.APIKeyWithUsage{ID: uuid.New(), Name: params.Name, Role: "agent"}
	}
	return "plain-key", keyInfo, nil
}

func (f *fakeDeveloperService) RevokeOwnKey(_ context.Context, developerID uuid.UUID, _ uuid.UUID) error {
	f.record("RevokeOwnKey", developerID)
	return nil
}

func contextWithDeveloper(developerID string) context.Context {
	return auth.ContextWithAgentKey(context.Background(), &auth.APIKey{
		ID:          "key-id",
		DeveloperID: developerID,
	})
}

func callTool(ctx context.Context, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) (*mcp.CallToolResult, error) {
	return handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "test",
			Arguments: args,
		},
	})
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "expected text content")
	return text.Text
}

func TestAPIKeysHandler_BindsToAuthenticatedDeveloper(t *testing.T) {
	developerA := uuid.New()
	developerB := uuid.New()

	t.Run("mismatched developer_id is rejected with no service calls", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(contextWithDeveloper(developerA.String()), tools.APIKeysHandler, map[string]any{
			"developer_id": developerB.String(),
		})

		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, resultText(t, result), "developer_id does not match the authenticated API key's developer")
		require.Empty(t, service.calls)
	})

	t.Run("matching developer_id succeeds", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(contextWithDeveloper(developerA.String()), tools.APIKeysHandler, map[string]any{
			"developer_id": developerA.String(),
		})

		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Equal(t, []string{"GetDeveloperByID", "ListOwnKeys"}, service.calls)
		require.Equal(t, []uuid.UUID{developerA, developerA}, service.developerIDs)
	})

	t.Run("omitted developer_id uses the key's developer", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(contextWithDeveloper(developerA.String()), tools.APIKeysHandler, map[string]any{})

		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Equal(t, []string{"GetDeveloperByID", "ListOwnKeys"}, service.calls)
		require.Equal(t, []uuid.UUID{developerA, developerA}, service.developerIDs)
	})

	t.Run("unbound key is rejected with no service calls", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(contextWithDeveloper(""), tools.APIKeysHandler, map[string]any{})

		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, resultText(t, result), "not bound to a developer")
		require.Empty(t, service.calls)
	})

	t.Run("anonymous caller is rejected with no service calls", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(context.Background(), tools.APIKeysHandler, map[string]any{})

		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, resultText(t, result), "requires an authenticated API key")
		require.Empty(t, service.calls)
	})

	t.Run("key usage verifies ownership against the key's developer", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(contextWithDeveloper(developerA.String()), tools.APIKeysHandler, map[string]any{
			"key_id": uuid.New().String(),
		})

		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Equal(t, []string{"CheckAPIKeyOwnership", "GetAPIKeyUsageStats"}, service.calls)
		require.Equal(t, developerA, service.developerIDs[0])
	})
}

func TestManageAPIKeyHandler_BindsToAuthenticatedDeveloper(t *testing.T) {
	developerA := uuid.New()
	developerB := uuid.New()

	t.Run("create cross-developer is rejected with no service calls", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(contextWithDeveloper(developerA.String()), tools.ManageAPIKeyHandler, map[string]any{
			"action":       "create",
			"developer_id": developerB.String(),
			"name":         "stolen key",
		})

		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, resultText(t, result), "developer_id does not match the authenticated API key's developer")
		require.Empty(t, service.calls)
	})

	t.Run("revoke cross-developer is rejected with no service calls", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(contextWithDeveloper(developerA.String()), tools.ManageAPIKeyHandler, map[string]any{
			"action":       "revoke",
			"developer_id": developerB.String(),
			"key_id":       uuid.New().String(),
		})

		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, resultText(t, result), "developer_id does not match the authenticated API key's developer")
		require.Empty(t, service.calls)
	})

	t.Run("create uses the key's developer when developer_id is omitted", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(contextWithDeveloper(developerA.String()), tools.ManageAPIKeyHandler, map[string]any{
			"action": "create",
			"name":   "new key",
		})

		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Equal(t, []string{"CreateAPIKey"}, service.calls)
		require.Equal(t, []uuid.UUID{developerA}, service.developerIDs)
	})

	t.Run("revoke uses the key's developer when developer_id matches", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(contextWithDeveloper(developerA.String()), tools.ManageAPIKeyHandler, map[string]any{
			"action":       "revoke",
			"developer_id": developerA.String(),
			"key_id":       uuid.New().String(),
		})

		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Equal(t, []string{"RevokeOwnKey"}, service.calls)
		require.Equal(t, []uuid.UUID{developerA}, service.developerIDs)
	})

	t.Run("unbound key is rejected with no service calls", func(t *testing.T) {
		service := &fakeDeveloperService{}
		tools := NewDeveloperTools(service, "")

		result, err := callTool(contextWithDeveloper(""), tools.ManageAPIKeyHandler, map[string]any{
			"action": "create",
			"name":   "new key",
		})

		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, resultText(t, result), "not bound to a developer")
		require.Empty(t, service.calls)
	})
}
