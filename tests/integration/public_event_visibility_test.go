package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/Togather-Foundation/server/tests/testhelpers"
	mcpTypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// publicVisibilitySeed is a set of events differing only in lifecycle state.
type publicVisibilitySeed struct {
	Published     string // ULID
	Draft         string
	PendingReview string
	DraftName     string
	PendingName   string
	PublishedName string
}

// seedPublicVisibilityEvents inserts one published, one draft, and one
// pending_review event, all with upcoming occurrences.
func seedPublicVisibilityEvents(t *testing.T, env *testEnv) publicVisibilitySeed {
	t.Helper()

	org := insertOrganization(t, env, "Public Visibility Org")
	place := insertPlace(t, env, "Public Visibility Venue", "Toronto")

	seed := publicVisibilitySeed{
		PublishedName: "Publicly Visible Concert",
		DraftName:     "Secret Draft Concert",
		PendingName:   "Secret Pending Concert",
	}

	seed.Published = insertEventWithOccurrence(t, env, seed.PublishedName, org.ID, place.ID, "music", "published", []string{"public"}, testhelpers.FutureTime(3))
	seed.Draft = insertEventWithOccurrence(t, env, seed.DraftName, org.ID, place.ID, "music", "draft", []string{"public"}, testhelpers.FutureTime(4))
	seed.PendingReview = insertEventWithOccurrence(t, env, seed.PendingName, org.ID, place.ID, "music", "pending_review", []string{"public"}, testhelpers.FutureTime(5))

	return seed
}

// TestPublicEventVisibilityListAndDetail verifies the anonymous JSON surfaces
// never leak draft or pending_review events.
func TestPublicEventVisibilityListAndDetail(t *testing.T) {
	env := setupTestEnv(t)
	seed := seedPublicVisibilityEvents(t, env)

	t.Run("list excludes draft and pending_review", func(t *testing.T) {
		resp := fetchEventsList(t, env, url.Values{})
		names := eventNames(resp.Items)
		require.Contains(t, names, seed.PublishedName)
		require.NotContains(t, names, seed.DraftName)
		require.NotContains(t, names, seed.PendingName)

		status := getPublicEventStatus(t, env, seed.Published)
		require.Equal(t, http.StatusOK, status, "published event must be served")
	})

	t.Run("state=draft yields an empty public list", func(t *testing.T) {
		resp := fetchEventsList(t, env, url.Values{"state": {"draft"}})
		require.Empty(t, resp.Items)
	})

	t.Run("detail hides draft and pending_review without leaking existence", func(t *testing.T) {
		require.Equal(t, http.StatusOK, getPublicEventStatus(t, env, seed.Published))
		require.Equal(t, http.StatusNotFound, getPublicEventStatus(t, env, seed.Draft))
		require.Equal(t, http.StatusNotFound, getPublicEventStatus(t, env, seed.PendingReview))
	})
}

// TestPublicEventVisibilityICS verifies the ICS export and single-event ICS
// hide non-public events.
func TestPublicEventVisibilityICS(t *testing.T) {
	env := setupTestEnv(t)
	seed := seedPublicVisibilityEvents(t, env)

	t.Run("feed excludes draft and pending_review", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/events.ics", nil)
		require.NoError(t, err)
		resp, err := env.Server.Client().Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		ics := string(body)

		require.Contains(t, ics, seed.PublishedName)
		require.NotContains(t, ics, seed.DraftName)
		require.NotContains(t, ics, seed.PendingName)
	})

	t.Run("single event hides draft and serves published", func(t *testing.T) {
		require.Equal(t, http.StatusNotFound, getStatus(t, env, "/api/v1/events/"+seed.Draft+"/ics"))
		require.Equal(t, http.StatusNotFound, getStatus(t, env, "/api/v1/events/"+seed.PendingReview+"/ics"))
		require.Equal(t, http.StatusOK, getStatus(t, env, "/api/v1/events/"+seed.Published+"/ics"))
	})
}

