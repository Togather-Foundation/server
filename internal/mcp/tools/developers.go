package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
)

// DeveloperTools provides MCP tools for developer API key management.
type DeveloperTools struct {
	service *developers.Service
	baseURL string
}

// NewDeveloperTools creates a new DeveloperTools instance.
func NewDeveloperTools(service *developers.Service, baseURL string) *DeveloperTools {
	return &DeveloperTools{
		service: service,
		baseURL: baseURL,
	}
}

// APIKeysTool returns the MCP tool definition for listing or getting API keys.
// If key_id is provided, returns detailed usage stats for that key. Otherwise, lists all keys.
func (t *DeveloperTools) APIKeysTool() mcp.Tool {
	return mcp.Tool{
		Name:        "api_keys",
		Description: "List all API keys for the authenticated developer OR get detailed usage statistics for a specific key. If 'key_id' is provided, returns usage stats with daily breakdown. Otherwise, returns all keys with summary usage (today, 7d, 30d). Does not return actual key values (they cannot be retrieved after creation).",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"developer_id": map[string]interface{}{
					"type":        "string",
					"description": "UUID of the authenticated developer (required)",
				},
				"key_id": map[string]interface{}{
					"type":        "string",
					"description": "Optional UUID of a specific API key. If provided, returns detailed usage statistics for this key only.",
				},
				"from": map[string]interface{}{
					"type":        "string",
					"description": "Start date in YYYY-MM-DD format (only used with key_id, default: 30 days ago)",
				},
				"to": map[string]interface{}{
					"type":        "string",
					"description": "End date in YYYY-MM-DD format (only used with key_id, default: today)",
				},
			},
			Required: []string{"developer_id"},
		},
	}
}

// APIKeysHandler handles the api_keys tool call.
// Routes to either list all keys or get specific key usage based on presence of key_id.
func (t *DeveloperTools) APIKeysHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.service == nil {
		return mcp.NewToolResultError("developer service not configured"), nil
	}

	// Parse arguments
	args := struct {
		DeveloperID string `json:"developer_id"`
		KeyID       string `json:"key_id,omitempty"`
		From        string `json:"from,omitempty"`
		To          string `json:"to,omitempty"`
	}{}

	if request.Params.Arguments != nil {
		data, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
		}
	}

	// Validate developer ID
	developerID, err := uuid.Parse(args.DeveloperID)
	if err != nil {
		return mcp.NewToolResultError("invalid developer_id: must be a valid UUID"), nil
	}

	// If key_id is provided, get detailed usage for that specific key
	if args.KeyID != "" {
		return t.handleGetKeyUsage(ctx, developerID, args.KeyID, args.From, args.To)
	}

	// Otherwise, list all keys
	return t.handleListKeys(ctx, developerID)
}

// handleListKeys lists all API keys for a developer with summary usage stats.
func (t *DeveloperTools) handleListKeys(ctx context.Context, developerID uuid.UUID) (*mcp.CallToolResult, error) {
	// Get developer to retrieve max_keys limit
	developer, err := t.service.GetDeveloperByID(ctx, developerID)
	if err != nil {
		return mcp.NewToolResultError("developer not found"), nil
	}

	// List all keys for this developer
	keys, err := t.service.ListOwnKeys(ctx, developerID)
	if err != nil {
		return mcp.NewToolResultError("failed to list API keys: " + err.Error()), nil
	}

	// Convert to response format
	items := make([]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		item := map[string]interface{}{
			"id":              key.ID.String(),
			"prefix":          key.Prefix,
			"name":            key.Name,
			"role":            key.Role,
			"rate_limit_tier": key.RateLimitTier,
			"is_active":       key.IsActive,
			"created_at":      key.CreatedAt.Format(time.RFC3339),
			"usage_today":     key.UsageToday,
			"usage_7d":        key.Usage7d,
			"usage_30d":       key.Usage30d,
		}

		if key.LastUsedAt != nil {
			item["last_used_at"] = key.LastUsedAt.Format(time.RFC3339)
		}

		if key.ExpiresAt != nil {
			item["expires_at"] = key.ExpiresAt.Format(time.RFC3339)
		}

		items = append(items, item)
	}

	// Use developer's max_keys, fallback to 5 if not set
	maxKeys := developer.MaxKeys
	if maxKeys == 0 {
		maxKeys = 5
	}

	response := map[string]interface{}{
		"items":     items,
		"max_keys":  maxKeys,
		"key_count": len(keys),
	}

	return toolResultJSON(response)
}

