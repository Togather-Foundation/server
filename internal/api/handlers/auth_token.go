package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/auth"
)

type TokenHandler struct {
	JWTManager *auth.JWTManager
	Env        string
}

func NewTokenHandler(jwtManager *auth.JWTManager, env string) *TokenHandler {
	return &TokenHandler{
		JWTManager: jwtManager,
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

	token, err := h.JWTManager.Generate(key.ID, "admin")
	if err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	expiresAt := time.Now().Add(h.JWTManager.Expiry())

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(tokenExchangeResponse{
		Token:     token,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
	}); err != nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, h.Env)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}