// TestPublicEventVisibilityHTMLPages verifies the public HTML detail page hides
// non-public events and still serves tombstones for deleted ones.
func TestPublicEventVisibilityHTMLPages(t *testing.T) {
	env := setupTestEnv(t)
	seed := seedPublicVisibilityEvents(t, env)

	require.Equal(t, http.StatusOK, getStatusWithAccept(t, env, "/events/"+seed.Published, "text/html"))
	require.Equal(t, http.StatusNotFound, getStatusWithAccept(t, env, "/events/"+seed.Draft, "text/html"))
	require.Equal(t, http.StatusNotFound, getStatusWithAccept(t, env, "/events/"+seed.PendingReview, "text/html"))

	softDeleteEvent(t, env, lookupEventIDByULID(t, env, seed.Published))
	require.Equal(t, http.StatusGone, getStatusWithAccept(t, env, "/events/"+seed.Published, "text/html"),
		"deleted events must still return a 410 tombstone on the HTML surface")
}

// TestPublicEventVisibilityChangeFeed verifies the unconditional public change
// feed omits non-public create/update rows but keeps delete rows.
func TestPublicEventVisibilityChangeFeed(t *testing.T) {
	env := setupTestEnv(t)
	seed := seedPublicVisibilityEvents(t, env)

	publishedID := lookupEventIDByULID(t, env, seed.Published)
	draftID := lookupEventIDByULID(t, env, seed.Draft)
	pendingID := lookupEventIDByULID(t, env, seed.PendingReview)

	insertChange(t, env, publishedID, "create")
	insertChange(t, env, draftID, "create")
	insertChange(t, env, pendingID, "create")
	insertChange(t, env, draftID, "update")
	insertChange(t, env, pendingID, "update")

	resp := fetchChangeFeed(t, env, url.Values{"limit": {"200"}})

	var seenPublished bool
	var lastSequence int64
	for _, change := range resp.Changes {
		require.Greater(t, change.SequenceNumber, lastSequence, "change feed must stay ordered by sequence_number")
		lastSequence = change.SequenceNumber

		require.NotEqual(t, draftID, change.EventID, "draft changes must not appear in the public feed")
		require.NotEqual(t, pendingID, change.EventID, "pending_review changes must not appear in the public feed")
		if change.EventID == publishedID && change.Action == "create" {
			seenPublished = true
		}
	}
	require.True(t, seenPublished, "published create change must appear in the feed")

	// Deleted events must still surface their delete row.
	softDeleteEvent(t, env, publishedID)
	insertChange(t, env, publishedID, "delete")

	resp = fetchChangeFeed(t, env, url.Values{"limit": {"200"}})
	var seenDelete bool
	for _, change := range resp.Changes {
		if change.EventID == publishedID {
			require.Equal(t, "delete", change.Action,
				"only the delete row of a deleted event may remain in the public feed")
			seenDelete = true
		}
	}
	require.True(t, seenDelete, "delete/tombstone row must be preserved")
}

