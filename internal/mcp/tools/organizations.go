package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
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

// OrganizationsTool returns the MCP tool definition for listing or getting organizations.
// If id parameter is provided, returns a single organization. Otherwise, returns a list of organizations.
func (t *OrganizationTools) OrganizationsTool() mcp.Tool {
	return mcp.Tool{
		Name:        "organizations",
		Description: "List organizations with optional filters, or get a specific organization by ULID. If 'id' is provided, returns a single JSON-LD formatted organization. Otherwise, returns a JSON array of organizations matching the filter criteria.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Optional ULID of a specific organization to retrieve. If provided, other parameters are ignored.",
				},
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

// OrganizationsHandler handles the organizations tool call.
// If id is provided, delegates to get organization logic. Otherwise, delegates to list organizations logic.
func (t *OrganizationTools) OrganizationsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if t == nil || t.orgService == nil {
		return mcp.NewToolResultError("organizations service not configured"), nil
	}

	args := struct {
		ID     string `json:"id"`
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

	// If id is provided, get single organization
	if strings.TrimSpace(args.ID) != "" {
		return t.getOrganizationByID(ctx, strings.TrimSpace(args.ID))
	}

	// Otherwise, list organizations with filters
	return t.listOrganizations(ctx, args.Query, args.Limit, args.Cursor)
}

// getOrganizationByID retrieves a single organization by ULID.
// Returns tombstone data if the organization is deleted.
func (t *OrganizationTools) getOrganizationByID(ctx context.Context, id string) (*mcp.CallToolResult, error) {
	if err := ids.ValidateULID(id); err != nil {
		return mcp.NewToolResultErrorFromErr("invalid ULID format", err), nil
	}

	org, err := t.orgService.GetByULID(ctx, id)
	if err != nil {
		if errors.Is(err, organizations.ErrNotFound) {
			tombstone, tombErr := t.orgService.GetTombstoneByULID(ctx, id)
			if tombErr != nil && !errors.Is(tombErr, organizations.ErrNotFound) {
				// Log tombstone fetch error for diagnostics (don't fail the request)
				fmt.Fprintf(os.Stderr, "MCP: failed to fetch tombstone for organization %s: %v\n", id, tombErr)
			}
			if tombErr == nil && tombstone != nil {
				payload, payloadErr := decodeTombstonePayload(tombstone.Payload)
				if payloadErr != nil {
					return mcp.NewToolResultErrorFromErr("failed to decode tombstone payload", payloadErr), nil
				}
				return toolResultJSON(payload)
			}
			return mcp.NewToolResultErrorf("organization not found: %s", id), nil
		}
		return mcp.NewToolResultErrorFromErr("failed to get organization", err), nil
	}

	// Defensive nil check (should not happen, but be safe)
	if org == nil {
		return mcp.NewToolResultErrorf("organization not found: %s", id), nil
	}

	if strings.EqualFold(org.Lifecycle, "deleted") {
		tombstone, tombErr := t.orgService.GetTombstoneByULID(ctx, id)
		if tombErr != nil && !errors.Is(tombErr, organizations.ErrNotFound) {
			// Log tombstone fetch error for diagnostics (don't fail the request)
			fmt.Fprintf(os.Stderr, "MCP: failed to fetch tombstone for deleted organization %s: %v\n", id, tombErr)
		}
		if tombErr == nil && tombstone != nil {
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

// listOrganizations retrieves a list of organizations with optional filters and pagination.
// Supports filtering by query text matching name or legal name.
func (t *OrganizationTools) listOrganizations(ctx context.Context, query string, limit int, cursor string) (*mcp.CallToolResult, error) {

	const maxListLimit = 200

	// Enforce limit caps
	if limit <= 0 {
		limit = 50
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	values := url.Values{}
	if strings.TrimSpace(query) != "" {
		values.Set("q", strings.TrimSpace(query))
	}
	values.Set("limit", strconv.Itoa(limit))
	if strings.TrimSpace(cursor) != "" {
		values.Set("after", strings.TrimSpace(cursor))
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

// AddOrganizationTool returns the MCP tool definition for creating an organization.
func (t *OrganizationTools) AddOrganizationTool() mcp.Tool {
	return mcp.Tool{
		Name:        "add_organization",
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

// AddOrganizationHandler handles the add_organization tool call.
func (t *OrganizationTools) AddOrganizationHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
