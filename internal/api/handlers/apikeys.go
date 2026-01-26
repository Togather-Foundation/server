package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

type APIKeyHandler struct {
	Queries *postgres.Queries
	Env     string
}

func NewAPIKeyHandler(queries *postgres.Queries, env string) *APIKeyHandler {
	return &APIKeyHandler{
		Queries: queries,
		Env:     env,
	}
}

type CreateAPIKeyRequest struct {
	Name          string  `json:"name"`
	SourceID      *string `json:"source_id,omitempty"`
	Role          string  `json:"role"`
	RateLimitTier string  `json:"rate_limit_tier"`
	ExpiresInDays *int    `json:"expires_in_days,omitempty"`
}

type CreateAPIKeyResponse struct {
	APIKey    string     `json:"api_key"` // Only shown once!
	Prefix    string     `json:"prefix"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateAPIKey handles POST /api/v1/admin/api-keys
func (h *APIKeyHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.Env)
		return
	}

	// Validate required fields
	if req.Name == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Name is required", nil, h.Env)
		return
	}
	if req.Role == "" {
		req.Role = "agent" // Default role
	}
	if req.RateLimitTier == "" {
		req.RateLimitTier = "agent" // Default tier
	}

	// Generate API key (32 bytes = 256 bits)
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to generate key", err, h.Env)
		return
	}
	apiKey := base64.URLEncoding.EncodeToString(keyBytes)
	prefix := apiKey[:8]

	// Hash the key using bcrypt
	keyHash, err := auth.HashAPIKey(apiKey)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to hash key", err, h.Env)
		return
	}

	// Calculate expiration
	var expiresAt *time.Time
	if req.ExpiresInDays != nil && *req.ExpiresInDays > 0 {
		exp := time.Now().AddDate(0, 0, *req.ExpiresInDays)
		expiresAt = &exp
	}

	// Store in database
	var sourceIDParam pgtype.UUID
	if req.SourceID != nil {
		if err := sourceIDParam.Scan(*req.SourceID); err != nil {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid source ID", err, h.Env)
			return
		}
	}

	var expiresAtParam pgtype.Timestamptz
	if expiresAt != nil {
		expiresAtParam = pgtype.Timestamptz{Time: *expiresAt, Valid: true}
	}

	key, err := h.Queries.CreateAPIKey(r.Context(), postgres.CreateAPIKeyParams{
		Prefix:        prefix,
		KeyHash:       keyHash,
		HashVersion:   int32(auth.HashVersionBcrypt),
		Name:          req.Name,
		SourceID:      sourceIDParam,
		Role:          req.Role,
		RateLimitTier: req.RateLimitTier,
		IsActive:      true,
		ExpiresAt:     expiresAtParam,
	})
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to create API key", err, h.Env)
		return
	}

	resp := CreateAPIKeyResponse{
		APIKey:    apiKey,
		Prefix:    prefix,
		Name:      key.Name,
		Role:      key.Role,
		ExpiresAt: expiresAt,
	}

	writeJSON(w, http.StatusCreated, resp, "application/json")
}

// ListAPIKeys handles GET /api/v1/admin/api-keys
func (h *APIKeyHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.Queries.ListAPIKeys(r.Context())
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to list API keys", err, h.Env)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": keys}, "application/json")
}

// RevokeAPIKey handles DELETE /api/v1/admin/api-keys/{id}
func (h *APIKeyHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	if id == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing ID", nil, h.Env)
		return
	}

	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid UUID", err, h.Env)
		return
	}

	err := h.Queries.DeactivateAPIKey(r.Context(), pgUUID)
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Failed to revoke API key", err, h.Env)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
