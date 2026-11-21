package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReviewQueueTablePartialURLs(t *testing.T) {
	const eventURL = "https://venue.example.org/events/community-arts"
	const sourceURL = "https://organizer.example.org/calendar"
	items := []ReviewQueueItem{
		{ID: 1, EventID: "listing-only", EventURL: eventURL},
		{ID: 2, EventID: "source-only", SourceURL: sourceURL},
	}

	var out strings.Builder
	require.NoError(t, printFlatTable(&out, items))
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 3)
	listingRow := strings.Fields(lines[1])
	sourceRow := strings.Fields(lines[2])
	require.GreaterOrEqual(t, len(listingRow), 2)
	require.GreaterOrEqual(t, len(sourceRow), 2)
	require.Equal(t, []string{eventURL, "-"}, listingRow[len(listingRow)-2:])
	require.Equal(t, []string{"-", sourceURL}, sourceRow[len(sourceRow)-2:])
}
