package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// newTestAdminHandler returns a minimal AdminHandler wired up with the given
// MockRepository (used to construct a real AdminService).
func newTestAdminHandler(repo *MockRepository) *AdminHandler {
	adminService := events.NewAdminService(
		repo,
		false, // requireHTTPS off for tests
		"America/Toronto",
		config.ValidationConfig{AllowTestDomains: true},
		"https://toronto.togather.foundation",
	)
	return &AdminHandler{
		AdminService: adminService,
		AuditLogger:  audit.NewLogger(),
		Env:          "test",
		BaseURL:      "https://toronto.togather.foundation",
	}
}

// consolidateBody marshals a consolidate request body into a reader.
func consolidateBody(t *testing.T, payload any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("consolidateBody: marshal: %v", err)
	}
	return bytes.NewReader(b)
}

// ---------------------------------------------------------------------------
// 400 – both event and event_ulid supplied
// ---------------------------------------------------------------------------

func TestConsolidate_400_BothEventFields(t *testing.T) {
	repo := new(MockRepository)
	h := newTestAdminHandler(repo)

	body := consolidateBody(t, map[string]any{
		"event":      map[string]any{"name": "My Event"},
		"event_ulid": "01HTEST0000000000000000001",
		"retire":     []string{"01HTEST0000000000000000002"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/consolidate", body)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()

	h.ConsolidateEvents(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "expected 400")
	repo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// 400 – neither event nor event_ulid supplied
// ---------------------------------------------------------------------------

func TestConsolidate_400_NoEventField(t *testing.T) {
	repo := new(MockRepository)
	h := newTestAdminHandler(repo)

	body := consolidateBody(t, map[string]any{
		"retire": []string{"01HTEST0000000000000000002"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/consolidate", body)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()

	h.ConsolidateEvents(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "expected 400")
	repo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// 400 – retire list empty
// ---------------------------------------------------------------------------

func TestConsolidate_400_EmptyRetire(t *testing.T) {
	repo := new(MockRepository)
	h := newTestAdminHandler(repo)

	body := consolidateBody(t, map[string]any{
		"event_ulid": "01HTEST0000000000000000001",
		"retire":     []string{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/consolidate", body)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()

	h.ConsolidateEvents(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "expected 400")
	repo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// 400 – canonical ULID is also in the retire list
// ---------------------------------------------------------------------------

func TestConsolidate_400_CanonicalInRetire(t *testing.T) {
	repo := new(MockRepository)
	h := newTestAdminHandler(repo)

	body := consolidateBody(t, map[string]any{
		"event_ulid": "01HTEST0000000000000000001",
		"retire":     []string{"01HTEST0000000000000000001"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/consolidate", body)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()

	h.ConsolidateEvents(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "expected 400")
	repo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// 200 – promote path: event_ulid + retire list → success
// ---------------------------------------------------------------------------

func TestConsolidate_200_PromotePath(t *testing.T) {
	const canonicalULID = "01HTEST0000000000000000001"
	const retireULID = "01HTEST0000000000000000002"

	repo := new(MockRepository)

	// Consolidate() begins a transaction.
	setupTxMock(repo)

	// Step 3: lock retired event (GetByULID × 2, LockEventForUpdate).
	retiredEvent := &events.Event{
		ID:             "uuid-retire",
		ULID:           retireULID,
		Name:           "Retire Me",
		LifecycleState: "published",
	}
	repo.On("GetByULID", mock.Anything, retireULID).Return(retiredEvent, nil).Times(2)
	repo.On("LockEventForUpdate", mock.Anything, "uuid-retire").Return(nil).Once()

	// Step 4: promote path — get & lock canonical.
	canonicalEvent := &events.Event{
		ID:             "uuid-canon",
		ULID:           canonicalULID,
		Name:           "Canonical Event",
		LifecycleState: "published",
	}
	repo.On("GetByULID", mock.Anything, canonicalULID).Return(canonicalEvent, nil).Times(2)
	repo.On("LockEventForUpdate", mock.Anything, "uuid-canon").Return(nil).Once()

	// Step 5: soft-delete retired + tombstone.
	repo.On("SoftDeleteEvent", mock.Anything, retireULID, "consolidated").Return(nil).Once()
	repo.On("CreateTombstone", mock.Anything, mock.Anything).Return(nil).Once()

	// Step 6: dismiss pending reviews.
	repo.On("DismissPendingReviewsByEventULIDs", mock.Anything, []string{retireULID}, "system").Return([]int{}, nil).Once()

	// Step 6b: strip stale dup warnings from canonical's pending review entry (none here).
	repo.On("GetPendingReviewByEventUlid", mock.Anything, canonicalULID).Return(nil, nil).Once()

	body := consolidateBody(t, map[string]any{
		"event_ulid": canonicalULID,
		"retire":     []string{retireULID},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/consolidate", body)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()

	h := newTestAdminHandler(repo)
	h.ConsolidateEvents(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "expected 200")

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	assert.Equal(t, "published", resp["lifecycle_state"], "lifecycle_state mismatch")
	assert.NotNil(t, resp["event"], "event field should be present")
	retiredList, ok := resp["retired"].([]any)
	assert.True(t, ok, "retired should be a list")
	assert.Len(t, retiredList, 1)
	assert.Equal(t, retireULID, retiredList[0])

	repo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// 422 – retire target already deleted
// ---------------------------------------------------------------------------

func TestConsolidate_422_RetiredAlreadyDeleted(t *testing.T) {
	const canonicalULID = "01HTEST0000000000000000001"
	const retireULID = "01HTEST0000000000000000002"

	repo := new(MockRepository)
	setupTxMock(repo)

	// Retired event is already deleted — service returns ErrConsolidateRetiredAlreadyDeleted.
	deletedEvent := &events.Event{
		ID:             "uuid-retire",
		ULID:           retireULID,
		Name:           "Already Gone",
		LifecycleState: "deleted",
	}
	repo.On("GetByULID", mock.Anything, retireULID).Return(deletedEvent, nil).Once()

	body := consolidateBody(t, map[string]any{
		"event_ulid": canonicalULID,
		"retire":     []string{retireULID},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/consolidate", body)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()

	h := newTestAdminHandler(repo)
	h.ConsolidateEvents(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "expected 422")
	repo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// 404 – retire target not found
// ---------------------------------------------------------------------------

func TestConsolidate_404_RetireTargetNotFound(t *testing.T) {
	const canonicalULID = "01HTEST0000000000000000001"
	const retireULID = "01HTEST0000000000000000002"

	repo := new(MockRepository)
	setupTxMock(repo)

	repo.On("GetByULID", mock.Anything, retireULID).Return(nil, events.ErrNotFound).Once()

	body := consolidateBody(t, map[string]any{
		"event_ulid": canonicalULID,
		"retire":     []string{retireULID},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/consolidate", body)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()

	h := newTestAdminHandler(repo)
	h.ConsolidateEvents(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code, "expected 404")
	repo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// 400 – malformed event_ulid
// ---------------------------------------------------------------------------

func TestConsolidate_400_MalformedEventULID(t *testing.T) {
	repo := new(MockRepository)
	h := newTestAdminHandler(repo)

	body := consolidateBody(t, map[string]any{
		"event_ulid": "not-a-ulid",
		"retire":     []string{"01HTEST0000000000000000002"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/consolidate", body)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()

	h.ConsolidateEvents(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "expected 400 for malformed event_ulid")
	repo.AssertExpectations(t) // no repo calls expected
}

// ---------------------------------------------------------------------------
// 400 – malformed ULID in retire list
// ---------------------------------------------------------------------------

func TestConsolidate_400_MalformedRetireULID(t *testing.T) {
	repo := new(MockRepository)
	h := newTestAdminHandler(repo)

	body := consolidateBody(t, map[string]any{
		"event_ulid": "01HTEST0000000000000000001",
		"retire":     []string{"01HTEST0000000000000000002", "not-a-ulid"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/consolidate", body)
	req = withAdminUser(req, "admin@example.com")
	rec := httptest.NewRecorder()

	h.ConsolidateEvents(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "expected 400 for malformed retire ULID")
	repo.AssertExpectations(t) // no repo calls expected
}
