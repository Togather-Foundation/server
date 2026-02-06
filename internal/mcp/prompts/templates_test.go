package prompts

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestGetArgString(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]string
		key      string
		expected string
	}{
		{
			name:     "nil map returns empty string",
			args:     nil,
			key:      "any_key",
			expected: "",
		},
		{
			name:     "empty map returns empty string",
			args:     map[string]string{},
			key:      "missing",
			expected: "",
		},
		{
			name:     "missing key returns empty string",
			args:     map[string]string{"present": "value"},
			key:      "missing",
			expected: "",
		},
		{
			name:     "existing key returns value",
			args:     map[string]string{"key": "value"},
			key:      "key",
			expected: "value",
		},
		{
			name:     "empty value returns empty string",
			args:     map[string]string{"key": ""},
			key:      "key",
			expected: "",
		},
		{
			name:     "multiple keys returns correct value",
			args:     map[string]string{"first": "alpha", "second": "beta"},
			key:      "second",
			expected: "beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getArgString(tt.args, tt.key)
			if result != tt.expected {
				t.Errorf("getArgString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestPromptTemplates_CreateEventFromTextPrompt(t *testing.T) {
	p := NewPromptTemplates()
	prompt := p.CreateEventFromTextPrompt()

	if prompt.Name != createEventFromTextPrompt {
		t.Errorf("Name = %q, want %q", prompt.Name, createEventFromTextPrompt)
	}

	if prompt.Description == "" {
		t.Error("Description should not be empty")
	}

	if len(prompt.Arguments) != 2 {
		t.Errorf("Arguments count = %d, want 2", len(prompt.Arguments))
	}
}

func TestPromptTemplates_FindVenuePrompt(t *testing.T) {
	p := NewPromptTemplates()
	prompt := p.FindVenuePrompt()

	if prompt.Name != findVenuePrompt {
		t.Errorf("Name = %q, want %q", prompt.Name, findVenuePrompt)
	}

	if prompt.Description == "" {
		t.Error("Description should not be empty")
	}

	if len(prompt.Arguments) != 3 {
		t.Errorf("Arguments count = %d, want 3", len(prompt.Arguments))
	}
}

func TestPromptTemplates_DuplicateCheckPrompt(t *testing.T) {
	p := NewPromptTemplates()
	prompt := p.DuplicateCheckPrompt()

	if prompt.Name != duplicateCheckPrompt {
		t.Errorf("Name = %q, want %q", prompt.Name, duplicateCheckPrompt)
	}

	if prompt.Description == "" {
		t.Error("Description should not be empty")
	}

	if len(prompt.Arguments) != 3 {
		t.Errorf("Arguments count = %d, want 3", len(prompt.Arguments))
	}
}

func TestPromptTemplates_CreateEventFromTextHandler(t *testing.T) {
	p := NewPromptTemplates()
	ctx := context.Background()

	tests := []struct {
		name             string
		args             map[string]string
		wantTimezone     string
		wantDescription  string
		wantErrNil       bool
		wantMessageCount int
	}{
		{
			name: "with all arguments",
			args: map[string]string{
				"description":      "Community potluck dinner at 6pm",
				"default_timezone": "America/Toronto",
			},
			wantTimezone:     "America/Toronto",
			wantDescription:  "Community potluck dinner at 6pm",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name: "missing timezone defaults to UTC",
			args: map[string]string{
				"description": "Tech meetup tomorrow",
			},
			wantTimezone:     "UTC",
			wantDescription:  "Tech meetup tomorrow",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name:             "nil arguments defaults to UTC",
			args:             nil,
			wantTimezone:     "UTC",
			wantDescription:  "",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name:             "empty arguments defaults to UTC",
			args:             map[string]string{},
			wantTimezone:     "UTC",
			wantDescription:  "",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name: "empty timezone string defaults to UTC",
			args: map[string]string{
				"description":      "Event",
				"default_timezone": "",
			},
			wantTimezone:     "UTC",
			wantDescription:  "Event",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.GetPromptRequest{
				Params: mcp.GetPromptParams{
					Name:      createEventFromTextPrompt,
					Arguments: tt.args,
				},
			}

			result, err := p.CreateEventFromTextHandler(ctx, request)

			if (err == nil) != tt.wantErrNil {
				t.Errorf("CreateEventFromTextHandler() error = %v, wantErrNil %v", err, tt.wantErrNil)
				return
			}

			if result == nil {
				t.Fatal("CreateEventFromTextHandler() result is nil")
			}

			if len(result.Messages) != tt.wantMessageCount {
				t.Errorf("Messages count = %d, want %d", len(result.Messages), tt.wantMessageCount)
			}

			if len(result.Messages) > 0 {
				msg := result.Messages[0]
				if msg.Role != mcp.RoleUser {
					t.Errorf("Message role = %v, want %v", msg.Role, mcp.RoleUser)
				}

				// Try to cast to TextContent
				textContent, ok := mcp.AsTextContent(msg.Content)
				if !ok {
					t.Fatal("Content is not TextContent")
				}

				if textContent.Type != "text" {
					t.Errorf("Content type = %q, want %q", textContent.Type, "text")
				}

				text := textContent.Text

				// Check that timezone appears in the message
				if !contains(text, tt.wantTimezone) {
					t.Errorf("Message does not contain timezone %q: %q", tt.wantTimezone, text)
				}

				// Check that description appears in the message (if provided)
				if tt.wantDescription != "" && !contains(text, tt.wantDescription) {
					t.Errorf("Message does not contain description %q: %q", tt.wantDescription, text)
				}
			}
		})
	}
}

func TestPromptTemplates_FindVenueHandler(t *testing.T) {
	p := NewPromptTemplates()
	ctx := context.Background()

	tests := []struct {
		name             string
		args             map[string]string
		wantRequirements string
		wantLocation     string
		wantCapacity     string
		wantErrNil       bool
		wantMessageCount int
	}{
		{
			name: "with all arguments",
			args: map[string]string{
				"requirements": "wheelchair accessible, projector",
				"location":     "downtown Toronto",
				"capacity":     "50",
			},
			wantRequirements: "wheelchair accessible, projector",
			wantLocation:     "downtown Toronto",
			wantCapacity:     "50",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name: "with missing location",
			args: map[string]string{
				"requirements": "outdoor space",
				"capacity":     "100",
			},
			wantRequirements: "outdoor space",
			wantLocation:     "",
			wantCapacity:     "100",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name:             "nil arguments",
			args:             nil,
			wantRequirements: "",
			wantLocation:     "",
			wantCapacity:     "",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name:             "empty arguments",
			args:             map[string]string{},
			wantRequirements: "",
			wantLocation:     "",
			wantCapacity:     "",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.GetPromptRequest{
				Params: mcp.GetPromptParams{
					Name:      findVenuePrompt,
					Arguments: tt.args,
				},
			}

			result, err := p.FindVenueHandler(ctx, request)

			if (err == nil) != tt.wantErrNil {
				t.Errorf("FindVenueHandler() error = %v, wantErrNil %v", err, tt.wantErrNil)
				return
			}

			if result == nil {
				t.Fatal("FindVenueHandler() result is nil")
			}

			if len(result.Messages) != tt.wantMessageCount {
				t.Errorf("Messages count = %d, want %d", len(result.Messages), tt.wantMessageCount)
			}

			if len(result.Messages) > 0 {
				msg := result.Messages[0]
				if msg.Role != mcp.RoleUser {
					t.Errorf("Message role = %v, want %v", msg.Role, mcp.RoleUser)
				}

				// Try to cast to TextContent
				textContent, ok := mcp.AsTextContent(msg.Content)
				if !ok {
					t.Fatal("Content is not TextContent")
				}

				if textContent.Type != "text" {
					t.Errorf("Content type = %q, want %q", textContent.Type, "text")
				}

				text := textContent.Text

				// Verify expected values appear in message (even empty ones are formatted into the text)
				if tt.wantRequirements != "" && !contains(text, tt.wantRequirements) {
					t.Errorf("Message does not contain requirements %q: %q", tt.wantRequirements, text)
				}
			}
		})
	}
}

func TestPromptTemplates_DuplicateCheckHandler(t *testing.T) {
	p := NewPromptTemplates()
	ctx := context.Background()

	tests := []struct {
		name             string
		args             map[string]string
		wantDescription  string
		wantDate         string
		wantLocation     string
		wantErrNil       bool
		wantMessageCount int
	}{
		{
			name: "with all arguments",
			args: map[string]string{
				"event_description": "Annual Tech Conference",
				"date":              "2026-03-15",
				"location":          "Metro Convention Center",
			},
			wantDescription:  "Annual Tech Conference",
			wantDate:         "2026-03-15",
			wantLocation:     "Metro Convention Center",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name: "with missing date",
			args: map[string]string{
				"event_description": "Book Club Meeting",
				"location":          "Public Library",
			},
			wantDescription:  "Book Club Meeting",
			wantDate:         "",
			wantLocation:     "Public Library",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name:             "nil arguments",
			args:             nil,
			wantDescription:  "",
			wantDate:         "",
			wantLocation:     "",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name:             "empty arguments",
			args:             map[string]string{},
			wantDescription:  "",
			wantDate:         "",
			wantLocation:     "",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
		{
			name: "with empty string values",
			args: map[string]string{
				"event_description": "",
				"date":              "",
				"location":          "",
			},
			wantDescription:  "",
			wantDate:         "",
			wantLocation:     "",
			wantErrNil:       true,
			wantMessageCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.GetPromptRequest{
				Params: mcp.GetPromptParams{
					Name:      duplicateCheckPrompt,
					Arguments: tt.args,
				},
			}

			result, err := p.DuplicateCheckHandler(ctx, request)

			if (err == nil) != tt.wantErrNil {
				t.Errorf("DuplicateCheckHandler() error = %v, wantErrNil %v", err, tt.wantErrNil)
				return
			}

			if result == nil {
				t.Fatal("DuplicateCheckHandler() result is nil")
			}

			if len(result.Messages) != tt.wantMessageCount {
				t.Errorf("Messages count = %d, want %d", len(result.Messages), tt.wantMessageCount)
			}

			if len(result.Messages) > 0 {
				msg := result.Messages[0]
				if msg.Role != mcp.RoleUser {
					t.Errorf("Message role = %v, want %v", msg.Role, mcp.RoleUser)
				}

				// Try to cast to TextContent
				textContent, ok := mcp.AsTextContent(msg.Content)
				if !ok {
					t.Fatal("Content is not TextContent")
				}

				if textContent.Type != "text" {
					t.Errorf("Content type = %q, want %q", textContent.Type, "text")
				}

				text := textContent.Text

				// Verify expected values appear in message when provided
				if tt.wantDescription != "" && !contains(text, tt.wantDescription) {
					t.Errorf("Message does not contain description %q: %q", tt.wantDescription, text)
				}
				if tt.wantDate != "" && !contains(text, tt.wantDate) {
					t.Errorf("Message does not contain date %q: %q", tt.wantDate, text)
				}
				if tt.wantLocation != "" && !contains(text, tt.wantLocation) {
					t.Errorf("Message does not contain location %q: %q", tt.wantLocation, text)
				}
			}
		})
	}
}

func TestNewPromptTemplates(t *testing.T) {
	p := NewPromptTemplates()
	if p == nil {
		t.Error("NewPromptTemplates() returned nil")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
