package events

import (
	"testing"
)

func TestAutoMergeFields(t *testing.T) {
	tests := []struct {
		name          string
		existing      *Event
		input         EventInput
		existingTrust int
		newTrust      int
		wantChanged   bool
		checkFn       func(t *testing.T, params UpdateEventParams)
	}{
		{
			name: "empty existing, new has data - all fields filled",
			existing: &Event{
				Name: "Test Event",
				// All other fields empty
			},
			input: EventInput{
				Name:        "Test Event",
				Description: "A great event",
				Image:       "https://example.com/img.jpg",
				URL:         "https://example.com/event",
				EventDomain: "music",
				Keywords:    []string{"jazz", "live"},
			},
			existingTrust: 5,
			newTrust:      5,
			wantChanged:   true,
			checkFn: func(t *testing.T, params UpdateEventParams) {
				if params.Description == nil || *params.Description != "A great event" {
					t.Error("expected Description to be filled")
				}
				if params.ImageURL == nil || *params.ImageURL != "https://example.com/img.jpg" {
					t.Error("expected ImageURL to be filled")
				}
				if params.PublicURL == nil || *params.PublicURL != "https://example.com/event" {
					t.Error("expected PublicURL to be filled")
				}
				if params.EventDomain == nil || *params.EventDomain != "music" {
					t.Error("expected EventDomain to be filled")
				}
				if len(params.Keywords) != 2 || params.Keywords[0] != "jazz" {
					t.Error("expected Keywords to be filled")
				}
				// Name should NOT be set (we never merge Name)
				if params.Name != nil {
					t.Error("expected Name to NOT be set")
				}
				// LifecycleState should NOT be set
				if params.LifecycleState != nil {
					t.Error("expected LifecycleState to NOT be set")
				}
			},
		},
		{
			name: "full existing, new has data with lower trust - nothing changes",
			existing: &Event{
				Name:        "Test Event",
				Description: "Existing description",
				ImageURL:    "https://example.com/old.jpg",
				PublicURL:   "https://example.com/old",
				EventDomain: "arts",
				Keywords:    []string{"painting"},
			},
			input: EventInput{
				Description: "New description",
				Image:       "https://example.com/new.jpg",
				URL:         "https://example.com/new",
				EventDomain: "music",
				Keywords:    []string{"jazz"},
			},
			existingTrust: 8,
			newTrust:      3, // lower trust (lower number = less trusted)
			wantChanged:   false,
			checkFn: func(t *testing.T, params UpdateEventParams) {
				if params.Description != nil {
					t.Error("expected Description to remain unchanged")
				}
				if params.ImageURL != nil {
					t.Error("expected ImageURL to remain unchanged")
				}
				if params.PublicURL != nil {
					t.Error("expected PublicURL to remain unchanged")
				}
				if params.EventDomain != nil {
					t.Error("expected EventDomain to remain unchanged")
				}
				if len(params.Keywords) > 0 {
					t.Error("expected Keywords to remain unchanged")
				}
			},
		},
		{
			name: "full existing, new has data with higher trust - fields overwritten",
			existing: &Event{
				Name:        "Test Event",
				Description: "Old description",
				ImageURL:    "https://example.com/old.jpg",
				PublicURL:   "https://example.com/old",
				EventDomain: "arts",
				Keywords:    []string{"painting"},
			},
			input: EventInput{
				Description: "Better description",
				Image:       "https://example.com/better.jpg",
				URL:         "https://example.com/better",
				EventDomain: "music",
				Keywords:    []string{"jazz", "live"},
			},
			existingTrust: 5,
			newTrust:      8, // higher trust (higher number = more trusted)
			wantChanged:   true,
			checkFn: func(t *testing.T, params UpdateEventParams) {
				if params.Description == nil || *params.Description != "Better description" {
					t.Error("expected Description to be overwritten")
				}
				if params.ImageURL == nil || *params.ImageURL != "https://example.com/better.jpg" {
					t.Error("expected ImageURL to be overwritten")
				}
				if params.PublicURL == nil || *params.PublicURL != "https://example.com/better" {
					t.Error("expected PublicURL to be overwritten")
				}
				if params.EventDomain == nil || *params.EventDomain != "music" {
					t.Error("expected EventDomain to be overwritten")
				}
				if len(params.Keywords) != 2 || params.Keywords[0] != "jazz" {
					t.Error("expected Keywords to be overwritten")
				}
			},
		},
		{
			name: "partial existing, new fills gaps",
			existing: &Event{
				Name:        "Test Event",
				Description: "Has description",
				// ImageURL, PublicURL, EventDomain, Keywords are empty
			},
			input: EventInput{
				Description: "Different description",
				Image:       "https://example.com/img.jpg",
				URL:         "https://example.com/event",
				EventDomain: "music",
				Keywords:    []string{"jazz"},
			},
			existingTrust: 8,
			newTrust:      3, // lower trust, but gaps should still fill
			wantChanged:   true,
			checkFn: func(t *testing.T, params UpdateEventParams) {
				// Description should NOT change (existing has value, new trust is lower)
				if params.Description != nil {
					t.Error("expected Description to remain unchanged (existing has value, lower trust)")
				}
				// Gaps should be filled regardless of trust
				if params.ImageURL == nil || *params.ImageURL != "https://example.com/img.jpg" {
					t.Error("expected ImageURL gap to be filled")
				}
				if params.PublicURL == nil || *params.PublicURL != "https://example.com/event" {
					t.Error("expected PublicURL gap to be filled")
				}
				if params.EventDomain == nil || *params.EventDomain != "music" {
					t.Error("expected EventDomain gap to be filled")
				}
				if len(params.Keywords) != 1 || params.Keywords[0] != "jazz" {
					t.Error("expected Keywords gap to be filled")
				}
			},
		},
		{
			name: "both empty - nothing changes",
			existing: &Event{
				Name: "Test Event",
			},
			input:         EventInput{},
			existingTrust: 5,
			newTrust:      5,
			wantChanged:   false,
			checkFn: func(t *testing.T, params UpdateEventParams) {
				if params.Description != nil || params.ImageURL != nil || params.PublicURL != nil || params.EventDomain != nil || len(params.Keywords) > 0 {
					t.Error("expected no fields to change when both are empty")
				}
			},
		},
		{
			name: "same trust level - keep existing (tie goes to existing)",
			existing: &Event{
				Name:        "Test Event",
				Description: "Existing",
				ImageURL:    "https://example.com/existing.jpg",
				PublicURL:   "https://example.com/existing",
				EventDomain: "arts",
				Keywords:    []string{"art"},
			},
			input: EventInput{
				Description: "New",
				Image:       "https://example.com/new.jpg",
				URL:         "https://example.com/new",
				EventDomain: "music",
				Keywords:    []string{"music"},
			},
			existingTrust: 5,
			newTrust:      5,
			wantChanged:   false,
			checkFn: func(t *testing.T, params UpdateEventParams) {
				if params.Description != nil {
					t.Error("expected Description unchanged with same trust")
				}
				if params.ImageURL != nil {
					t.Error("expected ImageURL unchanged with same trust")
				}
			},
		},
		{
			name: "keywords merge - existing empty",
			existing: &Event{
				Name: "Test Event",
			},
			input: EventInput{
				Keywords: []string{"jazz", "live", "music"},
			},
			existingTrust: 5,
			newTrust:      5,
			wantChanged:   true,
			checkFn: func(t *testing.T, params UpdateEventParams) {
				if len(params.Keywords) != 3 {
					t.Errorf("expected 3 keywords, got %d", len(params.Keywords))
				}
			},
		},
		{
			name: "keywords merge - existing has values, higher trust new",
			existing: &Event{
				Name:     "Test Event",
				Keywords: []string{"old"},
			},
			input: EventInput{
				Keywords: []string{"new", "better"},
			},
			existingTrust: 5,
			newTrust:      8, // higher trust
			wantChanged:   true,
			checkFn: func(t *testing.T, params UpdateEventParams) {
				if len(params.Keywords) != 2 || params.Keywords[0] != "new" {
					t.Error("expected keywords to be overwritten with higher trust")
				}
			},
		},
		{
			name: "keywords merge - existing has values, lower trust new",
			existing: &Event{
				Name:     "Test Event",
				Keywords: []string{"existing"},
			},
			input: EventInput{
				Keywords: []string{"new"},
			},
			existingTrust: 8,
			newTrust:      3, // lower trust
			wantChanged:   false,
			checkFn: func(t *testing.T, params UpdateEventParams) {
				if len(params.Keywords) > 0 {
					t.Error("expected keywords to remain unchanged with lower trust")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, changed := AutoMergeFields(tt.existing, tt.input, tt.existingTrust, tt.newTrust)
			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, params)
			}
		})
	}
}
