// Package performance provides load testing utilities for the Togather server.
// This tool generates realistic traffic patterns to validate monitoring dashboards
// and test blue-green deployment performance under load.
package loadtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Togather-Foundation/server/internal/testauth"
	"github.com/Togather-Foundation/server/tests/testdata"
)

// LoadProfile defines different load testing scenarios.
type LoadProfile string

const (
	ProfileLight  LoadProfile = "light"  // 5 req/s, 1 minute
	ProfileMedium LoadProfile = "medium" // 20 req/s, 2 minutes
	ProfileHeavy  LoadProfile = "heavy"  // 50 req/s, 5 minutes
	ProfileStress LoadProfile = "stress" // 100 req/s, 10 minutes
	ProfileBurst  LoadProfile = "burst"  // Spike pattern: 10 req/s → 100 req/s → 10 req/s
	ProfilePeak   LoadProfile = "peak"   // Simulates peak hours with gradual ramp-up/down
)

// ProfileConfig defines the parameters for a load test.
type ProfileConfig struct {
	RequestsPerSecond int           // Target requests per second
	Duration          time.Duration // How long to run the test
	RampUpTime        time.Duration // Time to gradually reach target RPS
	RampDownTime      time.Duration // Time to gradually decrease RPS
	ReadWriteRatio    float64       // Ratio of reads to writes (0.8 = 80% reads, 20% writes)
}

// LoadProfiles contains predefined load testing scenarios.
var LoadProfiles = map[LoadProfile]ProfileConfig{
	ProfileLight: {
		RequestsPerSecond: 5,
		Duration:          1 * time.Minute,
		RampUpTime:        10 * time.Second,
		RampDownTime:      10 * time.Second,
		ReadWriteRatio:    0.8,
	},
	ProfileMedium: {
		RequestsPerSecond: 20,
		Duration:          2 * time.Minute,
		RampUpTime:        20 * time.Second,
		RampDownTime:      20 * time.Second,
		ReadWriteRatio:    0.8,
	},
	ProfileHeavy: {
		RequestsPerSecond: 50,
		Duration:          5 * time.Minute,
		RampUpTime:        30 * time.Second,
		RampDownTime:      30 * time.Second,
		ReadWriteRatio:    0.7,
	},
	ProfileStress: {
		RequestsPerSecond: 100,
		Duration:          10 * time.Minute,
		RampUpTime:        1 * time.Minute,
		RampDownTime:      1 * time.Minute,
		ReadWriteRatio:    0.6,
	},
	ProfileBurst: {
		RequestsPerSecond: 10, // Base RPS (will spike to 100)
		Duration:          5 * time.Minute,
		RampUpTime:        0,
		RampDownTime:      0,
		ReadWriteRatio:    0.8,
	},
	ProfilePeak: {
		RequestsPerSecond: 40, // Peak RPS
		Duration:          10 * time.Minute,
		RampUpTime:        3 * time.Minute,
		RampDownTime:      3 * time.Minute,
		ReadWriteRatio:    0.75,
	},
}

// LoadTester orchestrates load testing operations.
type LoadTester struct {
	baseURL       string
	httpClient    *http.Client
	generator     *testdata.Generator
	stats         *Statistics
	authenticator *testauth.TestAuthenticator
	apiKeys       []string
	apiKeyIndex   uint32
	debugAuth     bool
}

// NewLoadTester creates a new load tester targeting the specified base URL.
func NewLoadTester(baseURL string) *LoadTester {
	// Try to create dev authenticator (will use dev defaults)
	auth, err := testauth.NewDevAuthenticator()
	if err != nil {
		// If auth fails, continue without authentication
		auth = nil
	}

	return &LoadTester{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		generator:     testdata.NewGenerator(time.Now().UnixNano()),
		stats:         &Statistics{},
		authenticator: auth,
	}
}

// WithAuth sets a custom authenticator for this load tester.
func (lt *LoadTester) WithAuth(auth *testauth.TestAuthenticator) *LoadTester {
	lt.authenticator = auth
	lt.apiKeys = nil
	return lt
}

// WithoutAuth disables authentication for this load tester.
func (lt *LoadTester) WithoutAuth() *LoadTester {
	lt.authenticator = nil
	lt.apiKeys = nil
	return lt
}

// WithDebugAuth enables auth debugging logs for 401 responses.
func (lt *LoadTester) WithDebugAuth(enabled bool) *LoadTester {
	lt.debugAuth = enabled
	return lt
}

