package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Togather-Foundation/server/internal/loadtest"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("loadtest", flag.ContinueOnError)
	flags.SetOutput(errOut)
	baseURL := flags.String("url", "http://localhost:8080", "Base URL of the server to test")
	profile := flags.String("profile", "light", "Load profile: light, medium, heavy, stress, burst, peak")
	rps := flags.Int("rps", 0, "Custom requests per second")
	duration := flags.Duration("duration", 0, "Custom test duration")
	readRatio := flags.Float64("read-ratio", 1, "Read fraction 0-1; below 1 explicitly enables writes")
	budget := flags.Float64("max-error-rate", 0, "Allowed fraction of non-authentication 4xx failures (0-1)")
	noRamp := flags.Bool("no-ramp", false, "Disable ramp-up/ramp-down")
	apiKey := flags.String("api-key", "", "API keys, comma-separated; required for writes")
	debugAuth := flags.Bool("debug-auth", false, "Log status and auth mode for 401 responses; never keys")
	jsonOutput := flags.Bool("json", false, "Print structured endpoint/status results")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	config, ok := loadtest.LoadProfiles[loadtest.LoadProfile(*profile)]
	if !ok {
		_, _ = fmt.Fprintf(errOut, "unknown profile: %s\n", *profile)
		return 2
	}
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "rps":
			config.RequestsPerSecond = *rps
		case "duration":
			config.Duration = *duration
		}
	})
	config.ReadWriteRatio = *readRatio
	config.MaxErrorRate = *budget
	if *noRamp {
		config.RampUpTime = 0
		config.RampDownTime = 0
	}
	tester := loadtest.NewLoadTester(*baseURL).WithoutAuth().WithDebugAuth(*debugAuth)
	keys := []string{}
	for _, key := range strings.Split(*apiKey, ",") {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) > 0 {
		tester.WithAPIKeys(keys)
	}
	stats, err := tester.RunCustom(ctx, config)
	if stats != nil {
		if *jsonOutput {
			if encodeErr := json.NewEncoder(out).Encode(stats.Summary()); encodeErr != nil {
				_, _ = fmt.Fprintf(errOut, "write results: %v\n", encodeErr)
				return 1
			}
		} else {
			if _, writeErr := fmt.Fprintln(out, stats.Report()); writeErr != nil {
				return 1
			}
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
		return 1
	}
	return 0
}