// TestPublicEventVisibilityMCP verifies the in-process MCP tools use the public
// read paths.
func TestPublicEventVisibilityMCP(t *testing.T) {
	env := setupTestEnv(t)
	seed := seedPublicVisibilityEvents(t, env)

	cli := setupMCPClient(t, env)
	defer func() { _ = cli.Close() }()

	ctx := context.Background()

	t.Run("list excludes draft and pending_review", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{Name: "events", Arguments: map[string]any{}},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		names := mcpEventNames(t, payload)
		require.Contains(t, names, seed.PublishedName)
		require.NotContains(t, names, seed.DraftName)
		require.NotContains(t, names, seed.PendingName)
	})

	t.Run("get hides draft and pending_review", func(t *testing.T) {
		for _, ulid := range []string{seed.Draft, seed.PendingReview} {
			result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
				Params: mcpTypes.CallToolParams{Name: "events", Arguments: map[string]any{"id": ulid}},
			})
			require.NoError(t, err)
			text, ok := mcpTypes.AsTextContent(result.Content[0])
			require.True(t, ok)
			require.Contains(t, text.Text, "event not found", "MCP get must not leak non-public events")
		}

		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{Name: "events", Arguments: map[string]any{"id": seed.Published}},
		})
		require.NoError(t, err)
		payload := decodeToolText(t, result)
		require.Equal(t, seed.PublishedName, payload["name"])
	})

	t.Run("search excludes draft and pending_review", func(t *testing.T) {
		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{Name: "search", Arguments: map[string]any{"query": "Secret", "types": []any{"event"}}},
		})
		require.NoError(t, err)

		payload := decodeToolText(t, result)
		names := mcpEventNames(t, payload)
		require.NotContains(t, names, seed.DraftName)
		require.NotContains(t, names, seed.PendingName)

		result, err = cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{Name: "search", Arguments: map[string]any{"query": "Publicly Visible", "types": []any{"event"}}},
		})
		require.NoError(t, err)
		payload = decodeToolText(t, result)
		require.Contains(t, mcpEventNames(t, payload), seed.PublishedName)
	})

	t.Run("deleted returns a tombstone result", func(t *testing.T) {
		softDeleteEvent(t, env, lookupEventIDByULID(t, env, seed.Published))

		result, err := cli.CallTool(ctx, mcpTypes.CallToolRequest{
			Params: mcpTypes.CallToolParams{Name: "events", Arguments: map[string]any{"id": seed.Published}},
		})
		require.NoError(t, err)

		text, ok := mcpTypes.AsTextContent(result.Content[0])
		require.True(t, ok)
		require.NotContains(t, text.Text, "event not found", "deleted events must not be reported as missing")

		payload := decodeToolText(t, result)
		require.Equal(t, true, payload["sel:tombstone"], "MCP must return the tombstone payload for deleted events")
	})
}

// TestPublicEventVisibilityChangeFeedSnapshots is the regression guard for
// pre-publication snapshot leakage: an event that is currently published must
// not re-expose change rows that were recorded while it was draft or
// pending_review.
func TestPublicEventVisibilityChangeFeedSnapshots(t *testing.T) {
	env := setupTestEnv(t)
	seed := seedPublicVisibilityEvents(t, env)

	publishedID := lookupEventIDByULID(t, env, seed.Published)

	const (
		draftMarker   = "DRAFT-SNAPSHOT-LEAK-MARKER"
		pendingMarker = "PENDING-SNAPSHOT-LEAK-MARKER"
		publicMarker  = "PUBLIC-SNAPSHOT-CONTROL-MARKER"
	)

	insertChangeWithSnapshot(t, env, publishedID, "update", map[string]any{
		"ulid":            seed.Published,
		"name":            draftMarker,
		"lifecycle_state": "draft",
	})
	insertChangeWithSnapshot(t, env, publishedID, "update", map[string]any{
		"ulid":            seed.Published,
		"name":            pendingMarker,
		"lifecycle_state": "pending_review",
	})
	insertChangeWithSnapshot(t, env, publishedID, "create", map[string]any{
		"ulid":            seed.Published,
		"name":            publicMarker,
		"lifecycle_state": "published",
	})

	resp := fetchChangeFeed(t, env, url.Values{"limit": {"200"}, "include_snapshot": {"true"}})

	var sawPublicControl bool
	for _, change := range resp.Changes {
		snapshot := string(change.Snapshot)
		require.NotContains(t, snapshot, draftMarker,
			"a row recorded while draft must not be served after the event is published")
		require.NotContains(t, snapshot, pendingMarker,
			"a row recorded while pending_review must not be served after the event is published")
		if change.Action != "delete" && strings.Contains(snapshot, publicMarker) {
			sawPublicControl = true
		}
	}
	require.True(t, sawPublicControl, "a row recorded while published must remain visible with its snapshot")

	// Defence in depth: the markers must not appear anywhere in the decoded feed.
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	require.NotContains(t, string(raw), draftMarker)
	require.NotContains(t, string(raw), pendingMarker)
}

