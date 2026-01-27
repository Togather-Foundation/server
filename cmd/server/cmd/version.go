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
		fmt.Printf("Togather SEL Server\n")
		fmt.Printf("Version:    %s\n", Version)
		fmt.Printf("Git commit: %s\n", GitCommit)
		fmt.Printf("Build date: %s\n", BuildDate)
		fmt.Printf("Go version: %s\n", runtime.Version())
		fmt.Printf("Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}
