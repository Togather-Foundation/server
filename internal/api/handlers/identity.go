package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/Togather-Foundation/server/internal/identity"
)

// defaultIdentityLimit is the default page size for the identity read feeds.
const defaultIdentityLimit = 50

// IdentityHandler serves the admin identity read + link/reject surface.
type IdentityHandler struct {
	Identity         identity.Service
	Executor         identity.Writer
	Env              string
	ConflictLimitMax int
}

// NewIdentityHandler assembles an IdentityHandler from its collaborators.
func NewIdentityHandler(identitySvc identity.Service, executor identity.Writer, env string, conflictLimitMax int) *IdentityHandler {
	if conflictLimitMax <= 0 {
		conflictLimitMax = defaultIdentityLimit
	}
	return &IdentityHandler{
		Identity:         identitySvc,
		Executor:         executor,
		Env:              env,
		ConflictLimitMax: conflictLimitMax,
	}
}

// GetIdentityView handles GET /api/v1/admin/identity/{type}/{id}.
func (h *IdentityHandler) GetIdentityView(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Identity == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	typ, ok := parseIdentityType(pathParam(r, "type"))
	if !ok {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid entity type", nil, h.Env)
		return
	}
	ulidValue, ok := ValidateAndExtractULID(w, r, "id", h.Env)
	if !ok {
		return
	}

	view, err := h.Identity.View(r.Context(), identity.IdentityRef{Type: typ, ULID: ulidValue})
	if err != nil {
		writeIdentityError(w, r, err, h.Env)
		return
	}
	writeJSON(w, http.StatusOK, view, "application/json")
}

// ListConflicts handles GET /api/v1/admin/identity/conflicts.
func (h *IdentityHandler) ListConflicts(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Identity == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	q := r.URL.Query()
	typ, ok := parseIdentityType(q.Get("type"))
	if !ok {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid or missing entity type", nil, h.Env)
		return
	}

	limit, err := parseIdentityLimit(q, h.ConflictLimitMax)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid limit", err, h.Env)
		return
	}

	resp, err := h.Identity.Conflicts(r.Context(), identity.ConflictsParams{
		Type:              typ,
		Limit:             limit,
		Cursor:            q.Get("cursor"),
		IncludeSuppressed: parseIdentityBool(q, "include_suppressed"),
	})
	if err != nil {
		writeIdentityError(w, r, err, h.Env)
		return
	}
	writeJSON(w, http.StatusOK, resp, "application/json")
}

// ListDecisions handles GET /api/v1/admin/identity/decisions.
func (h *IdentityHandler) ListDecisions(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Identity == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	q := r.URL.Query()
	limit, err := parseIdentityLimit(q, h.ConflictLimitMax)
	if err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid limit", err, h.Env)
		return
	}
	params := identity.DecisionsParams{
		Limit:  limit,
		Cursor: q.Get("cursor"),
	}

	if t := q.Get("type"); t != "" {
		typ, ok := parseIdentityType(t)
		if !ok {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid entity type", nil, h.Env)
			return
		}
		params.Type = &typ
	}

	if s := q.Get("since"); s != "" {
		since, err := time.Parse(time.RFC3339, s)
		if err != nil {
			problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "since must be an RFC3339 timestamp", err, h.Env)
			return
		}
		params.Since = &since
	}

	resp, err := h.Identity.Decisions(r.Context(), params)
	if err != nil {
		writeIdentityError(w, r, err, h.Env)
		return
	}
	writeJSON(w, http.StatusOK, resp, "application/json")
}

