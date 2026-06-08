package scraper

import "strings"

// joinSelectorTexts iterates over CSS selectors, extracts text from each using
// the provided extract function, skips empty strings, and joins results with
// a single space. Returns empty string if selectors is nil/empty or all
// extractors return empty.
func joinSelectorTexts(selectors []string, extract func(selector string) string) string {
	if len(selectors) == 0 {
		return ""
	}

	var parts []string
	for _, sel := range selectors {
		text := extract(sel)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}
