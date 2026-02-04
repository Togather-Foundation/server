package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Togather-Foundation/server/internal/loadtest"
	"github.com/Togather-Foundation/server/internal/testauth"
)

func main() {
	// Parse command-line flags
	var (
		baseURL   = flag.String("url", "http://localhost:8080", "Base URL of the server to test")
		profile   = flag.String("profile", "light", "Load profile: light, medium, heavy, stress, burst, peak")
		rps       = flag.Int("rps", 0, "Custom requests per second (overrides profile)")
		duration  = flag.Duration("duration", 0, "Custom test duration (overrides profile)")
		readRatio = flag.Float64("read-ratio", 0, "Read/write ratio 0.0-1.0 (overrides profile)")
		noRamp    = flag.Bool("no-ramp", false, "Disable ramp-up/ramp-down (instant start/stop)")
		apiKey    = flag.String("api-key", "", "API key(s) for write endpoints (comma-separated or repeatable in wrapper scripts)")
		debugAuth = flag.Bool("debug-auth", false, "Log auth debug lines for 401 responses")
	)
	flag.Parse()

	// Create load tester (defaults to no auth)
	tester := loadtest.NewLoadTester(*baseURL).WithoutAuth().WithDebugAuth(*debugAuth)

	// If API key(s) provided, use them for write endpoints
	if strings.TrimSpace(*apiKey) != "" {
		parts := strings.Split(*apiKey, ",")
		keys := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				keys = append(keys, trimmed)
			}
		}
		if len(keys) > 0 {
			tester = tester.WithAPIKeys(keys)
		} else {
			auth, err := testauth.NewAPIKeyAuthenticator(*apiKey)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Failed to create API key authenticator: %v\n", err)
				fmt.Fprintf(os.Stderr, "Note: API key auth is only needed for write-heavy testing\n")
				os.Exit(1)
			}
			tester = tester.WithAuth(auth)
		}
	}

	// Determine configuration
	var config loadtest.ProfileConfig
	if *rps > 0 || *duration > 0 || *readRatio > 0 || *noRamp {
		// Custom configuration
		baseConfig := loadtest.LoadProfiles[loadtest.ProfileLight]
		if *rps > 0 {
			baseConfig.RequestsPerSecond = *rps
		}
		if *duration > 0 {
			baseConfig.Duration = *duration
		}
		if *readRatio > 0 {
			baseConfig.ReadWriteRatio = *readRatio
		}
		if *noRamp {
			baseConfig.RampUpTime = 0
			baseConfig.RampDownTime = 0
		}
		config = baseConfig

		fmt.Printf("Running custom load test configuration\n\n")
	} else {
		// Use predefined profile
		loadProfile := loadtest.LoadProfile(*profile)

		fmt.Printf("Running load profile: %s\n\n", loadProfile)

		stats, err := tester.Run(contextWithSignal(), loadProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(stats.Report())
		os.Exit(0)
	}

	// Run custom configuration
	stats, err := tester.RunCustom(contextWithSignal(), config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(stats.Report())
}

// contextWithSignal returns a context that is cancelled on SIGINT/SIGTERM.
func contextWithSignal() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, stopping test...")
		cancel()
	}()

	return ctx
}
