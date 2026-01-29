package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Togather-Foundation/server/tests/testdata"
	"github.com/spf13/cobra"
)

var (
	generateCount int
	generateSeed  int64
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
	for i := 0; i < generateCount; i++ {
		event := gen.RandomEventInput()
		eventInputs = append(eventInputs, event)
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

		fmt.Printf("✓ Generated %d event(s) to %s\n", generateCount, outputFile)
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Printf("  server ingest %s\n", outputFile)
		fmt.Printf("  server ingest %s --watch\n", outputFile)
	} else {
		// Write to stdout
		fmt.Println(string(output))
	}

	return nil
}