// WithAPIKeys configures a rotating set of API keys for requests.
// When set, these keys are used instead of the authenticator.
func (lt *LoadTester) WithAPIKeys(keys []string) *LoadTester {
	lt.apiKeys = keys
	lt.apiKeyIndex = 0
	lt.authenticator = nil
	return lt
}

// Statistics tracks load test metrics.
type Statistics struct {
	mu sync.Mutex

	totalRequests   int64
	successRequests int64
	failedRequests  int64

	// Response time tracking (in milliseconds)
	responseTimes []int64

	// Errors
	errors map[int]int64 // status code -> count

	// Per-endpoint stats
	endpointStats map[string]*EndpointStats

	startTime time.Time
	endTime   time.Time
}

// EndpointStats tracks statistics for a specific endpoint.
type EndpointStats struct {
	count   int64
	total   int64   // total response time in ms
	times   []int64 // all response times for percentile calculation
	errors  int64
	minTime int64
	maxTime int64
}

// Run executes a load test with the specified profile.
func (lt *LoadTester) Run(ctx context.Context, profile LoadProfile) (*Statistics, error) {
	config, exists := LoadProfiles[profile]
	if !exists {
		return nil, fmt.Errorf("unknown profile: %s", profile)
	}

	return lt.RunCustom(ctx, config)
}

// RunCustom executes a load test with a custom configuration.
func (lt *LoadTester) RunCustom(ctx context.Context, config ProfileConfig) (*Statistics, error) {
	lt.stats = &Statistics{
		errors:        make(map[int]int64),
		endpointStats: make(map[string]*EndpointStats),
		startTime:     time.Now(),
	}

	fmt.Printf("Starting load test...\n")
	fmt.Printf("  Target: %s\n", lt.baseURL)
	fmt.Printf("  RPS: %d\n", config.RequestsPerSecond)
	fmt.Printf("  Duration: %s\n", config.Duration)
	fmt.Printf("  Ramp-up: %s, Ramp-down: %s\n", config.RampUpTime, config.RampDownTime)
	fmt.Printf("  Read/Write ratio: %.0f%%/%.0f%%\n", config.ReadWriteRatio*100, (1-config.ReadWriteRatio)*100)
	fmt.Println()

	// Create worker pool
	workers := config.RequestsPerSecond * 2 // 2 workers per target RPS
	if workers < 10 {
		workers = 10
	}

	workChan := make(chan workItem, workers*2)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lt.worker(ctx, workChan)
		}()
	}

	// Generate work
	go func() {
		defer close(workChan)
		lt.generateWork(ctx, config, workChan)
	}()

	// Wait for completion or cancellation
	wg.Wait()
	lt.stats.endTime = time.Now()

	return lt.stats, nil
}

// workItem represents a single HTTP request to be executed.
type workItem struct {
	method   string
	path     string
	body     interface{}
	endpoint string // for stats tracking
}

// generateWork produces work items according to the load profile.
func (lt *LoadTester) generateWork(ctx context.Context, config ProfileConfig, workChan chan<- workItem) {
	startTime := time.Now()

	// Calculate initial RPS
	currentRPS := 1
	if config.RampUpTime == 0 {
		currentRPS = config.RequestsPerSecond
	}

	ticker := time.NewTicker(time.Second / time.Duration(currentRPS))
	defer ticker.Stop()

	lastRPS := currentRPS

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			elapsed := time.Since(startTime)

			// Check if test duration is complete
			totalDuration := config.RampUpTime + config.Duration + config.RampDownTime
			if elapsed > totalDuration {
				return
			}

			// Calculate current RPS based on ramp-up/down
			currentRPS = lt.calculateCurrentRPS(elapsed, config)

			// Only reset ticker if RPS changed significantly (avoid constant resets)
			if currentRPS != lastRPS {
				ticker.Reset(time.Second / time.Duration(currentRPS))
				lastRPS = currentRPS
			}

			// Generate work item (read or write based on ratio)
			if rand.Float64() < config.ReadWriteRatio {
				// Read operation
				workChan <- lt.generateReadRequest()
			} else {
				// Write operation
				workChan <- lt.generateWriteRequest()
			}
		}
	}
}

// calculateCurrentRPS determines the current RPS based on ramp-up/down timing.
func (lt *LoadTester) calculateCurrentRPS(elapsed time.Duration, config ProfileConfig) int {
	targetRPS := config.RequestsPerSecond

	// Ramp-up phase
	if elapsed < config.RampUpTime {
		progress := float64(elapsed) / float64(config.RampUpTime)
		rps := int(float64(targetRPS) * progress)
		if rps < 1 {
			rps = 1 // Ensure minimum RPS of 1
		}
		return rps
	}

	// Steady state
	steadyEnd := config.RampUpTime + config.Duration
	if elapsed < steadyEnd {
		return targetRPS
	}

	// Ramp-down phase
	rampDownProgress := elapsed - steadyEnd
	if rampDownProgress < config.RampDownTime {
		progress := float64(rampDownProgress) / float64(config.RampDownTime)
		rps := int(float64(targetRPS) * (1.0 - progress))
		if rps < 1 {
			rps = 1 // Ensure minimum RPS of 1
		}
		return rps
	}

	return 1 // Minimum fallback
}

