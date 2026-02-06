package prompts

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	createEventFromTextPrompt = "create_event_from_text"
	findVenuePrompt           = "find_venue"
	duplicateCheckPrompt      = "duplicate_check"
)

type PromptTemplates struct{}

func NewPromptTemplates() *PromptTemplates {
	return &PromptTemplates{}
}

func (p *PromptTemplates) CreateEventFromTextPrompt() mcp.Prompt {
	return mcp.NewPrompt(
		createEventFromTextPrompt,
		mcp.WithPromptDescription("Parse natural language event details into a structured JSON-LD event payload"),
		mcp.WithArgument("description", mcp.ArgumentDescription("Raw event description text")),
		mcp.WithArgument("default_timezone", mcp.ArgumentDescription("IANA timezone (e.g. America/Toronto)")),
	)
}

func (p *PromptTemplates) FindVenuePrompt() mcp.Prompt {
	return mcp.NewPrompt(
		findVenuePrompt,
		mcp.WithPromptDescription("Find a suitable venue for an event based on requirements and location"),
		mcp.WithArgument("requirements", mcp.ArgumentDescription("Venue requirements (capacity, accessibility, amenities)")),
		mcp.WithArgument("location", mcp.ArgumentDescription("Preferred location or neighborhood")),
		mcp.WithArgument("capacity", mcp.ArgumentDescription("Expected attendee count")),
	)
}

func (p *PromptTemplates) DuplicateCheckPrompt() mcp.Prompt {
	return mcp.NewPrompt(
		duplicateCheckPrompt,
		mcp.WithPromptDescription("Check for potential duplicate events before creating a new one"),
		mcp.WithArgument("event_description", mcp.ArgumentDescription("Summary of the event")),
		mcp.WithArgument("date", mcp.ArgumentDescription("Proposed event date (YYYY-MM-DD)")),
		mcp.WithArgument("location", mcp.ArgumentDescription("Proposed event location")),
	)
}

func (p *PromptTemplates) CreateEventFromTextHandler(_ context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := request.Params.Arguments
	description := getArgString(args, "description")
	timezone := getArgString(args, "default_timezone")
	if timezone == "" {
		timezone = "UTC"
	}

	text := fmt.Sprintf("Convert the following event description into a JSON-LD event payload that conforms to the SEL schema. Use timezone %s for any times.\n\nEvent description:\n%s", timezone, description)

	return &mcp.GetPromptResult{
		Description: "Create JSON-LD event payload from natural language description",
		Messages: []mcp.PromptMessage{
			{
				Role:    mcp.RoleUser,
				Content: mcp.NewTextContent(text),
			},
		},
	}, nil
}

func (p *PromptTemplates) FindVenueHandler(_ context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := request.Params.Arguments
	requirements := getArgString(args, "requirements")
	location := getArgString(args, "location")
	capacity := getArgString(args, "capacity")

	text := fmt.Sprintf("Find a venue for an event with the following requirements. Provide a shortlist and explain why each fits.\n\nRequirements: %s\nLocation: %s\nCapacity: %s", requirements, location, capacity)

	return &mcp.GetPromptResult{
		Description: "Find a venue based on requirements and location",
		Messages: []mcp.PromptMessage{
			{
				Role:    mcp.RoleUser,
				Content: mcp.NewTextContent(text),
			},
		},
	}, nil
}

func (p *PromptTemplates) DuplicateCheckHandler(_ context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := request.Params.Arguments
	description := getArgString(args, "event_description")
	date := getArgString(args, "date")
	location := getArgString(args, "location")

	text := fmt.Sprintf("Check for potential duplicate events based on the description below. Suggest search queries or filters to verify uniqueness.\n\nDescription: %s\nDate: %s\nLocation: %s", description, date, location)

	return &mcp.GetPromptResult{
		Description: "Check for potential duplicate events",
		Messages: []mcp.PromptMessage{
			{
				Role:    mcp.RoleUser,
				Content: mcp.NewTextContent(text),
			},
		},
	}, nil
}

func getArgString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if value, ok := args[key]; ok {
		switch v := value.(type) {
		case string:
			return v
		case fmt.Stringer:
			return v.String()
		}
	}
	return ""
}
