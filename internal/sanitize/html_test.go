package sanitize

import (
	"testing"
)

func TestText_RemovesAllHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "script tag",
			input:    `Hello <script>alert('xss')</script> World`,
			expected: `Hello  World`,
		},
		{
			name:     "inline event handler",
			input:    `<div onclick="alert('xss')">Click me</div>`,
			expected: `Click me`,
		},
		{
			name:     "iframe injection",
			input:    `Safe text <iframe src="evil.com"></iframe> more text`,
			expected: `Safe text  more text`,
		},
		{
			name:     "style tag with expression",
			input:    `<style>body{background:url(javascript:alert('xss'))}</style>Text`,
			expected: `Text`,
		},
		{
			name:     "mixed HTML tags",
			input:    `<b>Bold</b> <i>Italic</i> <a href="http://example.com">Link</a>`,
			expected: `Bold Italic Link`,
		},
		{
			name:     "plain text unchanged",
			input:    `Just plain text`,
			expected: `Just plain text`,
		},
		{
			name:     "empty string",
			input:    ``,
			expected: ``,
		},
		{
			name:     "image tag with onerror",
			input:    `<img src=x onerror="alert('xss')">`,
			expected: ``,
		},
		{
			name:     "svg with script",
			input:    `<svg onload="alert('xss')"><script>alert(1)</script></svg>`,
			expected: ``,
		},
		{
			name:     "data URI",
			input:    `<a href="data:text/html,<script>alert('xss')</script>">Click</a>`,
			expected: `Click`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Text(tt.input)
			if result != tt.expected {
				t.Errorf("Text(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestHTML_AllowsSafeFormatting(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes script tags",
			input:    `<p>Hello <script>alert('xss')</script> World</p>`,
			expected: `<p>Hello  World</p>`,
		},
		{
			name:     "removes inline event handlers",
			input:    `<p onclick="alert('xss')">Click me</p>`,
			expected: `<p>Click me</p>`,
		},
		{
			name:     "removes iframe",
			input:    `<p>Safe text <iframe src="evil.com"></iframe> more</p>`,
			expected: `<p>Safe text  more</p>`,
		},
		{
			name:     "allows basic formatting",
			input:    `<p><b>Bold</b> <i>Italic</i> <em>Emphasis</em> <strong>Strong</strong></p>`,
			expected: `<p><b>Bold</b> <i>Italic</i> <em>Emphasis</em> <strong>Strong</strong></p>`,
		},
		{
			name:     "allows safe links",
			input:    `<p><a href="https://example.com">Link</a></p>`,
			expected: `<p><a href="https://example.com" rel="nofollow">Link</a></p>`,
		},
		{
			name:     "allows lists",
			input:    `<ul><li>Item 1</li><li>Item 2</li></ul>`,
			expected: `<ul><li>Item 1</li><li>Item 2</li></ul>`,
		},
		{
			name:     "allows br tags",
			input:    `Line 1<br>Line 2`,
			expected: `Line 1<br>Line 2`,
		},
		{
			name:     "removes dangerous link protocols",
			input:    `<a href="javascript:alert('xss')">Click</a>`,
			expected: `Click`,
		},
		{
			name:     "removes style attributes",
			input:    `<p style="color:red; background:url(javascript:alert(1))">Text</p>`,
			expected: `<p>Text</p>`,
		},
		{
			name:     "plain text unchanged",
			input:    `Just plain text`,
			expected: `Just plain text`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTML(tt.input)
			if result != tt.expected {
				t.Errorf("HTML(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTextSlice_SanitizesAllElements(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "multiple strings with HTML",
			input:    []string{"<b>Item 1</b>", "<script>alert(1)</script>Item 2", "Plain text"},
			expected: []string{"Item 1", "Item 2", "Plain text"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name:     "single element",
			input:    []string{"<i>Single</i>"},
			expected: []string{"Single"},
		},
		{
			name:     "XSS attempts in keywords",
			input:    []string{"concert", "<script>alert('xss')</script>music", "live<img src=x onerror=alert(1)>"},
			expected: []string{"concert", "music", "live"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TextSlice(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("TextSlice(%v) returned %d elements, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("TextSlice(%v)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

// Benchmark tests to ensure sanitization is performant
func BenchmarkText_ShortString(b *testing.B) {
	input := "Concert at <b>The Venue</b>"
	for i := 0; i < b.N; i++ {
		Text(input)
	}
}

func BenchmarkText_LongString(b *testing.B) {
	lorem := "Lorem ipsum dolor sit amet. "
	repeated := ""
	for i := 0; i < 10; i++ {
		repeated += lorem
	}
	input := "<p>This is a much longer event description with <b>bold text</b>, <i>italic text</i>, " +
		"<a href='http://example.com'>links</a>, and various <script>alert('xss')</script> attempts " +
		"to inject malicious code. " + repeated
	for i := 0; i < b.N; i++ {
		Text(input)
	}
}

func BenchmarkHTML_ShortString(b *testing.B) {
	input := "<p>Concert at <b>The Venue</b></p>"
	for i := 0; i < b.N; i++ {
		HTML(input)
	}
}

func BenchmarkHTML_LongString(b *testing.B) {
	lorem := "<p>Lorem ipsum dolor sit amet.</p>"
	repeated := ""
	for i := 0; i < 10; i++ {
		repeated += lorem
	}
	input := "<p>This is a much longer event description with <b>bold text</b>, <i>italic text</i>, " +
		"<a href='http://example.com'>links</a>, and various <script>alert('xss')</script> attempts " +
		"to inject malicious code.</p>" + repeated
	for i := 0; i < b.N; i++ {
		HTML(input)
	}
}

func BenchmarkTextSlice_10Elements(b *testing.B) {
	input := []string{
		"concert", "music", "live", "<script>xss</script>rock",
		"festival", "tickets<img src=x>", "venue", "performance",
		"band<iframe>", "show",
	}
	for i := 0; i < b.N; i++ {
		TextSlice(input)
	}
}

// Test real-world XSS attack vectors
func TestText_CommonXSSVectors(t *testing.T) {
	vectors := []struct {
		name  string
		input string
	}{
		{"Basic XSS", `<script>alert('XSS')</script>`},
		{"IMG onerror", `<img src=x onerror=alert('XSS')>`},
		{"IMG with quotes", `<img src="x" onerror="alert('XSS')">`},
		{"SVG onload", `<svg onload=alert('XSS')>`},
		{"Body onload", `<body onload=alert('XSS')>`},
		{"Input autofocus", `<input autofocus onfocus=alert('XSS')>`},
		{"Marquee onstart", `<marquee onstart=alert('XSS')>text</marquee>`},
		{"Details ontoggle", `<details ontoggle=alert('XSS')><summary>Click</summary></details>`},
		{"JavaScript protocol", `<a href="javascript:alert('XSS')">Click</a>`},
		{"Data URI", `<a href="data:text/html,<script>alert('XSS')</script>">Click</a>`},
		{"Meta refresh", `<meta http-equiv="refresh" content="0;url=javascript:alert('XSS')">`},
		{"Object data", `<object data="javascript:alert('XSS')">`},
		{"Embed src", `<embed src="javascript:alert('XSS')">`},
		{"Form action", `<form action="javascript:alert('XSS')"><input type="submit"></form>`},
	}

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			result := Text(v.input)
			// Sanitized result should not contain 'alert', 'script', or 'javascript'
			dangerous := []string{"alert", "javascript:", "<script"}
			for _, d := range dangerous {
				if contains(result, d) {
					t.Errorf("Text(%q) still contains dangerous content %q: %q", v.input, d, result)
				}
			}
		})
	}
}

func TestHTML_CommonXSSVectors(t *testing.T) {
	vectors := []struct {
		name  string
		input string
	}{
		{"Script tag", `<p><script>alert('XSS')</script>Text</p>`},
		{"Inline handler", `<p onclick="alert('XSS')">Text</p>`},
		{"Style with expression", `<p style="background:expression(alert('XSS'))">Text</p>`},
		{"IMG onerror", `<p><img src=x onerror=alert('XSS')>Text</p>`},
		{"JavaScript href", `<p><a href="javascript:alert('XSS')">Link</a></p>`},
	}

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			result := HTML(v.input)
			// Sanitized result should not contain dangerous JavaScript
			dangerous := []string{"alert", "javascript:", "<script", "onerror=", "onclick=", "onload="}
			for _, d := range dangerous {
				if contains(result, d) {
					t.Errorf("HTML(%q) still contains dangerous content %q: %q", v.input, d, result)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