// LinkIdentifier handles POST /api/v1/admin/identity/link.
func (h *IdentityHandler) LinkIdentifier(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Executor == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	actor, ok := actorFromRequest(w, r, h.Env)
	if !ok {
		return
	}

	var req struct {
		EntityType string   `json:"entity_type"`
		EntityID   string   `json:"entity_id"`
		Authority  string   `json:"authority"`
		URI        string   `json:"uri"`
		Method     string   `json:"method"`
		Confidence *float64 `json:"confidence"`
		Source     string   `json:"source"`
		Rationale  string   `json:"rationale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.Env)
		return
	}

	typ, ok := parseIdentityType(req.EntityType)
	if !ok {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid entity_type", nil, h.Env)
		return
	}
	if err := ids.ValidateULID(req.EntityID); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid entity_id", err, h.Env)
		return
	}

	// Phase 1 allow-list: an agent cannot assert auto_high/auto_low; any
	// client-supplied method/source other than "manual" is normalised to manual.
	method := req.Method
	if method != "manual" {
		method = "manual"
	}
	source := req.Source
	if source != "manual" {
		source = "manual"
	}
	confidence := 1.0
	if req.Confidence != nil {
		confidence = *req.Confidence
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	rec, err := h.Executor.LinkIdentifier(r.Context(), identity.IdentityRef{Type: typ, ULID: req.EntityID}, identity.IdentifierObservation{
		Authority:  req.Authority,
		URI:        req.URI,
		Method:     method,
		Confidence: confidence,
		Source:     source,
	}, actor)
	if err != nil {
		writeIdentityError(w, r, err, h.Env)
		return
	}
	writeJSON(w, http.StatusCreated, rec, "application/json")
}

// Reject handles POST /api/v1/admin/identity/reject.
func (h *IdentityHandler) Reject(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Executor == nil {
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", nil, h.Env)
		return
	}

	actor, ok := actorFromRequest(w, r, h.Env)
	if !ok {
		return
	}

	var req struct {
		EntityType    string `json:"entity_type"`
		EntityID      string `json:"entity_id"`
		CounterpartID string `json:"counterpart_id"`
		Reason        string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request body", err, h.Env)
		return
	}

	typ, ok := parseIdentityType(req.EntityType)
	if !ok {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid entity_type", nil, h.Env)
		return
	}
	if err := ids.ValidateULID(req.EntityID); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid entity_id", err, h.Env)
		return
	}
	if err := ids.ValidateULID(req.CounterpartID); err != nil {
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid counterpart_id", err, h.Env)
		return
	}

	rec, err := h.Executor.Reject(r.Context(),
		identity.IdentityRef{Type: typ, ULID: req.EntityID},
		identity.IdentityRef{Type: typ, ULID: req.CounterpartID},
		actor, req.Reason)
	if err != nil {
		writeIdentityError(w, r, err, h.Env)
		return
	}
	writeJSON(w, http.StatusCreated, rec, "application/json")
}

// actorFromRequest extracts the admin JWT subject. The actor is always taken
// from the JWT, never the request body.
func actorFromRequest(w http.ResponseWriter, r *http.Request, env string) (string, bool) {
	claims := middleware.AdminClaims(r)
	if claims == nil {
		problem.Write(w, r, http.StatusUnauthorized, "https://sel.events/problems/unauthorized", "Unauthorized", problem.ErrUnauthorized, env)
		return "", false
	}
	return claims.Subject, true
}

// parseIdentityType validates an entity type string against the Phase 1 set.
func parseIdentityType(s string) (identity.EntityType, bool) {
	switch identity.EntityType(s) {
	case identity.EntityTypePlace, identity.EntityTypeOrganization:
		return identity.EntityType(s), true
	default:
		return "", false
	}
}

// parseIdentityLimit parses and validates the `limit` query parameter. A
// missing value yields the default; a non-numeric or out-of-range value is a
// structural (400) error.
func parseIdentityLimit(q map[string][]string, max int) (int, error) {
	v := ""
	if vals, ok := q["limit"]; ok && len(vals) > 0 {
		v = vals[0]
	}
	if v == "" {
		return defaultIdentityLimit, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer, got %q", v)
	}
	if n < 1 || n > max {
		return 0, fmt.Errorf("limit must be between 1 and %d, got %d", max, n)
	}
	return n, nil
}

// parseIdentityBool parses a boolean query parameter (default false).
func parseIdentityBool(q map[string][]string, key string) bool {
	vals, ok := q[key]
	if !ok || len(vals) == 0 {
		return false
	}
	b, err := strconv.ParseBool(vals[0])
	if err != nil {
		return false
	}
	return b
}

// writeIdentityError maps identity domain errors to RFC 7807 responses.
func writeIdentityError(w http.ResponseWriter, r *http.Request, err error, env string) {
	switch {
	case errors.Is(err, identity.ErrEntityNotFound):
		problem.Write(w, r, http.StatusNotFound, "https://sel.events/problems/not-found", "Entity not found", err, env)
	case errors.Is(err, identity.ErrInvalidEntityType),
		errors.Is(err, identity.ErrInvalidCursor),
		errors.Is(err, identity.ErrEmptyActor),
		errors.Is(err, identity.ErrInvalidURI),
		errors.Is(err, identity.ErrUnknownAuthority),
		errors.Is(err, identity.ErrMismatchedEntityTypes),
		errors.Is(err, identity.ErrSameEntityPair):
		problem.Write(w, r, http.StatusBadRequest, "https://sel.events/problems/validation-error", "Invalid request", err, env)
	default:
		problem.Write(w, r, http.StatusInternalServerError, "https://sel.events/problems/server-error", "Server error", err, env)
	}
}
