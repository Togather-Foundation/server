package events

// review_warning_policy.go — per-source suppression of review-gating warnings.
//
// Every warning is always generated and passed to the caller/ingest pipeline.
// The suppression list only decides whether a warning is allowed to push an
// event into pending_review. This exists because some sources (e.g. public
// library calendars) legitimately publish long-running recurring programmes
// that trip the cross-week-series and multi-session heuristics on every single
// event, which would otherwise flood the review queue.
//
// Sources set the list in their scraper YAML config:
//
//	suppress_review_warnings:
//	  - cross_week_series_companion
//
// The list travels to the ingest API on EventInput.SuppressReviewWarnings.
//
// Trust model: API payloads are NOT trusted. Only the small
// payload-suppressible allowlist below may be suppressed from a payload (and
// therefore from a scraper source config). Integrity and duplicate signals
// (exact_duplicate, potential_duplicate, near_duplicate_of_new_event,
// outside_geo_boundary, place/organ possible duplicates, reversed-date
// corrections, zero_duration_occurrence) can never be suppressed. Unknown and
// non-suppressible codes are rejected at the ingest boundary.

import (
	"fmt"
	"sort"
	"strings"
)

// knownReviewWarningCodes is every warning code this package can emit. It is
// used for validation messages only; a code being known does NOT mean it can
// be suppressed. See payloadSuppressibleReviewWarningCodes.
var knownReviewWarningCodes = []string{
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

// payloadSuppressibleReviewWarningCodes is the allowlist a payload (and a
// scraper source config) may suppress. They are benign heuristics about shape
// and data quality: suppressing them cannot hide a duplicate, publish an event
// outside the geographic boundary, or publish a mis-dated event.
//
// Date-plausibility codes (too_far_future, ambiguous_year_far_future) are NOT
// suppressible: they are the only guard against parser year errors (e.g. a
// source year "2026" turning into "2062"), and an agent key must not be able to
// publish mis-dated events without review.
//
// missing_description, missing_image and low_confidence are included on
// purpose: they describe content quality the caller already controls, and a
// caller can trivially avoid them by sending the field. They do not affect
// duplicate detection or territorial safety.
var payloadSuppressibleReviewWarningCodes = []string{
	"cross_week_series_companion",
	"low_confidence",
	"missing_description",
	"missing_image",
	"multi_session_likely",
}

// KnownReviewWarningCodes returns a sorted copy of every warning code this
// package can emit.
func KnownReviewWarningCodes() []string {
	out := make([]string, len(knownReviewWarningCodes))
	copy(out, knownReviewWarningCodes)
	sort.Strings(out)
	return out
}

// IsKnownReviewWarningCode reports whether code is a recognised warning code.
func IsKnownReviewWarningCode(code string) bool {
	_, ok := knownCodeSet[code]
	return ok
}

// PayloadSuppressibleReviewWarningCodes returns a sorted copy of the codes a
// payload or scraper source config may suppress.
func PayloadSuppressibleReviewWarningCodes() []string {
	out := make([]string, len(payloadSuppressibleReviewWarningCodes))
	copy(out, payloadSuppressibleReviewWarningCodes)
	sort.Strings(out)
	return out
}

// IsPayloadSuppressibleReviewWarningCode reports whether code may be suppressed
// from a payload or scraper source config.
func IsPayloadSuppressibleReviewWarningCode(code string) bool {
	_, ok := payloadSuppressibleCodeSet[code]
	return ok
}

var knownCodeSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(knownReviewWarningCodes))
	for _, code := range knownReviewWarningCodes {
		set[code] = struct{}{}
	}
	return set
}()

var payloadSuppressibleCodeSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(payloadSuppressibleReviewWarningCodes))
	for _, code := range payloadSuppressibleReviewWarningCodes {
		set[code] = struct{}{}
	}
	return set
}()

// NormalizeReviewWarningCodes trims, de-duplicates and sorts a suppression
// list. Normalizing keeps dedup-hash inputs stable and avoids spurious 409s
// caused only by list order or repeated entries.
func NormalizeReviewWarningCodes(codes []string) []string {
	if len(codes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(codes))
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

// ValidateSuppressReviewWarnings checks a suppression list against the
// payload-suppressible allowlist. It returns an error naming the offending
// entry; callers wrap it in a ValidationError (ingest) or append it to config
// validation errors (scraper sync).
func ValidateSuppressReviewWarnings(codes []string) error {
	allowed := strings.Join(PayloadSuppressibleReviewWarningCodes(), ", ")
	for i, code := range codes {
		trimmed := strings.TrimSpace(code)
		switch {
		case trimmed == "":
			return fmt.Errorf("entry %d: must not be empty", i)
		case !IsKnownReviewWarningCode(trimmed):
			return fmt.Errorf("entry %d: unknown review warning code %q (suppressible: %s)", i, code, allowed)
		case !IsPayloadSuppressibleReviewWarningCode(trimmed):
			return fmt.Errorf("entry %d: review warning code %q cannot be suppressed by a payload (suppressible: %s)", i, code, allowed)
		}
	}
	return nil
}

// suppressedWarningSet builds the look-up set used to decide whether a warning
// code gates review. Only payload-suppressible codes are admitted: even if a
// caller bypasses validation, integrity signals cannot be suppressed.
func suppressedWarningSet(codes []string) map[string]struct{} {
	normalized := NormalizeReviewWarningCodes(codes)
	if len(normalized) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(normalized))
	for _, code := range normalized {
		if IsPayloadSuppressibleReviewWarningCode(code) {
			set[code] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

// warningRequiresReview reports whether a single warning code should gate the
// event. A code the payload suppressed is ignored for gating (but the warning
// is still generated and passed along); everything else gates.
func warningRequiresReview(code string, suppressed map[string]struct{}) bool {
	if code == "" || len(suppressed) == 0 {
		return true
	}
	_, skip := suppressed[code]
	return !skip
}

// warningsRequireReview reports whether any warning in the slice should push
// the event into review.
func warningsRequireReview(warnings []ValidationWarning, suppressed map[string]struct{}) bool {
	for _, w := range warnings {
		if warningRequiresReview(w.Code, suppressed) {
			return true
		}
	}
	return false
}

// ignoredReviewWarningCodes returns the sorted, de-duplicated warning codes that
// are present in warnings and were ignored for gating because the payload
// suppressed them. An empty result means suppression had no effect.
func ignoredReviewWarningCodes(warnings []ValidationWarning, suppressed map[string]struct{}) []string {
	if len(suppressed) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(suppressed))
	var out []string
	for _, w := range warnings {
		if _, skip := suppressed[w.Code]; !skip {
			continue
		}
		if _, dup := seen[w.Code]; dup {
			continue
		}
		seen[w.Code] = struct{}{}
		out = append(out, w.Code)
	}
	sort.Strings(out)
	return out
}