// TestPublicEventVisibilityChangeFeedPaginationSkipsHidden verifies that a
// hidden row in the middle of a page does not produce an empty or duplicated
// page when paging with limit=1.
func TestPublicEventVisibilityChangeFeedPaginationSkipsHidden(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Feed Pagination Org")
	place := insertPlace(t, env, "Feed Pagination Venue", "Toronto")

	firstULID := insertEventWithOccurrence(t, env, "Pagination Public A", org.ID, place.ID, "music", "published", nil, testhelpers.FutureTime(6))
	draftULID := insertEventWithOccurrence(t, env, "Pagination Hidden", org.ID, place.ID, "music", "draft", nil, testhelpers.FutureTime(7))
	secondULID := insertEventWithOccurrence(t, env, "Pagination Public B", org.ID, place.ID, "music", "published", nil, testhelpers.FutureTime(8))

	firstID := lookupEventIDByULID(t, env, firstULID)
	draftID := lookupEventIDByULID(t, env, draftULID)
	secondID := lookupEventIDByULID(t, env, secondULID)

	page1 := fetchChangeFeed(t, env, url.Values{"limit": {"1"}})
	require.Len(t, page1.Changes, 1, "first page must contain the first published event")
	require.Equal(t, firstID, page1.Changes[0].EventID)
	require.NotEqual(t, draftID, page1.Changes[0].EventID)
	require.NotEmpty(t, page1.NextCursor)

	page2 := fetchChangeFeed(t, env, url.Values{"limit": {"1"}, "since": {page1.NextCursor}})
	require.Len(t, page2.Changes, 1, "second page must skip the hidden draft row and contain the next published event")
	require.Equal(t, secondID, page2.Changes[0].EventID)
	require.NotEqual(t, draftID, page2.Changes[0].EventID)
	require.Greater(t, page2.Changes[0].SequenceNumber, page1.Changes[0].SequenceNumber,
		"pages must preserve sequence ordering")
	require.NotEqual(t, firstID, page2.Changes[0].EventID, "the second page must not repeat the first event")

	for _, change := range append(page1.Changes, page2.Changes...) {
		require.NotEqual(t, draftID, change.EventID, "draft rows must never be paged into the public feed")
	}
}

// TestPublicEventVisibilityApprovalTransition exercises the real
// pending_review -> published UPDATE: the approval row must appear as a visible
// `update` with a published snapshot, while the pre-publication `create` row
// stays hidden forever.
func TestPublicEventVisibilityApprovalTransition(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Approval Transition Org")
	place := insertPlace(t, env, "Approval Transition Venue", "Toronto")

	const eventName = "Approved After Review"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "pending_review", nil, testhelpers.FutureTime(9))
	eventID := lookupEventIDByULID(t, env, eventULID)

	// Baseline: the pending_review create row must not be visible at all.
	baseline := fetchChangeFeed(t, env, url.Values{"limit": {"200"}, "include_snapshot": {"true"}})
	for _, change := range baseline.Changes {
		require.NotEqual(t, eventID, change.EventID, "pre-publication rows must not be served")
	}

	// Real lifecycle transition (this is what review approval does).
	_, err := env.Pool.Exec(env.Context,
		`UPDATE events SET lifecycle_state = 'published', updated_at = now() WHERE id = $1`,
		eventID)
	require.NoError(t, err)

	resp := fetchChangeFeed(t, env, url.Values{"limit": {"200"}, "include_snapshot": {"true"}})

	var sawPublishedUpdate bool
	for _, change := range resp.Changes {
		if change.EventID != eventID {
			continue
		}
		require.Equal(t, "update", change.Action, "only the approval update row may be served; the create row must stay hidden")
		require.NotEmpty(t, change.Snapshot, "approval update must carry its published snapshot")

		var snapshot map[string]any
		require.NoError(t, json.Unmarshal(change.Snapshot, &snapshot))
		require.Equal(t, "published", snapshot["lifecycleState"])
		require.Equal(t, eventName, snapshot["name"])
		sawPublishedUpdate = true
	}
	require.True(t, sawPublishedUpdate, "approval must surface a visible update row for the event")
}

