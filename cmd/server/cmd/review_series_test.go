package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintSeriesCandidates(t *testing.T) {
	resp := SeriesCandidatesResponse{
		TotalClusters: 1,
		Items: []SeriesCandidateItem{
			{
				SuggestedCanonicalULID: "01HEVENT000000000000000001",
				Name:                   "Family Storytime",
				Venue:                  SeriesCandidateVenue{ID: "01HPLACE000000000000000001", Name: "Main Branch"},
				DayOfWeek:              2,
				LocalStartTime:         "18:00",
				EventCount:             3,
				OccurrenceCount:        3,
				FirstDate:              "2026-11-10",
				LastDate:               "2026-11-24",
				Members: []SeriesCandidateMember{
					{ULID: "01HEVENT000000000000000001", Name: "Family Storytime", StartDate: "2026-11-10", OccurrenceCount: 1, LifecycleState: "published"},
					{ULID: "01HEVENT000000000000000002", Name: "Family Storytime", StartDate: "2026-11-17", OccurrenceCount: 1, LifecycleState: "published"},
					{ULID: "01HEVENT000000000000000003", Name: "Family Storytime (Toddler)", StartDate: "2026-11-24", OccurrenceCount: 1, LifecycleState: "published"},
				},
			},
		},
	}

	var out bytes.Buffer
	printSeriesCandidates(&out, resp)
	got := out.String()

	for _, want := range []string{
		"1) Family Storytime — Main Branch — Tue 18:00 — 3 events — 2026-11-10..2026-11-24",
		"names differ: inspect carefully",
		"canonical: 01HEVENT000000000000000001",
		"members:",
		"* 01HEVENT000000000000000001  2026-11-10  Family Storytime",
		"  01HEVENT000000000000000003  2026-11-24  Family Storytime (Toddler)",
		"next: server review consolidate 01HEVENT000000000000000001 01HEVENT000000000000000002 01HEVENT000000000000000003 --transfer-occurrences --dry-run",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}
}

func TestPrintSeriesCandidatesUniformNames(t *testing.T) {
	resp := SeriesCandidatesResponse{
		TotalClusters: 1,
		Items: []SeriesCandidateItem{{
			SuggestedCanonicalULID: "01HEVENT000000000000000001",
			Name:                   "Family Storytime",
			Venue:                  SeriesCandidateVenue{Name: "Main Branch"},
			EventCount:             2,
			Members: []SeriesCandidateMember{
				{ULID: "01HEVENT000000000000000001", Name: "Family Storytime", StartDate: "2026-11-10"},
				{ULID: "01HEVENT000000000000000002", Name: " family storytime ", StartDate: "2026-11-17"},
			},
		}},
	}
	var out bytes.Buffer
	printSeriesCandidates(&out, resp)
	if strings.Contains(out.String(), "names differ") {
		t.Errorf("trimmed/case-insensitive names must not trigger the divergence warning:\n%s", out.String())
	}
}

func TestPrintSeriesCandidatesEmpty(t *testing.T) {
	var out bytes.Buffer
	printSeriesCandidates(&out, SeriesCandidatesResponse{})

	got := out.String()
	if !strings.Contains(got, "No series candidates found") {
		t.Errorf("empty output should explain the result, got:\n%s", got)
	}
	if !strings.Contains(got, "published events") || !strings.Contains(got, "conservative") {
		t.Errorf("empty output should explain scope and conservatism, got:\n%s", got)
	}
}

func TestWeekdayName(t *testing.T) {
	cases := map[int]string{1: "Mon", 2: "Tue", 3: "Wed", 4: "Thu", 5: "Fri", 6: "Sat", 7: "Sun", 0: "?"}
	for isodow, want := range cases {
		if got := weekdayName(isodow); got != want {
			t.Errorf("weekdayName(%d) = %q, want %q", isodow, got, want)
		}
	}
}
