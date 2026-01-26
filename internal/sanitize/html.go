package sanitize

import (
	"github.com/microcosm-cc/bluemonday"
)

var (
	// StrictPolicy removes all HTML tags and attributes.
	// Use for fields that should only contain plain text (names, keywords).
	StrictPolicy = bluemonday.StrictPolicy()

	// UGCPolicy allows safe user-generated content with basic formatting.
	// Permits: <p>, <b>, <i>, <em>, <strong>, <a>, <ul>, <ol>, <li>, <br>
	// Use for fields where basic formatting is acceptable (descriptions).
	UGCPolicy = bluemonday.UGCPolicy()
)

// Text strips all HTML tags and returns plain text.
// Use for: event names, keywords, URLs, domain names.
func Text(input string) string {
	return StrictPolicy.Sanitize(input)
}

// HTML sanitizes HTML content, allowing safe formatting tags.
// Use for: event descriptions, organization bios.
// Removes: <script>, <iframe>, onclick handlers, style attributes.
func HTML(input string) string {
	return UGCPolicy.Sanitize(input)
}

// TextSlice sanitizes each string in a slice, removing all HTML.
func TextSlice(inputs []string) []string {
	if inputs == nil {
		return nil
	}
	sanitized := make([]string, len(inputs))
	for i, input := range inputs {
		sanitized[i] = Text(input)
	}
	return sanitized
}
