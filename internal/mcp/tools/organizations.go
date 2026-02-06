package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/mark3labs/mcp-go/mcp"
)

// OrganizationTools provides MCP tools for querying and managing organizations.
type OrganizationTools struct {
	orgService *organizations.Service
	baseURL    string
}

// NewOrganizationTools creates a new OrganizationTools instance.
func NewOrganizationTools(orgService *organizations.Service, baseURL string) *OrganizationTools {
	return &OrganizationTools{
		orgService: orgService,
		baseURL:    strings.TrimSpace(baseURL),
	}
}

// ListOrganizationsTool returns the MCP tool definition for listing organizations.
func (t *OrganizationTools) ListOrganizationsTool() mcp.Tool {
	return mcp.Tool{
		Name:        "list_organizations",
		Description: "List organizations with optional filters for query and pagination. Returns a JSON array of organizations matching the criteria.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query to filter organizations by name or legal name",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of organizations to return (default: 50, max: 200)",
					"default":     50,
				},
				"cursor": map[string]interface{}{
					"type":        "string",
					"description": "Pagination cursor from a previous response",
				},
			},
		},
	}
}

// ListOrganizationsHandler handles the list_organizations tool call.
func (t *OrganizationTools) ListOrganizationsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.orgService == nil {
		return mcp.NewToolResultError("organizations service not configured"), nil
	}

	args := struct {
		Query  string `json:"query"`
		Limit  int    `json:"limit"`
		Cursor string `json:"cursor"`
	}{
		Limit: 50,
	}

	if request.Params.Arguments != nil {
		data, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
	}

	values := url.Values{}
	if strings.TrimSpace(args.Query) != "" {
		values.Set("q", strings.TrimSpace(args.Query))
	}
	if args.Limit > 0 {
		values.Set("limit", strconv.Itoa(args.Limit))
	}
	if strings.TrimSpace(args.Cursor) != "" {
		values.Set("after", strings.TrimSpace(args.Cursor))
	}

	filters, pagination, err := organizations.ParseFilters(values)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("invalid filters", err), nil
	}

	result, err := t.orgService.List(ctx, filters, pagination)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to list organizations", err), nil
	}

	items := make([]map[string]any, 0, len(result.Organizations))
	for _, org := range result.Organizations {
		items = append(items, buildOrganizationListItem(org, t.baseURL))
	}

	response := map[string]any{
		"items":       items,
		"next_cursor": result.NextCursor,
	}

	return toolResultJSON(response)
}

// GetOrganizationTool returns the MCP tool definition for getting a single organization by ID.
func (t *OrganizationTools) GetOrganizationTool() mcp.Tool {
	return mcp.Tool{
		Name:        "get_organization",
		Description: "Get detailed information about a specific organization by its ULID. Returns a JSON-LD formatted organization object.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "The ULID of the organization to retrieve",
				},
			},
			Required: []string{"id"},
		},
	}
}

// GetOrganizationHandler handles the get_organization tool call.
func (t *OrganizationTools) GetOrganizationHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.orgService == nil {
		return mcp.NewToolResultError("organizations service not configured"), nil
	}

	args := struct {
		ID string `json:"id"`
	}{}

	if request.Params.Arguments != nil {
		data, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
	}

	if strings.TrimSpace(args.ID) == "" {
		return mcp.NewToolResultError("id parameter is required"), nil
	}

	if err := ids.ValidateULID(args.ID); err != nil {
		return mcp.NewToolResultErrorFromErr("invalid ULID format", err), nil
	}

	org, err := t.orgService.GetByULID(ctx, args.ID)
	if err != nil {
		if errors.Is(err, organizations.ErrNotFound) {
			if tombstone, tombErr := t.orgService.GetTombstoneByULID(ctx, args.ID); tombErr == nil && tombstone != nil {
				payload, payloadErr := decodeTombstonePayload(tombstone.Payload)
				if payloadErr != nil {
					return mcp.NewToolResultErrorFromErr("failed to decode tombstone payload", payloadErr), nil
				}
				return toolResultJSON(payload)
			}
			return mcp.NewToolResultErrorf("organization not found: %s", args.ID), nil
		}
		return mcp.NewToolResultErrorFromErr("failed to get organization", err), nil
	}

	if strings.EqualFold(org.Lifecycle, "deleted") {
		if tombstone, tombErr := t.orgService.GetTombstoneByULID(ctx, args.ID); tombErr == nil && tombstone != nil {
			payload, payloadErr := decodeTombstonePayload(tombstone.Payload)
			if payloadErr != nil {
				return mcp.NewToolResultErrorFromErr("failed to decode tombstone payload", payloadErr), nil
			}
			return toolResultJSON(payload)
		}

		payload := map[string]any{
			"@context":      defaultContext(),
			"@type":         "Organization",
			"sel:tombstone": true,
			"sel:deletedAt": time.Now().Format(time.RFC3339),
		}
		if uri := buildOrganizationURI(t.baseURL, org.ULID); uri != "" {
			payload["@id"] = uri
		}
		return toolResultJSON(payload)
	}

	payload := buildOrganizationPayload(org, t.baseURL)
	return toolResultJSON(payload)
}

