package events

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Togather-Foundation/server/internal/config"
)

func TestIsKnownReviewWarningCode(t *testing.T) {
	t.Parallel()

	for _, code := range []string{
		"cross_week_series_companion",
		"multi_session_likely",
		"potential_duplicate",
		"near_duplicate_of_new_event",
		"too_far_future",
		"outside_geo_boundary",
		"missing_image",
		"missing_description",
	} {
		if !IsKnownReviewWarningCode(code) {
			t.Errorf("IsKnownReviewWarningCode(%q) = false, want true", code)
		}
	}

	for _, code := range []string{"", "not_a_code", "CROSS_WEEK_SERIES_COMPANION"} {
		if IsKnownReviewWarningCode(code) {
			t.Errorf("IsKnownReviewWarningCode(%q) = true, want false", code)
		}
	}

	wantKnown := []string{
		"ambiguous_year_far_future",
		"cross_week_series_companion",
		"exact_duplicate",
		"link_check_failed",
		"low_confidence",
		"missing_description",
		"missing_image",
		"multi_session_likely",
		"near_duplicate_of_new_event",
		"org_possible_duplicate",
		"outside_geo_boundary",
		"place_possible_duplicate",
		"potential_duplicate",
		"reversed_dates_corrected_needs_review",
		"reversed_dates_timezone_likely",
		"too_far_future",
		"zero_duration_occurrence",
	}
	if got := KnownReviewWarningCodes(); !reflect.DeepEqual(got, wantKnown) {
		t.Errorf("KnownReviewWarningCodes() = %v, want %v", got, wantKnown)
	}
}