// generateReadRequest creates a random read operation.
func (lt *LoadTester) generateReadRequest() workItem {
	operations := []workItem{
		{method: "GET", path: "/health", endpoint: "health"},
		{method: "GET", path: "/api/v1/events", endpoint: "list_events"},
		{method: "GET", path: "/api/v1/places", endpoint: "list_places"},
		{method: "GET", path: "/api/v1/organizations", endpoint: "list_orgs"},
		{method: "GET", path: "/metrics", endpoint: "metrics"},
	}
	return operations[rand.Intn(len(operations))]
}

// generateWriteRequest creates a random write operation.
func (lt *LoadTester) generateWriteRequest() workItem {
	eventInput := lt.generator.RandomEventInput()

	return workItem{
		method:   "POST",
		path:     "/api/v1/events",
		body:     eventInput,
		endpoint: "create_event",
	}
}

// worker processes work items from the work channel.
func (lt *LoadTester) worker(ctx context.Context, workChan <-chan workItem) {
	for {
		select {
		case <-ctx.Done():
			return
		case work, ok := <-workChan:
			if !ok {
				return
			}
			lt.executeRequest(ctx, work)
		}
	}
}

// executeRequest performs an HTTP request and records statistics.
func (lt *LoadTester) executeRequest(ctx context.Context, work workItem) {
	atomic.AddInt64(&lt.stats.totalRequests, 1)

	start := time.Now()

	// Build request
	var reqBody io.Reader
	if work.body != nil {
		jsonData, err := json.Marshal(work.body)
		if err != nil {
			lt.recordError(0, work.endpoint)
			return
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, work.method, lt.baseURL+work.path, reqBody)
	if err != nil {
		lt.recordError(0, work.endpoint)
		return
	}

	if work.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Add authentication if available
	authMode := "none"
	keyPrefix := ""
	if len(lt.apiKeys) > 0 {
		idx := atomic.AddUint32(&lt.apiKeyIndex, 1)
		key := lt.apiKeys[int(idx-1)%len(lt.apiKeys)]
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
			authMode = "api_key"
			keyPrefix = apiKeyPrefix(key)
		}
	} else if lt.authenticator != nil {
		lt.authenticator.AddAuth(req)
		authMode = "authenticator"
	}

	// Execute request
	resp, err := lt.httpClient.Do(req)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		lt.recordError(0, work.endpoint)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body (to ensure full response time is measured)
	_, _ = io.Copy(io.Discard, resp.Body)

	// Record statistics
	lt.recordResponse(resp.StatusCode, duration, work.endpoint)
	if lt.debugAuth && resp.StatusCode == http.StatusUnauthorized {
		fmt.Fprintf(os.Stderr, "AUTH_DEBUG status=401 endpoint=%s method=%s path=%s auth=%s key_prefix=%s\n",
			work.endpoint, work.method, work.path, authMode, keyPrefix,
		)
	}
}

func apiKeyPrefix(key string) string {
	const prefixLen = 8
	if len(key) <= prefixLen {
		return key
	}
	return key[:prefixLen]
}

// recordResponse records a successful response.
func (lt *LoadTester) recordResponse(statusCode int, durationMs int64, endpoint string) {
	lt.stats.mu.Lock()
	defer lt.stats.mu.Unlock()

	lt.stats.responseTimes = append(lt.stats.responseTimes, durationMs)

	if statusCode >= 200 && statusCode < 300 {
		atomic.AddInt64(&lt.stats.successRequests, 1)
	} else {
		atomic.AddInt64(&lt.stats.failedRequests, 1)
		lt.stats.errors[statusCode]++
	}

	// Update endpoint stats
	if lt.stats.endpointStats[endpoint] == nil {
		lt.stats.endpointStats[endpoint] = &EndpointStats{
			minTime: durationMs,
			maxTime: durationMs,
		}
	}

	epStats := lt.stats.endpointStats[endpoint]
	epStats.count++
	epStats.total += durationMs
	epStats.times = append(epStats.times, durationMs)

	if durationMs < epStats.minTime {
		epStats.minTime = durationMs
	}
	if durationMs > epStats.maxTime {
		epStats.maxTime = durationMs
	}

	if statusCode < 200 || statusCode >= 300 {
		epStats.errors++
	}
}