// CreateOrganizationTool returns the MCP tool definition for creating an organization.
func (t *OrganizationTools) CreateOrganizationTool() mcp.Tool {
	return mcp.Tool{
		Name:        "create_organization",
		Description: "Create a new organization. Accepts a JSON-LD organization object and returns the created organization with its assigned ID.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"organization": map[string]interface{}{
					"type":        "object",
					"description": "JSON-LD organization object with organization details",
				},
			},
			Required: []string{"organization"},
		},
	}
}

// CreateOrganizationHandler handles the create_organization tool call.
func (t *OrganizationTools) CreateOrganizationHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.orgService == nil {
		return mcp.NewToolResultError("organizations service not configured"), nil
	}

	args := struct {
		Organization json.RawMessage `json:"organization"`
	}{}

	if request.Params.Arguments != nil {
		data, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return mcp.NewToolResultErrorFromErr("invalid arguments", err), nil
		}
	}

	if len(args.Organization) == 0 {
		return mcp.NewToolResultError("organization parameter is required"), nil
	}

	var raw map[string]any
	if err := json.Unmarshal(args.Organization, &raw); err != nil {
		return mcp.NewToolResultErrorFromErr("invalid organization payload", err), nil
	}

	params, err := parseCreateOrganizationParams(raw, t.baseURL)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("invalid organization payload", err), nil
	}

	org, err := t.orgService.Create(ctx, params)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to create organization", err), nil
	}

	response := map[string]any{
		"id":           org.ULID,
		"organization": buildOrganizationPayload(org, t.baseURL),
	}
	if uri := buildOrganizationURI(t.baseURL, org.ULID); uri != "" {
		response["@id"] = uri
	}

	return toolResultJSON(response)
}

func buildOrganizationListItem(org organizations.Organization, baseURL string) map[string]any {
	item := map[string]any{
		"@context": defaultContext(),
		"@type":    "Organization",
		"name":     org.Name,
	}
	if org.LegalName != "" {
		item["legalName"] = org.LegalName
	}
	if uri := buildOrganizationURI(baseURL, org.ULID); uri != "" {
		item["@id"] = uri
	}
	return item
}

func buildOrganizationPayload(org *organizations.Organization, baseURL string) map[string]any {
	if org == nil {
		return map[string]any{}
	}

	payload := map[string]any{
		"@context": defaultContext(),
		"@type":    "Organization",
		"name":     org.Name,
	}
	if uri := buildOrganizationURI(baseURL, org.ULID); uri != "" {
		payload["@id"] = uri
	}
	if org.LegalName != "" {
		payload["legalName"] = org.LegalName
	}
	if org.Description != "" {
		payload["description"] = org.Description
	}
	if org.URL != "" {
		payload["url"] = org.URL
	}

	return payload
}

func buildOrganizationURI(baseURL, ulid string) string {
	if baseURL == "" || ulid == "" {
		return ""
	}
	uri, err := ids.BuildCanonicalURI(baseURL, "organizations", ulid)
	if err != nil {
		return ""
	}
	return uri
}

func parseCreateOrganizationParams(raw map[string]any, baseURL string) (organizations.CreateParams, error) {
	var params organizations.CreateParams

	name := strings.TrimSpace(getString(raw["name"]))
	if name == "" {
		return params, errors.New("name is required")
	}
	params.Name = name
	params.LegalName = strings.TrimSpace(getString(raw["legalName"]))
	params.Description = strings.TrimSpace(getString(raw["description"]))
	params.URL = strings.TrimSpace(getString(raw["url"]))

	address := map[string]any{}
	if addrValue, ok := raw["address"]; ok {
		if addrMap, ok := addrValue.(map[string]any); ok {
			address = addrMap
		}
	}

	street := strings.TrimSpace(getString(address["streetAddress"]))
	locality := strings.TrimSpace(getString(address["addressLocality"]))
	region := strings.TrimSpace(getString(address["addressRegion"]))
	postal := strings.TrimSpace(getString(address["postalCode"]))
	country := strings.TrimSpace(getString(address["addressCountry"]))

	if street == "" {
		street = strings.TrimSpace(getString(raw["streetAddress"]))
	}
	if locality == "" {
		locality = strings.TrimSpace(getString(raw["addressLocality"]))
	}
	if region == "" {
		region = strings.TrimSpace(getString(raw["addressRegion"]))
	}
	if postal == "" {
		postal = strings.TrimSpace(getString(raw["postalCode"]))
	}
	if country == "" {
		country = strings.TrimSpace(getString(raw["addressCountry"]))
	}

	params.StreetAddress = street
	params.AddressLocality = locality
	params.AddressRegion = region
	params.PostalCode = postal
	params.AddressCountry = country

	if idValue := strings.TrimSpace(getString(raw["@id"])); idValue != "" {
		if strings.TrimSpace(baseURL) == "" {
			params.FederationURI = &idValue
		} else {
			if parsed, err := ids.ParseEntityURI(baseURL, "organizations", idValue, "federation"); err == nil {
				if parsed.Role == ids.RoleCanonical {
					params.ULID = parsed.ULID
				} else {
					params.FederationURI = &idValue
				}
			} else {
				params.FederationURI = &idValue
			}
		}
	}

	return params, nil
}
