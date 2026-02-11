package events

import "strings"

// AutoMergeFields computes which fields on an existing event should be updated
// based on the new submission and source trust levels.
//
// existingTrust: the highest trust level among sources already linked to this event
// newTrust: the trust level of the incoming source
//
// Trust levels are integers 1-10 where HIGHER values mean MORE trusted (10 = most trusted).
//
// Returns an UpdateEventParams with only the fields that should change,
// plus a boolean indicating whether any changes were made.
func AutoMergeFields(existing *Event, input EventInput, existingTrust int, newTrust int) (UpdateEventParams, bool) {
	var params UpdateEventParams
	changed := false

	// We do NOT merge Name (dedup hash matched on name; changing it would change the hash)
	// We do NOT merge LifecycleState

	// Description: fill gap or overwrite with higher trust
	if mergeStringField(existing.Description, input.Description, existingTrust, newTrust, &params.Description) {
		changed = true
	}

	// ImageURL: fill gap or overwrite with higher trust
	if mergeStringField(existing.ImageURL, input.Image, existingTrust, newTrust, &params.ImageURL) {
		changed = true
	}

	// PublicURL: fill gap or overwrite with higher trust
	if mergeStringField(existing.PublicURL, input.URL, existingTrust, newTrust, &params.PublicURL) {
		changed = true
	}

	// EventDomain: fill gap or overwrite with higher trust
	if mergeStringField(existing.EventDomain, input.EventDomain, existingTrust, newTrust, &params.EventDomain) {
		changed = true
	}

	// Keywords: fill gap or overwrite with higher trust
	if mergeKeywordsField(existing.Keywords, input.Keywords, existingTrust, newTrust, &params.Keywords) {
		changed = true
	}

	return params, changed
}

// mergeStringField applies the merge strategy for a single string field.
// - If existing is empty and new has data → fill (set target to new value)
// - If both have data and new has higher trust (lower number) → overwrite
// - Otherwise → keep existing (no change)
//
// Returns true if the field was changed.
func mergeStringField(existingVal, newVal string, existingTrust, newTrust int, target **string) bool {
	newTrimmed := strings.TrimSpace(newVal)
	if newTrimmed == "" {
		return false
	}

	existingTrimmed := strings.TrimSpace(existingVal)
	if existingTrimmed == "" {
		// Gap fill: existing is empty, new has data
		*target = &newTrimmed
		return true
	}

	// Both have values: only overwrite if new source has strictly higher trust (higher number)
	if newTrust > existingTrust {
		*target = &newTrimmed
		return true
	}

	return false
}

// mergeKeywordsField applies the merge strategy for the keywords slice.
// - If existing is empty and new has data → fill
// - If both have data and new has higher trust (higher number) → overwrite
// - Otherwise → keep existing
//
// Returns true if the field was changed.
func mergeKeywordsField(existingVal, newVal []string, existingTrust, newTrust int, target *[]string) bool {
	if len(newVal) == 0 {
		return false
	}

	if len(existingVal) == 0 {
		// Gap fill: existing is empty, new has data
		*target = newVal
		return true
	}

	// Both have values: only overwrite if new source has strictly higher trust (higher number)
	if newTrust > existingTrust {
		*target = newVal
		return true
	}

	return false
}
