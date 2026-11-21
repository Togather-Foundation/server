package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/tests/testhelpers"
	"github.com/stretchr/testify/require"
)

func TestFederationSyncSynthetic(t *testing.T) {
	env := setupTestEnv(t)
	const domain = "synthetic-peer.example.org"
	const baseURL = "https://" + domain
	nodeID := insertFederationNode(t, env, domain, "Synthetic Test Peer", baseURL, "active", 8)
	apiKey := testhelpers.InsertAPIKeyWithFederationNode(t, env.Pool, env.Context, "synthetic-federation-key", nodeID)

	for _, tc := range []struct {
		name      string
		synthetic bool
	}{
		{name: "ordinary"},
		{name: "synthetic", synthetic: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eventURI := baseURL + "/events/" + tc.name
			place := map[string]any{
				"@type": "Place", "@id": baseURL + "/places/" + tc.name,
				"name": "Original Place", "addressLocality": "Toronto",
			}
			organization := map[string]any{
				"@type": "Organization", "@id": baseURL + "/organizations/" + tc.name,
				"name": "Original Organization",
			}
			payload := map[string]any{
				"@context":  "https://togather.foundation/contexts/sel/v0.1.jsonld",
				"@type":     "Event",
				"@id":       eventURI,
				"name":      "Original Event",
				"startDate": testhelpers.FutureTime(1).Format(time.RFC3339),
				"location":  place,
				"organizer": organization,
			}
			if tc.synthetic {
				payload["sel:synthetic"] = true
			}
			resp := postFederationSync(t, env, apiKey, payload)
			require.Equal(t, http.StatusCreated, resp.StatusCode)
			localULID := getLocalULID(t, env, eventURI)
			eventID := lookupEventIDByULID(t, env, localULID)

			// Changing the classification must roll back writes already attempted
			// for the location and organizer as well as preserving the event.
			if tc.synthetic {
				delete(payload, "sel:synthetic")
			} else {
				payload["sel:synthetic"] = true
			}
			payload["name"] = "Rejected Event"
			payload["startDate"] = testhelpers.FutureTime(2).Format(time.RFC3339)
			place["name"] = "Rejected Place"
			organization["name"] = "Rejected Organization"
			assertFederationInvalidJSONLD(t, postFederationSync(t, env, apiKey, payload))

			var storedName, storedPlace, storedOrganization string
			var storedSynthetic bool
			var occurrences int
			require.NoError(t, env.Pool.QueryRow(env.Context, `
				SELECT e.name, e.synthetic, p.name, o.name,
				       (SELECT COUNT(*) FROM event_occurrences WHERE event_id = e.id)
				FROM events e
				JOIN places p ON p.id = e.primary_venue_id
				JOIN organizations o ON o.id = e.organizer_id
				WHERE e.id = $1`, eventID).Scan(&storedName, &storedSynthetic, &storedPlace, &storedOrganization, &occurrences))
			require.Equal(t, "Original Event", storedName)
			require.Equal(t, "Original Place", storedPlace)
			require.Equal(t, "Original Organization", storedOrganization)
			require.Equal(t, tc.synthetic, storedSynthetic)
			require.Equal(t, 1, occurrences)

			// Explicit false also matches an ordinary event imported without the field.
			payload["sel:synthetic"] = tc.synthetic
			payload["name"] = "Updated Event"
			place["name"] = "Updated Place"
			organization["name"] = "Updated Organization"
			resp = postFederationSync(t, env, apiKey, payload)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, localULID, getLocalULID(t, env, eventURI))

			payload["sel:synthetic"] = nil
			assertFederationInvalidJSONLD(t, postFederationSync(t, env, apiKey, payload))

			snapshotParams := url.Values{"include_snapshot": {"true"}}
			actions := map[string]string{"create": "Original Event", "update": "Updated Event"}
			for _, item := range fetchChangeFeed(t, env, snapshotParams).Changes {
				if item.EventID == eventID {
					require.Contains(t, actions, item.Action)
					requireSyntheticSnapshot(t, item.Snapshot, tc.synthetic)
					var snapshot map[string]any
					require.NoError(t, json.Unmarshal(item.Snapshot, &snapshot))
					require.Equal(t, actions[item.Action], snapshot["name"])
					delete(actions, item.Action)
				}
			}
			require.Empty(t, actions)

			softDeleteEvent(t, env, eventID)
			createEventTombstone(t, env, eventID, eventURI, "Synthetic marker test")
			snapshotParams.Set("action", "delete")
			var foundDelete bool
			for _, item := range fetchChangeFeed(t, env, snapshotParams).Changes {
				if item.EventID == eventID {
					requireSyntheticSnapshot(t, item.Snapshot, tc.synthetic)
					foundDelete = true
				}
			}
			require.True(t, foundDelete)
		})
	}
}

func assertFederationInvalidJSONLD(t *testing.T, resp *http.Response) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", body)
	var problemBody map[string]any
	require.NoError(t, json.Unmarshal(body, &problemBody))
	require.Equal(t, "https://sel.events/problems/validation-error", problemBody["type"])
	require.Equal(t, "Invalid JSON-LD", problemBody["title"])
}

func requireSyntheticSnapshot(t *testing.T, snapshot json.RawMessage, synthetic bool) {
	t.Helper()
	if synthetic {
		require.Contains(t, string(snapshot), `"sel:synthetic":true`)
	} else {
		require.NotContains(t, string(snapshot), `"sel:synthetic"`)
	}
}
