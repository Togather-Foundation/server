package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	configPath string
	logLevel   string
	logFormat  string

	// rootCmd represents the base command when called without any subcommands
	rootCmd = &cobra.Command{
		Use:   "server",
		Short: "Togather SEL server - Shared Events Library backend",
		Long: `Togather SEL server implements the Shared Events Library (SEL) specification,
providing a federated backend for event data aggregation, enrichment, and distribution.

The server supports:
- Event ingestion from multiple sources (HTML, JSON-LD, ICS, RSS, APIs)
- Duplicate detection and deduplication
- JSON-LD export with Schema.org vocabulary
- Federation with other SEL nodes
- Knowledge graph reconciliation (Artsdata, Wikidata)`,
		// Run the serve command by default if no subcommand is specified
		RunE: func(cmd *cobra.Command, args []string) error {
			// If no subcommand provided, run serve by default
			return serveCmd.RunE(cmd, args)
		},
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Global flags available to all subcommands
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "config file path (optional, uses env vars by default)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error) (default: info)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "", "log format (json, console) (default: json)")

	// Add subcommands
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(healthcheckCmd)
}
