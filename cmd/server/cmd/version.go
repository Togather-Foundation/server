package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	// Version information (set via ldflags during build)
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version number, git commit, build date, and Go runtime version.`,
	Run: func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Togather SEL Server\n")
		fmt.Fprintf(out, "Version:    %s\n", Version)
		fmt.Fprintf(out, "Git commit: %s\n", GitCommit)
		fmt.Fprintf(out, "Build date: %s\n", BuildDate)
		fmt.Fprintf(out, "Go version: %s\n", runtime.Version())
		fmt.Fprintf(out, "Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}
