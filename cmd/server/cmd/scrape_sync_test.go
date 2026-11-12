package cmd

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncResultJSON_PreservesCountsAndReportsConfigFailure(t *testing.T) {
	result := syncResult{SourcesFound: 3, Created: 1, Updated: 1, Total: 2, Errors: 1, ErrorDetails: []syncErrorDetail{
		newSyncErrorDetail("calendar-feed", fmt.Errorf("register feed %q: source policy conflict; reconcile policy explicitly before syncing", "https://operator:secret@calendar.example.org/feed.ics?api_key=private#token")),
	}}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.JSONEq(t, `{"sources_found":3,"created":1,"updated":1,"total":2,"warnings":0,"errors":1,"error_details":[{"name":"calendar-feed","error":"register feed \"https://calendar.example.org/feed.ics\": source policy conflict; reconcile policy explicitly before syncing"}]}`, string(data))
	require.NotContains(t, string(data), "secret")
	require.NotContains(t, string(data), "private")
	require.NotContains(t, string(data), "operator")
	require.NotContains(t, string(data), "token")
}

func TestSyncResultJSON_SuccessRetainsExistingShape(t *testing.T) {
	data, err := json.Marshal(syncResult{SourcesFound: 1, Updated: 1, Total: 1})
	require.NoError(t, err)
	require.JSONEq(t, `{"sources_found":1,"created":0,"updated":1,"total":1,"warnings":0,"errors":0}`, string(data))
}
