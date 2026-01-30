package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Togather-Foundation/server/internal/loadtest"
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
	)
	flag.Parse()

	// Create load tester
	tester := loadtest.NewLoadTester(*baseURL)

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