// handleGetKeyUsage gets detailed usage statistics for a specific API key.
func (t *DeveloperTools) handleGetKeyUsage(ctx context.Context, developerID uuid.UUID, keyIDStr, fromStr, toStr string) (*mcp.CallToolResult, error) {
	// Validate key ID
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		return mcp.NewToolResultError("invalid key_id: must be a valid UUID"), nil
	}

	// Parse date range
	now := time.Now()
	defaultFrom := now.AddDate(0, 0, -30) // Last 30 days by default

	var from, to time.Time
	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return mcp.NewToolResultError("invalid 'from' date format (expected YYYY-MM-DD)"), nil
		}
	} else {
		from = defaultFrom
	}

	if toStr != "" {
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			return mcp.NewToolResultError("invalid 'to' date format (expected YYYY-MM-DD)"), nil
		}
	} else {
		to = now
	}

	// Verify ownership
	owns, err := t.service.CheckAPIKeyOwnership(ctx, keyID, developerID)
	if err != nil {
		return mcp.NewToolResultError("failed to verify API key ownership: " + err.Error()), nil
	}
	if !owns {
		return mcp.NewToolResultError("API key not found"), nil
	}

	// Get usage statistics
	totalRequests, totalErrors, dailyRecords, err := t.service.GetAPIKeyUsageStats(ctx, keyID, from, to)
	if err != nil {
		return mcp.NewToolResultError("failed to get usage statistics: " + err.Error()), nil
	}

	// Convert daily records to response format
	daily := make([]map[string]interface{}, 0, len(dailyRecords))
	for _, record := range dailyRecords {
		daily = append(daily, map[string]interface{}{
			"date":     record.Date.Format("2006-01-02"),
			"requests": strconv.FormatInt(record.Requests, 10),
			"errors":   strconv.FormatInt(record.Errors, 10),
		})
	}

	response := map[string]interface{}{
		"api_key_id": keyID.String(),
		"period": map[string]interface{}{
			"from": from.Format(time.RFC3339),
			"to":   to.Format(time.RFC3339),
		},
		"total_requests": strconv.FormatInt(totalRequests, 10),
		"total_errors":   strconv.FormatInt(totalErrors, 10),
		"daily":          daily,
	}

	return toolResultJSON(response)
}

// ManageAPIKeyTool returns the MCP tool definition for creating or revoking API keys.
// Single management tool for key lifecycle operations.
func (t *DeveloperTools) ManageAPIKeyTool() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_api_key",
		Description: "Create or revoke API keys for the authenticated developer. Use action='create' to generate a new key (returns the key value which can only be seen once) or action='revoke' to deactivate an existing key. All keys are created with role='agent' and subject to max_keys limit (default 5).",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"create", "revoke"},
					"description": "Action to perform: 'create' for new key, 'revoke' to deactivate existing key",
				},
				"developer_id": map[string]interface{}{
					"type":        "string",
					"description": "UUID of the authenticated developer (required)",
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Name for the API key (required for create, e.g., 'Production Key', 'Testing Key')",
				},
				"expires_in_days": map[string]interface{}{
					"type":        "integer",
					"description": "Number of days until key expires (optional for create, e.g., 30, 90, 365)",
				},
				"key_id": map[string]interface{}{
					"type":        "string",
					"description": "UUID of the API key to revoke (required for revoke)",
				},
			},
			Required: []string{"action", "developer_id"},
		},
	}
}

