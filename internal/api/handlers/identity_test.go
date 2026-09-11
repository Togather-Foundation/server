package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/identity"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

const (
	testULID  = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	testULID2 = "01ARZ3NDEKTSV4RRFFQ69G5FBV"
)

// --- stubs -----------------------------------------------------------------

type stubIdentityService struct {
	view         identity.IdentityView
	viewErr      error
	conflicts    identity.ConflictsResponse
	conflictsErr error
	conflictsArg identity.ConflictsParams
	decisions    identity.DecisionsResponse
	decisionsErr error
	decisionsArg identity.DecisionsParams
}

func (s *stubIdentityService) View(_ context.Context, _ identity.IdentityRef) (identity.IdentityView, error) {
	return s.view, s.viewErr
}

func (s *stubIdentityService) Conflicts(_ context.Context, arg identity.ConflictsParams) (identity.ConflictsResponse, error) {
	s.conflictsArg = arg
	return s.conflicts, s.conflictsErr
}

func (s *stubIdentityService) Decisions(_ context.Context, arg identity.DecisionsParams) (identity.DecisionsResponse, error) {
	s.decisionsArg = arg
	return s.decisions, s.decisionsErr
}

type stubIdentityWriter struct {
	linkRec      identity.DecisionRecord
	linkErr      error
	linkRef      identity.IdentityRef
	linkObs      identity.IdentifierObservation
	linkActor    string
	rejectRec    identity.DecisionRecord
	rejectErr    error
	rejectA      identity.IdentityRef
	rejectB      identity.IdentityRef
	rejectActor  string
	rejectReason string
}

func (w *stubIdentityWriter) LinkIdentifier(_ context.Context, ref identity.IdentityRef, obs identity.IdentifierObservation, actor string) (identity.DecisionRecord, error) {
	w.linkRef = ref
	w.linkObs = obs
	w.linkActor = actor
	return w.linkRec, w.linkErr
}

func (w *stubIdentityWriter) Reject(_ context.Context, a, b identity.IdentityRef, actor, reason string) (identity.DecisionRecord, error) {
	w.rejectA = a
	w.rejectB = b
	w.rejectActor = actor
	w.rejectReason = reason
	return w.rejectRec, w.rejectErr
}

func newTestIdentityHandler(svc identity.Service, w identity.Writer) *IdentityHandler {
	return &IdentityHandler{Identity: svc, Executor: w, Env: "test", ConflictLimitMax: 200}
}

func withAdminSubject(r *http.Request, subject string) {
	claims := &auth.Claims{Role: "admin", RegisteredClaims: jwt.RegisteredClaims{Subject: subject}}
	*r = *r.WithContext(middleware.ContextWithAdminClaims(r.Context(), claims))
}

func getView(t *testing.T, h *IdentityHandler, typ, id string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/identity/"+typ+"/"+id, nil)
	req.SetPathValue("type", typ)
	req.SetPathValue("id", id)
	if mutate != nil {
		mutate(req)
	}
	w := httptest.NewRecorder()
	h.GetIdentityView(w, req)
	return w
}

func postBody(t *testing.T, h *IdentityHandler, path, body string, invoke func(http.ResponseWriter, *http.Request), mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	if mutate != nil {
		mutate(req)
	}
	w := httptest.NewRecorder()
	invoke(w, req)
	return w
}

// --- GET view --------------------------------------------------------------

func TestGetIdentityView_200(t *testing.T) {
	svc := &stubIdentityService{view: identity.IdentityView{
		Ref: identity.IdentityRef{Type: identity.EntityTypePlace, ULID: testULID},
	}}
	h := newTestIdentityHandler(svc, &stubIdentityWriter{})

	w := getView(t, h, "place", testULID, nil)
	require.Equal(t, http.StatusOK, w.Code)

	var out identity.IdentityView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, testULID, out.Ref.ULID)
}

func TestGetIdentityView_400_BadULID(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{})
	w := getView(t, h, "place", "not-a-ulid", nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "https://sel.events/problems/validation-error")
}

func TestGetIdentityView_400_BadType(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{})
	w := getView(t, h, "event", testULID, nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetIdentityView_404_Absent(t *testing.T) {
	svc := &stubIdentityService{viewErr: identity.ErrEntityNotFound}
	h := newTestIdentityHandler(svc, &stubIdentityWriter{})
	w := getView(t, h, "place", testULID, nil)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "https://sel.events/problems/not-found")
}

// --- link ------------------------------------------------------------------

