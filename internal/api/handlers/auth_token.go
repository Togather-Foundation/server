package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/auth"
)

type TokenHandler struct {
	JWTManager *auth.JWTManager
	JWTExpiry  time.Duration
	Env        string
}

func NewTokenHandler(jwtManager *auth.JWTManager, jwtExpiry time.Duration, env string) *TokenHandler {
	return &TokenHandler{
		JWTManager: jwtManager,
		JWTExpiry:  jwtExpiry,
		Env:        env,
	}
}

type tokenExchangeResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func (h *TokenHandler) Exchange(w http.ResponseWriter, r *http.Request) {
	key := middleware.AgentKey(r)
	if key == nil {
		problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Unauthorized", problem.ErrUnauthorized, h.Env)
		return
	}

	if !auth.IsAdmin(key.Role) {
		problem.Write(w, r, http.StatusForbidden, "https://sel.events/problems/forbidden", "Admin API key required", problem.ErrForbidden, h.Env)
		return
	}

	if h.JWTManager == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	token, err := h.JWTManager.Generate(key.Name, "admin")
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	expiresAt := time.Now().Add(h.JWTExpiry)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(tokenExchangeResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