// TestPublicEventVisibilityRejectedSubmissionNotInFeed guards the tombstone
// leak: rejecting a submission that never left pending_review must not publish
// a delete row (with @id/name/deletion reason) on the anonymous feed.
func TestPublicEventVisibilityRejectedSubmissionNotInFeed(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Rejected Submission Org")
	place := insertPlace(t, env, "Rejected Submission Venue", "Toronto")

	const eventName = "Rejected Never Public Submission"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "pending_review", nil, testhelpers.FutureTime(10))
	eventID := lookupEventIDByULID(t, env, eventULID)

	var reviewID int
	err := env.Pool.QueryRow(env.Context,
		`INSERT INTO event_review_queue (event_id, original_payload, normalized_payload, warnings, event_start_time)
		 VALUES ($1, '{}'::jsonb, '{}'::jsonb, '[]'::jsonb, $2)
		 RETURNING id`,
		eventID, testhelpers.FutureTime(10)).Scan(&reviewID)
	require.NoError(t, err)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")

	body, err := json.Marshal(map[string]string{"reason": "insufficient_quality"})
	require.NoError(t, err)
	rejectReq, err := http.NewRequest(http.MethodPost,
		env.Server.URL+"/api/v1/admin/review-queue/"+strconv.Itoa(reviewID)+"/reject",
		bytes.NewReader(body))
	require.NoError(t, err)
	rejectReq.Header.Set("Authorization", "Bearer "+adminToken)
	rejectReq.Header.Set("Content-Type", "application/json")
	rejectReq.Header.Set("Accept", "application/json")

	rejectResp, err := env.Server.Client().Do(rejectReq)
	require.NoError(t, err)
	defer func() { _ = rejectResp.Body.Close() }()
	require.Equal(t, http.StatusOK, rejectResp.StatusCode, "reject route must succeed")

	var lifecycle string
	require.NoError(t, env.Pool.QueryRow(env.Context,
		`SELECT lifecycle_state FROM events WHERE id = $1`, eventID).Scan(&lifecycle))
	require.Equal(t, "deleted", lifecycle)

	for _, includeSnapshot := range []bool{false, true} {
		params := url.Values{"limit": {"200"}}
		if includeSnapshot {
			params.Set("include_snapshot", "true")
		}

		raw := fetchChangeFeedBody(t, env, params)
		require.NotContains(t, raw, eventID, "event UUID must not appear in the public feed")
		require.NotContains(t, raw, eventULID, "event ULID must not appear in the public feed")
		require.NotContains(t, raw, eventName, "never-public event name must not appear in the public feed")

		feed := fetchChangeFeed(t, env, params)
		for _, change := range feed.Changes {
			require.NotEqual(t, eventID, change.EventID,
				"a rejected never-public submission must not emit a delete row")
		}
	}
}