func TestLinkIdentifier_201_ActorFromJWT(t *testing.T) {
	writer := &stubIdentityWriter{linkRec: identity.DecisionRecord{ID: "idn-1", EntityType: identity.EntityTypePlace, EntityID: testULID, Action: identity.ActionLink}}
	h := newTestIdentityHandler(&stubIdentityService{}, writer)

	body := `{"entity_type":"place","entity_id":"` + testULID + `","authority":"artsdata","uri":"https://kg.artsdata.ca/resource/K11-24","method":"manual","confidence":1.0,"source":"manual"}`
	w := postBody(t, h, "/api/v1/admin/identity/link", body, h.LinkIdentifier, func(r *http.Request) {
		withAdminSubject(r, "test-admin")
	})
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "test-admin", writer.linkActor)
	require.Equal(t, testULID, writer.linkRef.ULID)
	require.Equal(t, "manual", writer.linkObs.Method)
}

func TestLinkIdentifier_401_NoJWT(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{})
	body := `{"entity_type":"place","entity_id":"` + testULID + `","authority":"artsdata","uri":"https://kg.artsdata.ca/resource/K11-24"}`
	w := postBody(t, h, "/api/v1/admin/identity/link", body, h.LinkIdentifier, nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Body.String(), "https://sel.events/problems/unauthorized")
}

func TestLinkIdentifier_400_BadULID(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{})
	body := `{"entity_type":"place","entity_id":"bad","authority":"artsdata","uri":"https://kg.artsdata.ca/resource/K11-24"}`
	w := postBody(t, h, "/api/v1/admin/identity/link", body, h.LinkIdentifier, func(r *http.Request) {
		withAdminSubject(r, "test-admin")
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLinkIdentifier_NormalizesMethodSourceConfidence(t *testing.T) {
	writer := &stubIdentityWriter{}
	h := newTestIdentityHandler(&stubIdentityService{}, writer)

	// auto_high method / agent source / out-of-range confidence must be
	// normalised to manual/manual and clamped.
	body := `{"entity_type":"place","entity_id":"` + testULID + `","authority":"artsdata","uri":"https://kg.artsdata.ca/resource/K11-24","method":"auto_high","confidence":2.5,"source":"agent"}`
	w := postBody(t, h, "/api/v1/admin/identity/link", body, h.LinkIdentifier, func(r *http.Request) {
		withAdminSubject(r, "test-admin")
	})
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "manual", writer.linkObs.Method)
	require.Equal(t, "manual", writer.linkObs.Source)
	require.Equal(t, 1.0, writer.linkObs.Confidence)
}

// --- reject ----------------------------------------------------------------

func TestReject_201_ActorFromJWT(t *testing.T) {
	writer := &stubIdentityWriter{rejectRec: identity.DecisionRecord{ID: "idn-2", Action: identity.ActionReject}}
	h := newTestIdentityHandler(&stubIdentityService{}, writer)

	body := `{"entity_type":"place","entity_id":"` + testULID + `","counterpart_id":"` + testULID2 + `","reason":"different address"}`
	w := postBody(t, h, "/api/v1/admin/identity/reject", body, h.Reject, func(r *http.Request) {
		withAdminSubject(r, "test-admin")
	})
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "test-admin", writer.rejectActor)
	require.Equal(t, testULID, writer.rejectA.ULID)
	require.Equal(t, testULID2, writer.rejectB.ULID)
	require.Equal(t, "different address", writer.rejectReason)
}

func TestReject_401_NoJWT(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{})
	body := `{"entity_type":"place","entity_id":"` + testULID + `","counterpart_id":"` + testULID2 + `"}`
	w := postBody(t, h, "/api/v1/admin/identity/reject", body, h.Reject, nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestReject_400_BadULID(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{})
	body := `{"entity_type":"place","entity_id":"` + testULID + `","counterpart_id":"bad"}`
	w := postBody(t, h, "/api/v1/admin/identity/reject", body, h.Reject, func(r *http.Request) {
		withAdminSubject(r, "test-admin")
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLinkIdentifier_404_AbsentEntity(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{linkErr: identity.ErrEntityNotFound})
	body := `{"entity_type":"place","entity_id":"` + testULID + `","authority":"artsdata","uri":"https://kg.artsdata.ca/resource/K11-24"}`
	w := postBody(t, h, "/api/v1/admin/identity/link", body, h.LinkIdentifier, func(r *http.Request) {
		withAdminSubject(r, "test-admin")
	})
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "https://sel.events/problems/not-found")
}

func TestReject_404_AbsentCounterpart(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{rejectErr: identity.ErrEntityNotFound})
	body := `{"entity_type":"place","entity_id":"` + testULID + `","counterpart_id":"` + testULID2 + `"}`
	w := postBody(t, h, "/api/v1/admin/identity/reject", body, h.Reject, func(r *http.Request) {
		withAdminSubject(r, "test-admin")
	})
	require.Equal(t, http.StatusNotFound, w.Code)
}

