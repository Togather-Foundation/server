package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/jsonld/schema"
)

type OrganizationsHandler struct {
	Service *organizations.Service
	Env     string
	BaseURL string
}

func NewOrganizationsHandler(service *organizations.Service, env string, baseURL string) *OrganizationsHandler {
	return &OrganizationsHandler{Service: service, Env: env, BaseURL: baseURL}
}

func (h *OrganizationsHandler) List(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	filters, pagination, err := organizations.ParseFilters(r.URL.Query())
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	result, err := h.Service.List(r.Context(), filters, pagination)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	contextValue := loadDefaultContext()
	items := make([]*schema.Organization, 0, len(result.Organizations))
	for _, org := range result.Organizations {
		item := schema.NewOrganization(org.Name)
		item.Context = contextValue
		item.ID = schema.BuildOrganizationURI(h.BaseURL, org.ULID)
		item.Description = org.Description
		item.URL = org.URL
		item.Email = org.Email
		item.Telephone = org.Telephone
		item.Address = schema.NewPostalAddress(org.StreetAddress, org.AddressLocality, org.AddressRegion, org.PostalCode, org.AddressCountry)
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, listResponse{Items: items, NextCursor: result.NextCursor}, contentTypeFromRequest(r))
}

func (h *OrganizationsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
		return
	}

	item, err := h.Service.GetByULID(r.Context(), ulidValue)
	if err != nil {
		if errors.Is(err, organizations.ErrNotFound) {
			// Check if there's a tombstone for this organization
			tombstone, tombErr := h.Service.GetTombstoneByULID(r.Context(), ulidValue)
			if tombErr == nil && tombstone != nil {
				WriteTombstoneResponse(w, r, tombstone.Payload, tombstone.DeletedAt, "Organization")
				return
			}
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	if strings.EqualFold(item.Lifecycle, "deleted") {
		tombstone, tombErr := h.Service.GetTombstoneByULID(r.Context(), ulidValue)
		if tombErr == nil && tombstone != nil {
			WriteTombstoneResponse(w, r, tombstone.Payload, tombstone.DeletedAt, "Organization")
			return
		}

		payload := map[string]any{
			"@context":      loadDefaultContext(),
			"@type":         "Organization",
			"sel:tombstone": true,
			"sel:deletedAt": time.Now().Format(time.RFC3339),
		}
		if uri, err := ids.BuildCanonicalURI(h.BaseURL, "organizations", ulidValue); err == nil {
			payload["@id"] = uri
		}
		writeJSON(w, http.StatusGone, payload, contentTypeFromRequest(r))
		return
	}

	org := schema.NewOrganization(item.Name)
	org.Context = loadDefaultContext()
	org.ID = schema.BuildOrganizationURI(h.BaseURL, item.ULID)
	org.LegalName = item.LegalName
	org.Description = item.Description
	org.URL = item.URL
	org.Email = item.Email
	org.Telephone = item.Telephone
	org.Address = schema.NewPostalAddress(item.StreetAddress, item.AddressLocality, item.AddressRegion, item.PostalCode, item.AddressCountry)

	writeJSON(w, http.StatusOK, org, contentTypeFromRequest(r))
}