// TestPublicEventVisibilityAdminListUnfiltered verifies the admin list keeps
// serving non-public events.
func TestPublicEventVisibilityAdminListUnfiltered(t *testing.T) {
	env := setupTestEnv(t)
	seed := seedPublicVisibilityEvents(t, env)

	insertAdminUser(t, env, "admin", "admin-password-123", "admin@togather.test", "admin")
	adminToken := adminLogin(t, env, "admin", "admin-password-123")

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/api/v1/admin/events?limit=200", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Accept", "application/json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var list struct {
		Items []struct {
			ID             string `json:"@id"`
			LifecycleState string `json:"lifecycle_state"`
		} `json:"items"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))

	states := map[string]string{}
	for _, item := range list.Items {
		for _, ulid := range []string{seed.Published, seed.Draft, seed.PendingReview} {
			if strings.Contains(item.ID, ulid) {
				states[ulid] = item.LifecycleState
			}
		}
	}
	require.Equal(t, "published", states[seed.Published])
	require.Equal(t, "draft", states[seed.Draft])
	require.Equal(t, "pending_review", states[seed.PendingReview])
}

// TestPublicEventVisibilityRepositoryFilters exercises the storage predicates directly.
func TestPublicEventVisibilityRepositoryFilters(t *testing.T) {
	env := setupTestEnv(t)
	seed := seedPublicVisibilityEvents(t, env)

	repo, err := postgres.NewRepository(env.Pool, testhelpers.TestLogger())
	require.NoError(t, err)
	eventsRepo := repo.Events()

	t.Run("List with PublicOnly excludes draft and pending_review", func(t *testing.T) {
		result, err := eventsRepo.List(env.Context, events.Filters{PublicOnly: true}, events.Pagination{Limit: 200})
		require.NoError(t, err)

		names := make([]string, 0, len(result.Events))
		states := map[string]bool{}
		for _, event := range result.Events {
			names = append(names, event.Name)
			states[event.LifecycleState] = true
		}
		require.Contains(t, names, seed.PublishedName)
		require.NotContains(t, names, seed.DraftName)
		require.NotContains(t, names, seed.PendingName)
		require.True(t, states["published"])
		require.False(t, states["draft"])
		require.False(t, states["pending_review"])
	})

	t.Run("List without PublicOnly keeps admin visibility", func(t *testing.T) {
		result, err := eventsRepo.List(env.Context, events.Filters{}, events.Pagination{Limit: 200})
		require.NoError(t, err)

		names := make([]string, 0, len(result.Events))
		for _, event := range result.Events {
			names = append(names, event.Name)
		}
		require.Contains(t, names, seed.PublishedName)
		require.Contains(t, names, seed.DraftName)
		require.Contains(t, names, seed.PendingName)
	})

	t.Run("GetPublicByULID hides draft and pending_review", func(t *testing.T) {
		published, err := eventsRepo.GetPublicByULID(env.Context, seed.Published)
		require.NoError(t, err)
		require.Equal(t, "published", published.LifecycleState)

		_, err = eventsRepo.GetPublicByULID(env.Context, seed.Draft)
		require.ErrorIs(t, err, events.ErrNotFound)

		_, err = eventsRepo.GetPublicByULID(env.Context, seed.PendingReview)
		require.ErrorIs(t, err, events.ErrNotFound)
	})

	t.Run("GetPublicByULID still returns deleted rows for tombstones", func(t *testing.T) {
		deletedULID := seed.Published
		deletedID := lookupEventIDByULID(t, env, deletedULID)
		softDeleteEvent(t, env, deletedID)

		event, err := eventsRepo.GetPublicByULID(env.Context, deletedULID)
		require.NoError(t, err)
		require.Equal(t, "deleted", event.LifecycleState)
	})
}

func getPublicEventStatus(t *testing.T, env *testEnv, ulid string) int {
	t.Helper()
	return getStatusWithAccept(t, env, "/api/v1/events/"+ulid, "application/ld+json")
}

func getStatus(t *testing.T, env *testEnv, path string) int {
	t.Helper()
	return getStatusWithAccept(t, env, path, "")
}

func getStatusWithAccept(t *testing.T, env *testEnv, path, accept string) int {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+path, nil)
	require.NoError(t, err)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// insertChange appends an event_changes row for the given event ID.
func insertChange(t *testing.T, env *testEnv, eventID, action string) {
	t.Helper()

	_, err := env.Pool.Exec(env.Context,
		`INSERT INTO event_changes (event_id, action, changed_at) VALUES ($1, $2, now())`,
		eventID, action,
	)
	require.NoError(t, err)

	// Ensure distinct sequence numbers for ordering assertions.
	time.Sleep(2 * time.Millisecond)
}

// insertChangeWithSnapshot appends an event_changes row carrying a full event
// snapshot (the shape the change-feed triggers write).
func insertChangeWithSnapshot(t *testing.T, env *testEnv, eventID, action string, snapshot map[string]any) {
	t.Helper()

	payload, err := json.Marshal(snapshot)
	require.NoError(t, err)

	_, err = env.Pool.Exec(env.Context,
		`INSERT INTO event_changes (event_id, action, snapshot, changed_at) VALUES ($1, $2, $3, now())`,
		eventID, action, payload,
	)
	require.NoError(t, err)

	time.Sleep(2 * time.Millisecond)
}

// fetchChangeFeedBody returns the raw change-feed response body so tests can
// assert that forbidden identifiers/names appear nowhere in the payload.
func fetchChangeFeedBody(t *testing.T, env *testEnv, params url.Values) string {
	t.Helper()

	u := env.Server.URL + "/api/v1/feeds/changes?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/ld+json")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func mcpEventNames(t *testing.T, payload map[string]any) []string {
	t.Helper()

	rawItems, ok := payload["items"].([]any)
	require.True(t, ok, "MCP payload missing items array: %+v", payload)

	names := make([]string, 0, len(rawItems))
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if itemType, _ := item["type"].(string); itemType != "" && itemType != "event" {
			continue
		}
		if name, ok := item["name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}
