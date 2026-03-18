package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Togather-Foundation/server/tests/testdata"
	"github.com/spf13/cobra"
)

var (
	generateCount        int
	generateSeed         int64
	generateReviewQueue  bool
	generateReviewEvents bool
)

// generateCmd provides test event generation
var generateCmd = &cobra.Command{
	Use:   "generate [output-file]",
	Short: "Generate test events from fixtures",
	Long: `Generate realistic test events using built-in fixtures.

This command creates valid event JSON that can be ingested into the SEL server.
Events are based on real Toronto venues and include proper Schema.org structure.

Examples:
  # Generate one event to stdout
  server generate

  # Generate 5 events to a file
  server generate test-events.json --count 5

  # Generate with specific seed for reproducibility
  server generate events.json --seed 42

  # Generate curated review event fixture set (11 scenario groups, 22 events)
  server generate review-fixtures.json --review-events

  # Generate and immediately ingest
  server generate events.json && server ingest events.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGenerate(args)
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)

	generateCmd.Flags().IntVarP(&generateCount, "count", "n", 1, "number of events to generate")
	generateCmd.Flags().Int64Var(&generateSeed, "seed", 0, "random seed (0 = random)")
	generateCmd.Flags().BoolVar(&generateReviewQueue, "review-queue", false, "generate events that need review (data quality issues)")
	generateCmd.Flags().BoolVar(&generateReviewEvents, "review-events", false, "generate curated review event fixture set (11 scenario groups, 22 events)")
}

func runGenerate(args []string) error {
	// Create generator
	var gen *testdata.Generator
	if generateSeed != 0 {
		gen = testdata.NewGenerator(generateSeed)
	} else {
		gen = testdata.NewGenerator(0) // Will use time-based seed
	}

	if generateReviewEvents {
		return runGenerateReviewEvents(gen, args)
	}

	// Generate events
	var eventInputs []interface{}

	if generateReviewQueue {
		// Generate events with data quality issues for review queue testing
		reviewEvents := gen.BatchReviewQueueInputs(generateCount)
		for _, event := range reviewEvents {
			eventInputs = append(eventInputs, event)
		}
	} else {
		// Generate normal events
		for i := 0; i < generateCount; i++ {
			event := gen.RandomEventInput()
			eventInputs = append(eventInputs, event)
		}
	}

	// Create batch wrapper
	batch := map[string]interface{}{
		"events": eventInputs,
	}

	// Marshal to JSON
	output, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}

	// Write output
	if len(args) > 0 {
		// Write to file
		outputFile := args[0]
		if err := os.WriteFile(outputFile, output, 0644); err != nil {
			return fmt.Errorf("write file: %w", err)
		}

		eventType := "event(s)"
		if generateReviewQueue {
			eventType = "review queue event(s)"
		}
		fmt.Printf("✓ Generated %d %s to %s\n", generateCount, eventType, outputFile)
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Printf("  server ingest %s\n", outputFile)
		if generateReviewQueue {
			fmt.Printf("  server ingest %s --watch  # Watch for review queue entries\n", outputFile)
		} else {
			fmt.Printf("  server ingest %s --watch\n", outputFile)
		}
	} else {
		// Write to stdout
		fmt.Println(string(output))
	}

	return nil
}

// reviewEventsOutput is the JSON structure for --review-events output.
type reviewEventsOutput struct {
	Scenarios []reviewScenarioJSON `json:"scenarios"`
	Events    []any                `json:"events"`
}

type reviewScenarioJSON struct {
	GroupID     string `json:"group_id"`
	Description string `json:"description"`
	Events      []any  `json:"events"`
}

func runGenerateReviewEvents(gen *testdata.Generator, args []string) error {
	scenarios := gen.BatchReviewEventInputs()

	out := reviewEventsOutput{
		Scenarios: make([]reviewScenarioJSON, 0, len(scenarios)),
		Events:    make([]any, 0, 22),
	}

	for _, s := range scenarios {
		js := reviewScenarioJSON{
			GroupID:     s.GroupID,
			Description: s.Description,
			Events:      make([]any, len(s.Events)),
		}
		for i, ev := range s.Events {
			js.Events[i] = ev
			out.Events = append(out.Events, ev)
		}
		out.Scenarios = append(out.Scenarios, js)
	}

	// Marshal to JSON
	output, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal review events: %w", err)
	}

	if len(args) > 0 {
		outputFile := args[0]
		if err := os.WriteFile(outputFile, output, 0644); err != nil {
			return fmt.Errorf("write file: %w", err)
		}
		fmt.Printf("✓ Generated %d review event scenario groups (%d events) to %s\n",
			len(out.Scenarios), len(out.Events), outputFile)
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Printf("  # Ingest all events for batch testing:\n")
		fmt.Printf("  server ingest %s\n", outputFile)
		fmt.Printf("  # Or use the 'scenarios' key to ingest groups individually.\n")
	} else {
		fmt.Println(string(output))
	}

	return nil
}
