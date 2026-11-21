package fixtures

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/require"
)

func TestGeneratedEventsDeclareSyntheticProvenance(t *testing.T) {
	g := NewDeterministicGenerator()
	first, second := g.DuplicateCandidates()
	inputs := []events.EventInput{
		g.RandomEventInput(), g.MinimalEventInput(), g.VirtualEventInput(), g.HybridEventInput(),
		g.EventInputWithOccurrences(2), g.EventInputNeedsReview(), g.EventInputFarFuture(),
		g.EventInputReversedDates(), g.EventInputMissingVenue(), g.EventInputLikelyDuplicate(),
		g.EventInputMultipleWarnings(), g.NewRecurringSeriesBuilder().Build(), first, second,
	}
	for _, scenario := range g.BatchReviewEventInputs() {
		inputs = append(inputs, scenario.Events...)
	}
	for _, input := range inputs {
		t.Run(input.Name, func(t *testing.T) {
			raw, err := json.Marshal(input)
			require.NoError(t, err)
			require.Contains(t, string(raw), `"sel:synthetic":true`)
			var decoded events.EventInput
			require.NoError(t, json.Unmarshal(raw, &decoded))
			wire, err := json.Marshal(decoded)
			require.NoError(t, err)
			require.Contains(t, string(wire), `"sel:synthetic":true`)
			urls := []string{input.URL}
			if input.Source != nil {
				urls = append(urls, input.Source.URL)
			}
			if input.VirtualLocation != nil {
				urls = append(urls, input.VirtualLocation.URL)
			}
			for _, link := range urls {
				if link == "" {
					continue
				}
				u, err := url.Parse(link)
				require.NoError(t, err)
				require.True(t, strings.HasSuffix(u.Hostname(), ".invalid"), "generated link %s", link)
			}
		})
	}
}
