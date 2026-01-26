package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/federation"
	"github.com/google/uuid"
)

type FederationHandler struct {
	Service *federation.Service
	Env     string
}

func NewFederationHandler(service *federation.Service, env string) *FederationHandler {
	return &FederationHandler{
		Service: service,
		Env:     env,
	}
}

// CreateNode handles POST /api/v1/admin/federation/nodes
func (h *FederationHandler) CreateNode(w http.ResponseWriter, r *http.Request) {
	var req federation.CreateNodeParams
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	node, err := h.Service.CreateNode(r.Context(), req)
	if err != nil {
		if errors.Is(err, federation.ErrDuplicateDomain) {
			problem.Write(w, r, http.StatusConflict, "https://sel.events/problems/duplicate", "Domain already exists", err, h.Env)
			return
		}
		if errors.Is(err, federation.ErrInvalidNodeParams) {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid parameters", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	writeJSON(w, http.StatusCreated, node, "application/json")
}

// ListNodes handles GET /api/v1/admin/federation/nodes
func (h *FederationHandler) ListNodes(w http.ResponseWriter, r *http.Request) {
	filters := federation.ListNodesFilters{
		FederationStatus: r.URL.Query().Get("status"),
		Limit:            50,
	}

	if syncEnabled := r.URL.Query().Get("sync_enabled"); syncEnabled != "" {
		enabled := syncEnabled == "true"
		filters.SyncEnabled = &enabled
	}

	if isOnline := r.URL.Query().Get("is_online"); isOnline != "" {
		online := isOnline == "true"
		filters.IsOnline = &online
	}

	nodes, err := h.Service.ListNodes(r.Context(), filters)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": nodes}, "application/json")
}

// GetNode handles GET /api/v1/admin/federation/nodes/{id}
func (h *FederationHandler) GetNode(w http.ResponseWriter, r *http.Request) {
	idStr := pathParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid ID", err, h.Env)
		return
	}

	node, err := h.Service.GetNode(r.Context(), id)
	if err != nil {
		if errors.Is(err, federation.ErrNodeNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Node not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	writeJSON(w, http.StatusOK, node, "application/json")
}

// UpdateNode handles PUT /api/v1/admin/federation/nodes/{id}
func (h *FederationHandler) UpdateNode(w http.ResponseWriter, r *http.Request) {
	idStr := pathParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid ID", err, h.Env)
		return
	}

	var params federation.UpdateNodeParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	node, err := h.Service.UpdateNode(r.Context(), id, params)
	if err != nil {
		if errors.Is(err, federation.ErrNodeNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Node not found", err, h.Env)
			return
		}
		if errors.Is(err, federation.ErrInvalidNodeParams) {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid parameters", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	writeJSON(w, http.StatusOK, node, "application/json")
}

// DeleteNode handles DELETE /api/v1/admin/federation/nodes/{id}
func (h *FederationHandler) DeleteNode(w http.ResponseWriter, r *http.Request) {
	idStr := pathParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid ID", err, h.Env)
		return
	}

	err = h.Service.DeleteNode(r.Context(), id)
	if err != nil {
		if errors.Is(err, federation.ErrNodeNotFound) {
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Node not found", err, h.Env)
			return
		}
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
