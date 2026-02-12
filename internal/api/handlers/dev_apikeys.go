package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// DeveloperAPIKeyHandler handles developer API key management endpoints
type DeveloperAPIKeyHandler struct {
	service     *developers.Service
	logger      zerolog.Logger
	env         string
	auditLogger AuditLogger
}

// NewDeveloperAPIKeyHandler creates a new developer API key handler
func NewDeveloperAPIKeyHandler(
	service *developers.Service,
	logger zerolog.Logger,
	env string,
	auditLogger AuditLogger,
) *DeveloperAPIKeyHandler {
	return &DeveloperAPIKeyHandler{
		service:     service,
		logger:      logger.With().Str("handler", "dev_apikeys").Logger(),
		env:         env,
		auditLogger: auditLogger,
	}
}

type createDevAPIKeyRequest struct {
	Name          string `json:"name"`
	ExpiresInDays *int   `json:"expires_in_days,omitempty"`
}

type createDevAPIKeyResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Prefix    string  `json:"prefix"`
	Key       string  `json:"key"` // Only shown once!
	Role      string  `json:"role"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt *string `json:"expires_at,omitempty"`
	Warning   string  `json:"warning"`
}

type apiKeyListResponse struct {
	Items    []apiKeyInfo `json:"items"`
	MaxKeys  int          `json:"max_keys"`
	KeyCount int          `json:"key_count"`
}

type apiKeyInfo struct {
	ID            string  `json:"id"`
	Prefix        string  `json:"prefix"`
	Name          string  `json:"name"`
	Role          string  `json:"role"`
	RateLimitTier string  `json:"rate_limit_tier"`
	IsActive      bool    `json:"is_active"`
	CreatedAt     string  `json:"created_at"`
	LastUsedAt    *string `json:"last_used_at,omitempty"`
	ExpiresAt     *string `json:"expires_at,omitempty"`
	UsageToday    int64   `json:"usage_today"`
	Usage7d       int64   `json:"usage_7d"`
	Usage30d      int64   `json:"usage_30d"`
}

type usageStatsResponse struct {
	APIKeyID      string       `json:"api_key_id"`
	Period        periodInfo   `json:"period"`
	TotalRequests int64        `json:"total_requests"`
	TotalErrors   int64        `json:"total_errors"`
	Daily         []dailyUsage `json:"daily"`
}

type periodInfo struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type dailyUsage struct {
	Date     string `json:"date"`
	Requests int64  `json:"requests"`
	Errors   int64  `json:"errors"`
}

// ListAPIKeys handles GET /api/v1/dev/api-keys
// Lists all API keys owned by the authenticated developer with usage summary
func (h *DeveloperAPIKeyHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	claims := middleware.DeveloperClaims(r)
	if claims == nil {
		problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Unauthorized", nil, h.env)
		return
	}

	// Get developer ID from claims
	developerID := claims.DeveloperID

	// List all keys for this developer
	keys, err := h.service.ListOwnKeys(r.Context(), developerID)
	if err != nil {
		h.logger.Error().Err(err).Str("developer_id", developerID.String()).Msg("failed to list API keys")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.env)
		return
	}

	// Convert to response format
	items := make([]apiKeyInfo, 0, len(keys))
	for _, key := range keys {
		var lastUsed *string
		if key.LastUsedAt != nil {
			ts := key.LastUsedAt.Format(time.RFC3339)
			lastUsed = &ts
		}

		var expires *string
		if key.ExpiresAt != nil {
			ts := key.ExpiresAt.Format(time.RFC3339)
			expires = &ts
		}

		items = append(items, apiKeyInfo{
			ID:            key.ID.String(),
			Prefix:        key.Prefix,
			Name:          key.Name,
			Role:          key.Role,
			RateLimitTier: key.RateLimitTier,
			IsActive:      key.IsActive,
			CreatedAt:     key.CreatedAt.Format(time.RFC3339),
			LastUsedAt:    lastUsed,
			ExpiresAt:     expires,
			UsageToday:    key.UsageToday,
			Usage7d:       key.Usage7d,
			Usage30d:      key.Usage30d,
		})
	}

	resp := apiKeyListResponse{
		Items:    items,
		MaxKeys:  5, // Default max keys per developer
		KeyCount: len(keys),
	}

	writeJSON(w, http.StatusOK, resp, "application/json")
}

// CreateAPIKey handles POST /api/v1/dev/api-keys
// Creates a new API key for the authenticated developer (enforces max_keys limit, always role=agent)
func (h *DeveloperAPIKeyHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	claims := middleware.DeveloperClaims(r)
	if claims == nil {
		problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Unauthorized", nil, h.env)
		return
	}

	var req createDevAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, h.env)
		return
	}

	if req.Name == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Name is required", nil, h.env)
		return
	}

	// Create API key
	developerID := claims.DeveloperID
	params := developers.CreateAPIKeyParams{
		DeveloperID:   developerID,
		Name:          req.Name,
		ExpiresInDays: req.ExpiresInDays,
	}

	plainKey, keyInfo, err := h.service.CreateAPIKey(r.Context(), params)
	if err != nil {
		// Map domain errors to HTTP status codes
		switch {
		case errors.Is(err, developers.ErrMaxKeysReached):
			problem.Write(w, r, http.StatusForbidden, "https://sel.events/problems/max-keys-reached", "Maximum number of API keys reached", nil, h.env,
				problem.WithDetail("You have reached your maximum number of API keys. Please revoke an existing key before creating a new one."))
			return

		case errors.Is(err, developers.ErrDeveloperNotFound):
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Developer not found", nil, h.env)
			return

		default:
			h.logger.Error().Err(err).Str("developer_id", developerID.String()).Msg("failed to create API key")
			problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.env)
			return
		}
	}

	// Log successful key creation
	if h.auditLogger != nil {
		clientIP := extractClientIP(r)
		h.auditLogger.LogSuccess("dev.api_key_created", claims.Email, "api_key", keyInfo.ID.String(), clientIP, map[string]string{
			"key_name": req.Name,
		})
	}

	// Build response
	var expiresAtStr *string
	if keyInfo.ExpiresAt != nil {
		ts := keyInfo.ExpiresAt.Format(time.RFC3339)
		expiresAtStr = &ts
	}

	resp := createDevAPIKeyResponse{
		ID:        keyInfo.ID.String(),
		Name:      keyInfo.Name,
		Prefix:    keyInfo.Prefix,
		Key:       plainKey,
		Role:      keyInfo.Role,
		CreatedAt: keyInfo.CreatedAt.Format(time.RFC3339),
		ExpiresAt: expiresAtStr,
		Warning:   "IMPORTANT: Save this API key now. You won't be able to see it again. Store it securely.",
	}

	writeJSON(w, http.StatusCreated, resp, "application/json")
}

// RevokeAPIKey handles DELETE /api/v1/dev/api-keys/{id}
// Revokes an API key owned by the authenticated developer (verifies ownership)
func (h *DeveloperAPIKeyHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	claims := middleware.DeveloperClaims(r)
	if claims == nil {
		problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Unauthorized", nil, h.env)
		return
	}

	keyIDStr := pathParam(r, "id")
	if keyIDStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing key ID", nil, h.env)
		return
	}

	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid key ID", err, h.env)
		return
	}

	// Revoke the key
	developerID := claims.DeveloperID
	if err := h.service.RevokeOwnKey(r.Context(), developerID, keyID); err != nil {
		// Map domain errors to HTTP status codes
		switch {
		case errors.Is(err, developers.ErrAPIKeyNotFound):
			problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "API key not found", nil, h.env)
			return

		case errors.Is(err, developers.ErrUnauthorized):
			problem.Write(w, r, http.StatusForbidden, "https://sel.events/problems/forbidden", "You do not own this API key", nil, h.env)
			return

		default:
			h.logger.Error().Err(err).Str("developer_id", developerID.String()).Str("key_id", keyID.String()).Msg("failed to revoke API key")
			problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.env)
			return
		}
	}

	// Log successful revocation
	if h.auditLogger != nil {
		clientIP := extractClientIP(r)
		h.auditLogger.LogSuccess("dev.api_key_revoked", claims.Email, "api_key", keyID.String(), clientIP, nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAPIKeyUsage handles GET /api/v1/dev/api-keys/{id}/usage
// Returns usage statistics for an API key owned by the authenticated developer
func (h *DeveloperAPIKeyHandler) GetAPIKeyUsage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.DeveloperClaims(r)
	if claims == nil {
		problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Unauthorized", nil, h.env)
		return
	}

	keyIDStr := pathParam(r, "id")
	if keyIDStr == "" {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Missing key ID", nil, h.env)
		return
	}

	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid key ID", err, h.env)
		return
	}

	// Parse query parameters for date range
	now := time.Now()
	defaultFrom := now.AddDate(0, 0, -30) // Last 30 days by default

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var from, to time.Time
	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid 'from' date format (expected YYYY-MM-DD)", err, h.env)
			return
		}
	} else {
		from = defaultFrom
	}

	if toStr != "" {
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid 'to' date format (expected YYYY-MM-DD)", err, h.env)
			return
		}
	} else {
		to = now
	}

	// Verify ownership of the key
	developerID := claims.DeveloperID
	keys, err := h.service.ListOwnKeys(r.Context(), developerID)
	if err != nil {
		h.logger.Error().Err(err).Str("developer_id", developerID.String()).Msg("failed to list API keys")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.env)
		return
	}

	// Check if the key belongs to this developer
	keyFound := false
	for _, key := range keys {
		if key.ID == keyID {
			keyFound = true
			break
		}
	}
	if !keyFound {
		problem.Write(w, r, http.StatusForbidden, "https://sel.events/problems/forbidden", "You do not own this API key", nil, h.env)
		return
	}

	// Get usage statistics
	stats, err := h.service.GetUsageStats(r.Context(), developerID, from, to)
	if err != nil {
		h.logger.Error().Err(err).Str("developer_id", developerID.String()).Str("key_id", keyID.String()).Msg("failed to get usage stats")
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.env)
		return
	}

	// Build daily breakdown (placeholder - actual implementation would query daily stats)
	// For now, we'll return a simplified response
	daily := make([]dailyUsage, 0)

	// TODO: Implement daily breakdown by querying api_key_usage table grouped by day
	// For now, return total only
	totalDays := int(to.Sub(from).Hours() / 24)
	if totalDays > 0 {
		avgPerDay := stats.TotalRequests / int64(totalDays)
		avgErrorsPerDay := stats.TotalErrors / int64(totalDays)

		// Generate placeholder daily data (in production, query actual daily data)
		current := from
		for current.Before(to) || current.Equal(to) {
			daily = append(daily, dailyUsage{
				Date:     current.Format("2006-01-02"),
				Requests: avgPerDay,
				Errors:   avgErrorsPerDay,
			})
			current = current.AddDate(0, 0, 1)
		}
	}

	resp := usageStatsResponse{
		APIKeyID: keyID.String(),
		Period: periodInfo{
			From: from.Format(time.RFC3339),
			To:   to.Format(time.RFC3339),
		},
		TotalRequests: stats.TotalRequests,
		TotalErrors:   stats.TotalErrors,
		Daily:         daily,
	}

	writeJSON(w, http.StatusOK, resp, "application/json")
}