// ManageAPIKeyHandler handles the manage_api_key tool call.
// Routes to create or revoke based on the action parameter.
func (t *DeveloperTools) ManageAPIKeyHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.service == nil {
		return mcp.NewToolResultError("developer service not configured"), nil
	}

	// Parse arguments
	args := struct {
		Action        string `json:"action"`
		DeveloperID   string `json:"developer_id"`
		Name          string `json:"name,omitempty"`
		ExpiresInDays *int   `json:"expires_in_days,omitempty"`
		KeyID         string `json:"key_id,omitempty"`
	}{}

	if request.Params.Arguments != nil {
		data, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return mcp.NewToolResultError("invalid arguments: " + err.Error()), nil
		}
	}

	// Validate action
	if args.Action != "create" && args.Action != "revoke" {
		return mcp.NewToolResultError("action must be 'create' or 'revoke'"), nil
	}

	// Validate developer ID
	developerID, err := uuid.Parse(args.DeveloperID)
	if err != nil {
		return mcp.NewToolResultError("invalid developer_id: must be a valid UUID"), nil
	}

	// Route to appropriate handler
	switch args.Action {
	case "create":
		return t.handleCreateKey(ctx, developerID, args.Name, args.ExpiresInDays)
	case "revoke":
		return t.handleRevokeKey(ctx, developerID, args.KeyID)
	default:
		return mcp.NewToolResultError("invalid action"), nil
	}
}

// handleCreateKey creates a new API key.
func (t *DeveloperTools) handleCreateKey(ctx context.Context, developerID uuid.UUID, name string, expiresInDays *int) (*mcp.CallToolResult, error) {
	// Validate name
	if name == "" {
		return mcp.NewToolResultError("name is required for action='create'"), nil
	}

	// Create API key
	params := developers.CreateAPIKeyParams{
		DeveloperID:   developerID,
		Name:          name,
		ExpiresInDays: expiresInDays,
	}

	plainKey, keyInfo, err := t.service.CreateAPIKey(ctx, params)
	if err != nil {
		// Map domain errors to user-friendly messages
		switch err {
		case developers.ErrMaxKeysReached:
			return mcp.NewToolResultError("maximum number of API keys reached - please revoke an existing key before creating a new one"), nil
		case developers.ErrDeveloperNotFound:
			return mcp.NewToolResultError("developer not found"), nil
		default:
			return mcp.NewToolResultError("failed to create API key: " + err.Error()), nil
		}
	}

	// Build response
	response := map[string]interface{}{
		"action":     "create",
		"id":         keyInfo.ID.String(),
		"name":       keyInfo.Name,
		"prefix":     keyInfo.Prefix,
		"key":        plainKey,
		"role":       keyInfo.Role,
		"created_at": keyInfo.CreatedAt.Format(time.RFC3339),
		"warning":    "IMPORTANT: Save this API key now. You won't be able to see it again. Store it securely.",
	}

	if keyInfo.ExpiresAt != nil {
		response["expires_at"] = keyInfo.ExpiresAt.Format(time.RFC3339)
	}

	return toolResultJSON(response)
}

// handleRevokeKey revokes an existing API key.
func (t *DeveloperTools) handleRevokeKey(ctx context.Context, developerID uuid.UUID, keyIDStr string) (*mcp.CallToolResult, error) {
	// Validate key ID
	if keyIDStr == "" {
		return mcp.NewToolResultError("key_id is required for action='revoke'"), nil
	}

	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		return mcp.NewToolResultError("invalid key_id: must be a valid UUID"), nil
	}

	// Revoke the key
	if err := t.service.RevokeOwnKey(ctx, developerID, keyID); err != nil {
		// Map domain errors to user-friendly messages
		switch err {
		case developers.ErrAPIKeyNotFound:
			return mcp.NewToolResultError("API key not found"), nil
		case developers.ErrUnauthorized:
			return mcp.NewToolResultError("you do not own this API key"), nil
		default:
			return mcp.NewToolResultError("failed to revoke API key: " + err.Error()), nil
		}
	}

	response := map[string]interface{}{
		"action":  "revoke",
		"success": true,
		"message": fmt.Sprintf("API key %s successfully revoked", keyID.String()),
	}

	return toolResultJSON(response)
}