// --- conflicts / decisions -------------------------------------------------

func TestListConflicts_200(t *testing.T) {
	svc := &stubIdentityService{conflicts: identity.ConflictsResponse{
		Items: []identity.ConflictItem{{Authority: "artsdata"}},
	}}
	h := newTestIdentityHandler(svc, &stubIdentityWriter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/identity/conflicts?type=place", nil)
	w := httptest.NewRecorder()
	h.ListConflicts(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var out identity.ConflictsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Items, 1)
}

func TestListConflicts_400_MissingType(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/identity/conflicts", nil)
	w := httptest.NewRecorder()
	h.ListConflicts(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListConflicts_ForwardsParams(t *testing.T) {
	svc := &stubIdentityService{}
	h := newTestIdentityHandler(svc, &stubIdentityWriter{})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/identity/conflicts?type=place&limit=25&cursor=abc&include_suppressed=true", nil)
	w := httptest.NewRecorder()
	h.ListConflicts(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.Equal(t, identity.EntityTypePlace, svc.conflictsArg.Type)
	require.Equal(t, 25, svc.conflictsArg.Limit)
	require.Equal(t, "abc", svc.conflictsArg.Cursor)
	require.True(t, svc.conflictsArg.IncludeSuppressed)
}

func TestListConflicts_400_BadLimit(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{})
	for _, q := range []string{"limit=abc", "limit=0", "limit=201"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/identity/conflicts?type=place&"+q, nil)
		w := httptest.NewRecorder()
		h.ListConflicts(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code, "limit=%s must be rejected", q)
	}
}

func TestListConflicts_400_InvalidCursor(t *testing.T) {
	svc := &stubIdentityService{conflictsErr: identity.ErrInvalidCursor}
	h := newTestIdentityHandler(svc, &stubIdentityWriter{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/identity/conflicts?type=place&cursor=bad", nil)
	w := httptest.NewRecorder()
	h.ListConflicts(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListDecisions_200(t *testing.T) {
	svc := &stubIdentityService{decisions: identity.DecisionsResponse{
		Items: []identity.DecisionRecord{{ID: "idn-1"}},
	}}
	h := newTestIdentityHandler(svc, &stubIdentityWriter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/identity/decisions", nil)
	w := httptest.NewRecorder()
	h.ListDecisions(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var out identity.DecisionsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Items, 1)
}

func TestListDecisions_ForwardsParams(t *testing.T) {
	svc := &stubIdentityService{}
	h := newTestIdentityHandler(svc, &stubIdentityWriter{})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/identity/decisions?type=organization&since=2026-01-01T00:00:00Z&limit=10&cursor=xyz", nil)
	w := httptest.NewRecorder()
	h.ListDecisions(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.NotNil(t, svc.decisionsArg.Type)
	require.Equal(t, identity.EntityTypeOrganization, *svc.decisionsArg.Type)
	require.NotNil(t, svc.decisionsArg.Since)
	require.Equal(t, 10, svc.decisionsArg.Limit)
	require.Equal(t, "xyz", svc.decisionsArg.Cursor)
}

func TestListDecisions_400_BadSince(t *testing.T) {
	h := newTestIdentityHandler(&stubIdentityService{}, &stubIdentityWriter{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/identity/decisions?since=not-a-date", nil)
	w := httptest.NewRecorder()
	h.ListDecisions(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListDecisions_400_InvalidCursor(t *testing.T) {
	svc := &stubIdentityService{decisionsErr: identity.ErrInvalidCursor}
	h := newTestIdentityHandler(svc, &stubIdentityWriter{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/identity/decisions?cursor=bad", nil)
	w := httptest.NewRecorder()
	h.ListDecisions(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestIdentity_ContentType verifies the admin identity endpoints always emit
// application/json (never JSON-LD), matching the OpenAPI contract.
func TestIdentity_ContentType(t *testing.T) {
	svc := &stubIdentityService{view: identity.IdentityView{}}
	h := newTestIdentityHandler(svc, &stubIdentityWriter{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/identity/place/"+testULID, nil)
	req.Header.Set("Accept", "application/ld+json")
	req.SetPathValue("type", "place")
	req.SetPathValue("id", testULID)
	w := httptest.NewRecorder()
	h.GetIdentityView(w, req)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
}
