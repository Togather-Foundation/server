package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Togather-Foundation/server/tests/testdata"
	"github.com/spf13/cobra"
)

var (
	generateCount       int
	generateSeed        int64
	generateReviewQueue bool
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
}

func runGenerate(args []string) error {
	// Create generator
	var gen *testdata.Generator
	if generateSeed != 0 {
		gen = testdata.NewGenerator(generateSeed)
	} else {
		gen = testdata.NewGenerator(0) // Will use time-based seed
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
