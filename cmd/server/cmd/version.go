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
		_, _ = fmt.Fprintf(out, "Togather SEL Server\n")
		_, _ = fmt.Fprintf(out, "Version:    %s\n", Version)
		_, _ = fmt.Fprintf(out, "Git commit: %s\n", GitCommit)
		_, _ = fmt.Fprintf(out, "Build date: %s\n", BuildDate)
		_, _ = fmt.Fprintf(out, "Go version: %s\n", runtime.Version())
		_, _ = fmt.Fprintf(out, "Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}
