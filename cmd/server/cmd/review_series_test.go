package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetWithProblemDetailSurfacesSourceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"https://sel.events/problems/validation-error","title":"Invalid request","status":400,"detail":"source: must be a source name or UUID","instance":"/api/v1/admin/events/series-candidates"}`))
	}))
	defer srv.Close()

	_, err := getWithProblemDetail(srv.Client(), srv.URL, "")
	if err == nil {
		t.Fatal("expected an error from a 400 response")
	}
	for _, want := range []string{"server returned status 400", "Invalid request", "must be a source name or UUID"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
}

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
	printSeriesCandidates(&out, resp, 0)
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

func TestPrintSeriesCandidatesPagingHeader(t *testing.T) {
	items := make([]SeriesCandidateItem, 20)
	for i := range items {
		items[i] = SeriesCandidateItem{
			SuggestedCanonicalULID: "01HEVENT000000000000000001",
			Name:                   "Family Storytime",
			Venue:                  SeriesCandidateVenue{Name: "Main Branch"},
			DayOfWeek:              2,
			LocalStartTime:         "18:00",
			EventCount:             2,
			FirstDate:              "2026-11-10",
			LastDate:               "2026-11-17",
			Members: []SeriesCandidateMember{
				{ULID: "01HEVENT000000000000000001", Name: "Family Storytime", StartDate: "2026-11-10"},
				{ULID: "01HEVENT000000000000000002", Name: "Family Storytime", StartDate: "2026-11-17"},
			},
		}
	}

	resp := SeriesCandidatesResponse{TotalClusters: 526, Truncated: true, Items: items}
	var out bytes.Buffer
	printSeriesCandidates(&out, resp, 200)

	want := "526 series candidate(s) (showing 201-220 of 526; next: --offset 220)"
	if !strings.Contains(out.String(), want) {
		t.Errorf("paging header missing %q\n---\n%s", want, out.String())
	}
	if !strings.Contains(out.String(), "\n201) Family Storytime") {
		t.Errorf("cluster numbering should be global on page 2 (start at 201):\n%s", out.String())
	}
}

func TestPrintSeriesCandidatesOffsetPastEnd(t *testing.T) {
	resp := SeriesCandidatesResponse{TotalClusters: 526}
	var out bytes.Buffer
	printSeriesCandidates(&out, resp, 600)

	want := "526 series candidate(s) (offset 600 is past the end)"
	if !strings.Contains(out.String(), want) {
		t.Errorf("past-the-end output missing %q\n---\n%s", want, out.String())
	}
}

func TestPrintSeriesCandidatesNextOffsetRespectsMax(t *testing.T) {
	// A page ending at 10200 would suggest an offset the CLI's own validator
	// rejects, so the header keeps the range and omits the hint.
	items := make([]SeriesCandidateItem, 10)
	for i := range items {
		items[i] = SeriesCandidateItem{
			SuggestedCanonicalULID: "01HEVENT000000000000000001",
			Name:                   "Family Storytime",
			Members: []SeriesCandidateMember{
				{ULID: "01HEVENT000000000000000001", Name: "Family Storytime"},
				{ULID: "01HEVENT000000000000000002", Name: "Family Storytime"},
			},
		}
	}
	resp := SeriesCandidatesResponse{TotalClusters: 10300, Truncated: true, Items: items}
	var out bytes.Buffer
	printSeriesCandidates(&out, resp, 10190)

	if !strings.Contains(out.String(), "(showing 10191-10200 of 10300)") {
		t.Errorf("range header missing:\n%s", out.String())
	}
	if strings.Contains(out.String(), "next: --offset") {
		t.Errorf("must not suggest an offset above seriesMaxOffset:\n%s", out.String())
	}
}

func TestPrintSeriesCandidatesLastPageHeader(t *testing.T) {
	resp := SeriesCandidatesResponse{
		TotalClusters: 205,
		Items: []SeriesCandidateItem{{
			SuggestedCanonicalULID: "01HEVENT000000000000000001",
			Name:                   "Family Storytime",
			EventCount:             2,
			Members: []SeriesCandidateMember{
				{ULID: "01HEVENT000000000000000001", Name: "Family Storytime"},
				{ULID: "01HEVENT000000000000000002", Name: "Family Storytime"},
			},
		}},
	}
	var out bytes.Buffer
	printSeriesCandidates(&out, resp, 204)

	want := "205 series candidate(s) (showing 205-205 of 205)"
	if !strings.Contains(out.String(), want) {
		t.Errorf("last-page header missing %q\n---\n%s", want, out.String())
	}
	if strings.Contains(out.String(), "next: --offset") {
		t.Errorf("last page must not suggest a next offset:\n%s", out.String())
	}
}

func TestValidateSeriesFlagsOffset(t *testing.T) {
	old := seriesOffset
	defer func() { seriesOffset = old }()

	for _, invalid := range []int{-1, 10001} {
		seriesOffset = invalid
		if err := validateSeriesFlags(); err == nil {
			t.Errorf("offset %d should fail client-side validation", invalid)
		}
	}

	for _, valid := range []int{0, 1, 10000} {
		seriesOffset = valid
		if err := validateSeriesFlags(); err != nil {
			t.Errorf("offset %d should be accepted: %v", valid, err)
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
	printSeriesCandidates(&out, resp, 0)
	if strings.Contains(out.String(), "names differ") {
		t.Errorf("trimmed/case-insensitive names must not trigger the divergence warning:\n%s", out.String())
	}
}

func TestPrintSeriesCandidatesEmpty(t *testing.T) {
	var out bytes.Buffer
	printSeriesCandidates(&out, SeriesCandidatesResponse{}, 0)

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