func TestWarningsRequireReview(t *testing.T) {
	t.Parallel()

	warnings := []ValidationWarning{
		{Code: "cross_week_series_companion"},
		{Code: "missing_description"},
	}

	tests := []struct {
		name     string
		warnings []ValidationWarning
		suppress []string
		want     bool
	}{
		{
			name:     "no warnings",
			warnings: nil,
			want:     false,
		},
		{
			name:     "unsuppressed warning gates",
			warnings: warnings[:1],
			want:     true,
		},
		{
			name:     "only suppressed warning does not gate",
			warnings: warnings[:1],
			suppress: []string{"cross_week_series_companion"},
			want:     false,
		},
		{
			name:     "one unsuppressed warning still gates",
			warnings: warnings,
			suppress: []string{"cross_week_series_companion"},
			want:     true,
		},
		{
			name:     "all warnings suppressed does not gate",
			warnings: warnings,
			suppress: []string{"cross_week_series_companion", "missing_description"},
			want:     false,
		},
		{
			name:     "unrelated suppression does not silence the warning",
			warnings: warnings[:1],
			suppress: []string{"missing_image"},
			want:     true,
		},
		{
			name:     "whitespace in suppress entries is ignored",
			warnings: warnings[:1],
			suppress: []string{"  cross_week_series_companion  "},
			want:     false,
		},
		{
			name:     "empty warning code always gates",
			warnings: []ValidationWarning{{Code: ""}},
			suppress: []string{"cross_week_series_companion"},
			want:     true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := warningsRequireReview(tt.warnings, suppressedWarningSet(tt.suppress))
			if got != tt.want {
				t.Errorf("warningsRequireReview() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEventNeedsReview_SuppressReviewWarnings(t *testing.T) {
	t.Parallel()

	cfg := config.ValidationConfig{}.WithDefaults()

	base := EventInput{
		Name:        "Weekly Pottery",
		Description: "A recurring programme",
		StartDate:   "2026-12-01T19:00:00Z",
		EndDate:     "2026-12-01T21:00:00Z",
		License:     "CC0-1.0",
	}

	if !eventNeedsReview(base, nil, cfg) {
		t.Fatal("eventNeedsReview() = false for multi-session event, want true")
	}

	suppressed := base
	suppressed.SuppressReviewWarnings = []string{"multi_session_likely"}
	if eventNeedsReview(suppressed, nil, cfg) {
		t.Error("eventNeedsReview() = true when multi_session_likely is suppressed, want false")
	}

	// Suppressing an unrelated code must not silence the multi-session gate.
	unrelated := base
	unrelated.SuppressReviewWarnings = []string{"missing_image"}
	if !eventNeedsReview(unrelated, nil, cfg) {
		t.Error("eventNeedsReview() = false when only an unrelated code is suppressed, want true")
	}

	// missing_description is also gated through this helper, not just via the
	// warning slice.
	noDescription := EventInput{
		Name:      "Pottery Class",
		StartDate: "2026-12-01T19:00:00Z",
		EndDate:   "2026-12-01T21:00:00Z",
		License:   "CC0-1.0",
	}
	if !eventNeedsReview(noDescription, nil, cfg) {
		t.Fatal("eventNeedsReview() = false for missing description, want true")
	}
	noDescription.SuppressReviewWarnings = []string{"missing_description"}
	if eventNeedsReview(noDescription, nil, cfg) {
		t.Error("eventNeedsReview() = true when missing_description is suppressed, want false")
	}
}

func TestPayloadSuppressibleReviewWarningCodes_ExactSet(t *testing.T) {
	t.Parallel()

	want := []string{
		"cross_week_series_companion",
		"low_confidence",
		"missing_description",
		"missing_image",
		"multi_session_likely",
	}
	got := PayloadSuppressibleReviewWarningCodes()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PayloadSuppressibleReviewWarningCodes() = %v, want %v", got, want)
	}

	for _, code := range got {
		if !IsKnownReviewWarningCode(code) {
			t.Errorf("suppressible code %q missing from known registry", code)
		}
	}

	// Integrity and boundary signals must never be payload-suppressible.
	for _, code := range []string{
		"ambiguous_year_far_future",
		"too_far_future",
		"exact_duplicate",
		"potential_duplicate",
		"near_duplicate_of_new_event",
		"outside_geo_boundary",
		"place_possible_duplicate",
		"org_possible_duplicate",
		"reversed_dates_corrected_needs_review",
		"reversed_dates_timezone_likely",
		"zero_duration_occurrence",
		"link_check_failed",
	} {
		if !IsKnownReviewWarningCode(code) {
			t.Errorf("known code %q missing from registry", code)
		}
		if IsPayloadSuppressibleReviewWarningCode(code) {
			t.Errorf("IsPayloadSuppressibleReviewWarningCode(%q) = true, want false", code)
		}
	}
}

func TestValidateSuppressReviewWarnings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		codes   []string
		wantErr string
	}{
		{name: "empty list", codes: nil},
		{name: "allowed codes", codes: []string{"cross_week_series_companion", "multi_session_likely"}},
		{name: "whitespace tolerated", codes: []string{"  cross_week_series_companion  "}},
		{name: "empty entry", codes: []string{""}, wantErr: "must not be empty"},
		{name: "unknown code", codes: []string{"nope"}, wantErr: `unknown review warning code "nope"`},
		{name: "known but not suppressible", codes: []string{"potential_duplicate"}, wantErr: "cannot be suppressed by a payload"},
		{name: "far future date not suppressible", codes: []string{"too_far_future"}, wantErr: "cannot be suppressed by a payload"},
		{name: "ambiguous year not suppressible", codes: []string{"ambiguous_year_far_future"}, wantErr: "cannot be suppressed by a payload"},
		{name: "exact duplicate not suppressible", codes: []string{"exact_duplicate"}, wantErr: "cannot be suppressed by a payload"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSuppressReviewWarnings(tt.codes)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateSuppressReviewWarnings(%v) = %v, want nil", tt.codes, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateSuppressReviewWarnings(%v) = nil, want error containing %q", tt.codes, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestSuppressedWarningSet_RefusesNonPayloadSuppressibleCodes(t *testing.T) {
	t.Parallel()

	set := suppressedWarningSet([]string{
		"cross_week_series_companion",
		"exact_duplicate",
		"potential_duplicate",
	})

	if !warningRequiresReview("exact_duplicate", set) {
		t.Error("exact_duplicate must still gate even if present in the payload list")
	}
	if !warningRequiresReview("potential_duplicate", set) {
		t.Error("potential_duplicate must still gate even if present in the payload list")
	}
	if warningRequiresReview("cross_week_series_companion", set) {
		t.Error("cross_week_series_companion should be suppressed")
	}
}

func TestIgnoredReviewWarningCodes(t *testing.T) {
	t.Parallel()

	warnings := []ValidationWarning{
		{Code: "cross_week_series_companion"},
		{Code: "cross_week_series_companion"},
		{Code: "missing_description"},
		{Code: "potential_duplicate"},
	}
	set := suppressedWarningSet([]string{"missing_description", "cross_week_series_companion"})

	got := ignoredReviewWarningCodes(warnings, set)
	want := []string{"cross_week_series_companion", "missing_description"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ignoredReviewWarningCodes() = %v, want %v", got, want)
	}

	if got := ignoredReviewWarningCodes(warnings, nil); got != nil {
		t.Errorf("ignoredReviewWarningCodes(nil set) = %v, want nil", got)
	}
}

func TestValidateEventInputWithWarnings_SuppressReviewWarnings(t *testing.T) {
	t.Parallel()

	valid := validEventInput()

	t.Run("normalizes the list", func(t *testing.T) {
		t.Parallel()
		input := valid
		input.SuppressReviewWarnings = []string{
			" multi_session_likely ",
			"cross_week_series_companion",
			"multi_session_likely",
		}
		result, err := ValidateEventInputWithWarnings(input, "", nil, config.ValidationConfig{})
		if err != nil {
			t.Fatalf("ValidateEventInputWithWarnings() error = %v", err)
		}
		want := []string{"cross_week_series_companion", "multi_session_likely"}
		if !reflect.DeepEqual(result.Input.SuppressReviewWarnings, want) {
			t.Errorf("SuppressReviewWarnings = %v, want %v", result.Input.SuppressReviewWarnings, want)
		}
	})

	t.Run("rejects unknown code with validation error", func(t *testing.T) {
		t.Parallel()
		input := valid
		input.SuppressReviewWarnings = []string{"not_a_code"}
		_, err := ValidateEventInputWithWarnings(input, "", nil, config.ValidationConfig{})
		if err == nil {
			t.Fatal("expected error")
		}
		var ve ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("error type = %T, want ValidationError", err)
		}
		if ve.Field != "suppress_review_warnings" {
			t.Errorf("Field = %q, want suppress_review_warnings", ve.Field)
		}
		if !strings.Contains(ve.Message, `"not_a_code"`) {
			t.Errorf("Message = %q, want it to name the bad value", ve.Message)
		}
	})

	t.Run("rejects non-suppressible code with validation error", func(t *testing.T) {
		t.Parallel()
		input := valid
		input.SuppressReviewWarnings = []string{"exact_duplicate"}
		_, err := ValidateEventInputWithWarnings(input, "", nil, config.ValidationConfig{})
		if err == nil {
			t.Fatal("expected error")
		}
		var ve ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("error type = %T, want ValidationError", err)
		}
		if ve.Field != "suppress_review_warnings" {
			t.Errorf("Field = %q, want suppress_review_warnings", ve.Field)
		}
		if !strings.Contains(ve.Message, "cannot be suppressed") {
			t.Errorf("Message = %q, want it to explain the code is not suppressible", ve.Message)
		}
	})
}

func TestNormalizedInputForHash_IgnoresSuppressReviewWarnings(t *testing.T) {
	t.Parallel()

	base := EventInput{
		Name:      "Pottery Class",
		StartDate: "2026-12-01T19:00:00Z",
		License:   "CC0-1.0",
	}

	a := base
	a.SuppressReviewWarnings = []string{"cross_week_series_companion"}
	b := base
	b.SuppressReviewWarnings = []string{"missing_image", "multi_session_likely"}

	hashA, err := hashInput(normalizedInputForHash(a))
	if err != nil {
		t.Fatalf("hashInput(a): %v", err)
	}
	hashB, err := hashInput(normalizedInputForHash(b))
	if err != nil {
		t.Fatalf("hashInput(b): %v", err)
	}
	if hashA != hashB {
		t.Errorf("suppression list changed the request hash: %s != %s", hashA, hashB)
	}

	// Real event data must still change the hash.
	c := base
	c.Name = "Other Event"
	hashC, err := hashInput(normalizedInputForHash(c))
	if err != nil {
		t.Fatalf("hashInput(c): %v", err)
	}
	if hashC == hashA {
		t.Error("changing the event name did not change the request hash")
	}
}

func TestIngestWithIdempotency_SuppressReviewWarningsDoesNotConflict(t *testing.T) {
	repo := NewMockRepository()
	svc := NewIngestService(repo, "https://test.togather.ca", "America/Toronto",
		config.ValidationConfig{AllowTestDomains: true}, zerolog.Nop())

	start := time.Now().Add(48 * time.Hour).Format(time.RFC3339)
	base := EventInput{
		Name:        "Pottery Class",
		Description: "A hands-on pottery programme.",
		StartDate:   start,
		License:     "CC0-1.0",
		Location:    &PlaceInput{Name: "Test Venue"},
	}

	first := base
	first.SuppressReviewWarnings = []string{"cross_week_series_companion"}
	if _, err := svc.IngestWithIdempotency(context.Background(), first, "suppress-hash-key"); err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	second := base
	second.SuppressReviewWarnings = []string{"multi_session_likely"}
	result, err := svc.IngestWithIdempotency(context.Background(), second, "suppress-hash-key")
	if err != nil {
		t.Fatalf("second ingest with a different suppression list errored (should be a replay, not a conflict): %v", err)
	}
	if result == nil || !result.IsDuplicate {
		t.Error("second ingest should return the original event as a duplicate")
	}
}
