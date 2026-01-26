package handlers

import (
	"testing"
)

func TestMapToUpdateParams_SanitizesXSS(t *testing.T) {
	tests := []struct {
		name             string
		updates          map[string]any
		checkField       string
		wantSanitized    string
		shouldContain    string
		shouldNotContain []string
	}{
		{
			name: "name with script tag",
			updates: map[string]any{
				"name": "Concert <script>alert('xss')</script> Event",
			},
			checkField:       "name",
			wantSanitized:    "Concert  Event",
			shouldNotContain: []string{"<script>", "alert", "xss"},
		},
		{
			name: "description with safe HTML",
			updates: map[string]any{
				"description": "<p>Join us for <b>live music</b> and <i>great food</i>!</p>",
			},
			checkField:    "description",
			wantSanitized: "<p>Join us for <b>live music</b> and <i>great food</i>!</p>",
			shouldContain: "<b>",
		},
		{
			name: "description with XSS attempt",
			updates: map[string]any{
				"description": "<p>Event description</p><script>alert('xss')</script>",
			},
			checkField:       "description",
			wantSanitized:    "<p>Event description</p>",
			shouldNotContain: []string{"<script>", "alert"},
		},
		{
			name: "description with inline event handler",
			updates: map[string]any{
				"description": "<p onclick='alert(1)'>Click here</p>",
			},
			checkField:       "description",
			wantSanitized:    "<p>Click here</p>",
			shouldNotContain: []string{"onclick", "alert"},
		},
		{
			name: "keywords with XSS",
			updates: map[string]any{
				"keywords": []any{"music", "<script>xss</script>concert", "live<img src=x>"},
			},
			checkField:       "keywords",
			shouldNotContain: []string{"<script>", "<img"},
		},
		{
			name: "image_url with XSS",
			updates: map[string]any{
				"image_url": "https://example.com/image.jpg<script>alert(1)</script>",
			},
			checkField:       "image_url",
			wantSanitized:    "https://example.com/image.jpg",
			shouldNotContain: []string{"<script>", "alert"},
		},
		{
			name: "lifecycle_state with HTML",
			updates: map[string]any{
				"lifecycle_state": "published<b>test</b>",
			},
			checkField:       "lifecycle_state",
			wantSanitized:    "publishedtest",
			shouldNotContain: []string{"<b>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := mapToUpdateParams(tt.updates)

			// Check the sanitized value
			switch tt.checkField {
			case "name":
				if params.Name == nil {
					t.Fatal("Name should not be nil")
				}
				if *params.Name != tt.wantSanitized {
					t.Errorf("Name = %q, want %q", *params.Name, tt.wantSanitized)
				}
				for _, bad := range tt.shouldNotContain {
					if contains(*params.Name, bad) {
						t.Errorf("Name still contains dangerous content %q: %q", bad, *params.Name)
					}
				}
			case "description":
				if params.Description == nil {
					t.Fatal("Description should not be nil")
				}
				if *params.Description != tt.wantSanitized {
					t.Errorf("Description = %q, want %q", *params.Description, tt.wantSanitized)
				}
				if tt.shouldContain != "" && !contains(*params.Description, tt.shouldContain) {
					t.Errorf("Description should contain %q but got %q", tt.shouldContain, *params.Description)
				}
				for _, bad := range tt.shouldNotContain {
					if contains(*params.Description, bad) {
						t.Errorf("Description still contains dangerous content %q: %q", bad, *params.Description)
					}
				}
			case "keywords":
				if params.Keywords == nil {
					t.Fatal("Keywords should not be nil")
				}
				for _, kw := range params.Keywords {
					for _, bad := range tt.shouldNotContain {
						if contains(kw, bad) {
							t.Errorf("Keyword %q still contains dangerous content %q", kw, bad)
						}
					}
				}
			case "image_url":
				if params.ImageURL == nil {
					t.Fatal("ImageURL should not be nil")
				}
				if *params.ImageURL != tt.wantSanitized {
					t.Errorf("ImageURL = %q, want %q", *params.ImageURL, tt.wantSanitized)
				}
			case "lifecycle_state":
				if params.LifecycleState == nil {
					t.Fatal("LifecycleState should not be nil")
				}
				if *params.LifecycleState != tt.wantSanitized {
					t.Errorf("LifecycleState = %q, want %q", *params.LifecycleState, tt.wantSanitized)
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