// recordError records a failed request.
func (lt *LoadTester) recordError(statusCode int, endpoint string) {
	lt.stats.mu.Lock()
	defer lt.stats.mu.Unlock()

	atomic.AddInt64(&lt.stats.failedRequests, 1)
	lt.stats.errors[statusCode]++

	if lt.stats.endpointStats[endpoint] == nil {
		lt.stats.endpointStats[endpoint] = &EndpointStats{}
	}
	lt.stats.endpointStats[endpoint].errors++
}

// Report generates a summary report of the load test.
func (s *Statistics) Report() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	duration := s.endTime.Sub(s.startTime)
	totalReqs := s.totalRequests

	var report bytes.Buffer
	report.WriteString("\n")
	report.WriteString("═══════════════════════════════════════════════════════════════\n")
	report.WriteString("                    LOAD TEST RESULTS                           \n")
	report.WriteString("═══════════════════════════════════════════════════════════════\n\n")

	// Overall statistics
	report.WriteString(fmt.Sprintf("Duration:        %s\n", duration.Round(time.Second)))
	report.WriteString(fmt.Sprintf("Total Requests:  %d\n", totalReqs))
	report.WriteString(fmt.Sprintf("Successful:      %d (%.1f%%)\n", s.successRequests, float64(s.successRequests)/float64(totalReqs)*100))
	report.WriteString(fmt.Sprintf("Failed:          %d (%.1f%%)\n", s.failedRequests, float64(s.failedRequests)/float64(totalReqs)*100))
	report.WriteString(fmt.Sprintf("Requests/sec:    %.2f\n\n", float64(totalReqs)/duration.Seconds()))

	// Response time percentiles
	if len(s.responseTimes) > 0 {
		p50, p95, p99 := s.calculatePercentiles()
		avgTime := s.calculateAverage()

		report.WriteString("Response Times (ms):\n")
		report.WriteString(fmt.Sprintf("  Average:  %d\n", avgTime))
		report.WriteString(fmt.Sprintf("  p50:      %d\n", p50))
		report.WriteString(fmt.Sprintf("  p95:      %d\n", p95))
		report.WriteString(fmt.Sprintf("  p99:      %d\n\n", p99))
	}

	// Error breakdown
	if len(s.errors) > 0 {
		report.WriteString("Errors by Status Code:\n")
		for code, count := range s.errors {
			if code != 0 {
				report.WriteString(fmt.Sprintf("  %d: %d\n", code, count))
			}
		}
		report.WriteString("\n")
	}

	// Per-endpoint statistics
	if len(s.endpointStats) > 0 {
		report.WriteString("Per-Endpoint Statistics:\n")
		report.WriteString("─────────────────────────────────────────────────────────────\n")
		report.WriteString(fmt.Sprintf("%-20s %8s %8s %8s %8s %8s\n", "Endpoint", "Count", "Avg(ms)", "p95(ms)", "Min", "Max"))
		report.WriteString("─────────────────────────────────────────────────────────────\n")

		for endpoint, stats := range s.endpointStats {
			if stats.count == 0 {
				continue
			}
			avg := stats.total / stats.count
			p95 := calculatePercentile(stats.times, 0.95)
			report.WriteString(fmt.Sprintf("%-20s %8d %8d %8d %8d %8d\n",
				endpoint, stats.count, avg, p95, stats.minTime, stats.maxTime))
		}
		report.WriteString("\n")
	}

	report.WriteString("═══════════════════════════════════════════════════════════════\n")
	return report.String()
}

func (s *Statistics) calculatePercentiles() (p50, p95, p99 int64) {
	if len(s.responseTimes) == 0 {
		return 0, 0, 0
	}

	p50 = calculatePercentile(s.responseTimes, 0.50)
	p95 = calculatePercentile(s.responseTimes, 0.95)
	p99 = calculatePercentile(s.responseTimes, 0.99)
	return
}

func (s *Statistics) calculateAverage() int64 {
	if len(s.responseTimes) == 0 {
		return 0
	}

	var sum int64
	for _, t := range s.responseTimes {
		sum += t
	}
	return sum / int64(len(s.responseTimes))
}

func calculatePercentile(times []int64, percentile float64) int64 {
	if len(times) == 0 {
		return 0
	}

	// Make a copy and sort
	sorted := make([]int64, len(times))
	copy(sorted, times)

	// Simple insertion sort (good enough for this use case)
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	index := int(float64(len(sorted)) * percentile)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}
