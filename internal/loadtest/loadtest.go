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
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/loadtest/fixture"
	"github.com/Togather-Foundation/server/internal/testauth"
	"github.com/oklog/ulid/v2"
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
	MaxErrorRate      float64       // Allowed fraction of non-authentication 4xx failures; default zero
	ReadWriteRatio    float64       // Ratio of reads to writes (0.8 = 80% reads, 20% writes)
}

// LoadProfiles contains predefined load testing scenarios.
var LoadProfiles = map[LoadProfile]ProfileConfig{
	ProfileLight: {
		RequestsPerSecond: 5,
		Duration:          1 * time.Minute,
		RampUpTime:        10 * time.Second,
		RampDownTime:      10 * time.Second,
		ReadWriteRatio:    1,
	},
	ProfileMedium: {
		RequestsPerSecond: 20,
		Duration:          2 * time.Minute,
		RampUpTime:        20 * time.Second,
		RampDownTime:      20 * time.Second,
		ReadWriteRatio:    1,
	},
	ProfileHeavy: {
		RequestsPerSecond: 50,
		Duration:          5 * time.Minute,
		RampUpTime:        30 * time.Second,
		RampDownTime:      30 * time.Second,
		ReadWriteRatio:    1,
	},
	ProfileStress: {
		RequestsPerSecond: 100,
		Duration:          10 * time.Minute,
		RampUpTime:        1 * time.Minute,
		RampDownTime:      1 * time.Minute,
		ReadWriteRatio:    1,
	},
	ProfileBurst: {
		RequestsPerSecond: 10, // Base RPS (will spike to 100)
		Duration:          5 * time.Minute,
		RampUpTime:        0,
		RampDownTime:      0,
		ReadWriteRatio:    1,
	},
	ProfilePeak: {
		RequestsPerSecond: 40, // Peak RPS
		Duration:          10 * time.Minute,
		RampUpTime:        3 * time.Minute,
		RampDownTime:      3 * time.Minute,
		ReadWriteRatio:    1,
	},
}

// LoadTester orchestrates load testing operations.
type LoadTester struct {
	baseURL         string
	httpClient      *http.Client
	fixtureRun      *fixture.Manifest
	fixtureSequence uint64
	stats           *Statistics
	authenticator   *testauth.TestAuthenticator
	apiKeys         []string
	apiKeyIndex     uint32
	debugAuth       bool
}

// NewLoadTester creates a new load tester targeting the specified base URL.
func NewLoadTester(baseURL string) *LoadTester {
	return &LoadTester{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		stats: &Statistics{},
	}
}

// WithFixtureRun supplies the private ownership contract for all write fixtures.
func (lt *LoadTester) WithFixtureRun(manifest fixture.Manifest) *LoadTester {
	lt.fixtureRun = &manifest
	return lt
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
	mu                    sync.Mutex
	failures              []string
	unconditionalFailures []string

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
	count       int64
	total       int64   // total response time in ms
	times       []int64 // all response times for percentile calculation
	statusCodes map[int]int64
	errors      int64
	minTime     int64
	maxTime     int64
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
func (lt *LoadTester) RunCustom(ctx context.Context, config ProfileConfig) (stats *Statistics, runErr error) {
	lt.stats = &Statistics{
		errors:        make(map[int]int64),
		endpointStats: make(map[string]*EndpointStats),
		startTime:     time.Now(),
	}

	defer func() {
		lt.stats.mu.Lock()
		defer lt.stats.mu.Unlock()
		lt.stats.endTime = time.Now()
		if runErr != nil && len(lt.stats.failures) == 0 {
			lt.stats.failures = []string{runErr.Error()}
		}
	}()
	if config.RequestsPerSecond <= 0 || config.RequestsPerSecond > int(time.Second) || config.Duration <= 0 || config.RampUpTime < 0 || config.RampDownTime < 0 || math.IsNaN(config.ReadWriteRatio) || math.IsInf(config.ReadWriteRatio, 0) || config.ReadWriteRatio < 0 || config.ReadWriteRatio > 1 || math.IsNaN(config.MaxErrorRate) || math.IsInf(config.MaxErrorRate, 0) || config.MaxErrorRate < 0 || config.MaxErrorRate > 1 {
		return lt.stats, fmt.Errorf("invalid load configuration")
	}
	if config.ReadWriteRatio < 1 {
		if lt.fixtureRun == nil {
			return lt.stats, fmt.Errorf("write testing requires a private run manifest")
		}
		if err := lt.fixtureRun.Validate(); err != nil {
			return lt.stats, fmt.Errorf("invalid fixture run: %w", err)
		}
		if lt.fixtureRun.TargetURL != lt.baseURL {
			return lt.stats, fmt.Errorf("fixture run target does not match load target")
		}
		lt.fixtureSequence = 0
		if len(lt.apiKeys) == 0 && lt.authenticator == nil {
			return lt.stats, fmt.Errorf("write testing requires explicit credentials")
		}
		// Malformed JSON reaches the authenticated create handler but cannot decode
		// into an event. A 400 proves the credential passed the auth middleware
		// without creating a fixture; 401/403 are never accepted.
		attempts := len(lt.apiKeys)
		if attempts == 0 {
			attempts = 1
		}
		for i := 0; i < attempts; i++ {
			lt.executeRequest(ctx, workItem{method: "POST", path: "/api/v1/events", endpoint: "auth_preflight", rawBody: "{", expectedStatus: http.StatusBadRequest})
			if lt.stats.failedRequests > 0 {
				lt.stats.endTime = time.Now()
				_ = lt.stats.validate(0)
				return lt.stats, fmt.Errorf("authenticated write preflight failed; sustained traffic was not started")
			}
		}
		// Verify stored attribution for one distinct fixture per credential before load.
		for i := 0; i < attempts; i++ {
			var key *string
			if len(lt.apiKeys) > 0 {
				key = &lt.apiKeys[i]
			}
			if err := lt.verifyFixtureOwnership(ctx, key); err != nil {
				return lt.stats, err
			}
		}
	}

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

	if err := ctx.Err(); err != nil {
		return lt.stats, fmt.Errorf("load test interrupted: %w", err)
	}
	if err := lt.stats.validate(config.MaxErrorRate); err != nil {
		return lt.stats, err
	}
	return lt.stats, nil
}

// workItem represents a single HTTP request to be executed.
type workItem struct {
	method            string
	path              string
	body              interface{}
	endpoint          string // for stats tracking
	rawBody           string // Used only for guaranteed non-decodable preflight JSON
	expectedStatus    int    // Zero means any 2xx
	strictExpectation bool
	apiKeyOverride    *string
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
				select {
				case workChan <- lt.generateReadRequest():
				case <-ctx.Done():
					return
				}
			} else {
				// Write operation
				work, err := lt.generateWriteRequest()
				if err != nil {
					lt.failExpectation("fixture generation failed")
					return
				}
				select {
				case workChan <- work:
				case <-ctx.Done():
					return
				}
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

// publicReadOperations defines the public, non-mutating probe contract.
func publicReadOperations() []workItem {
	return []workItem{
		{method: "GET", path: "/health", endpoint: "health"},
		{method: "GET", path: "/api/v1/events", endpoint: "list_events"},
		{method: "GET", path: "/api/v1/places", endpoint: "list_places"},
		{method: "GET", path: "/api/v1/organizations", endpoint: "list_orgs"},
	}
}

// generateReadRequest creates a random read operation.
func (lt *LoadTester) generateReadRequest() workItem {
	operations := publicReadOperations()
	return operations[rand.Intn(len(operations))]
}

// generateWriteRequest creates a uniquely owned, deterministic virtual fixture.
func (lt *LoadTester) generateWriteRequest() (workItem, error) {
	if lt.fixtureRun == nil || lt.fixtureSequence == math.MaxUint64 {
		return workItem{}, fmt.Errorf("fixture sequence unavailable")
	}
	lt.fixtureSequence++
	input, err := lt.fixtureRun.Event(lt.fixtureSequence)
	if err != nil {
		return workItem{}, err
	}
	return workItem{method: "POST", path: "/api/v1/events", body: input, endpoint: "create_event", expectedStatus: http.StatusCreated, strictExpectation: true}, nil
}

func (lt *LoadTester) failExpectation(message string) {
	lt.stats.mu.Lock()
	defer lt.stats.mu.Unlock()
	lt.stats.unconditionalFailures = append(lt.stats.unconditionalFailures, message)
}

// verifyFixtureOwnership proves actual stored provenance, rather than key association.
func (lt *LoadTester) verifyFixtureOwnership(ctx context.Context, key *string) error {
	work, err := lt.generateWriteRequest()
	if err != nil {
		return err
	}
	input := work.body.(events.EventInput)
	work.endpoint = "ownership_create"
	work.apiKeyOverride = key
	status, body := lt.executeRequest(ctx, work)
	if status != http.StatusCreated {
		return fmt.Errorf("fixture ownership preflight requires HTTP 201; sustained traffic was not started")
	}
	var created struct {
		ID string `json:"@id"`
	}
	if json.Unmarshal(body, &created) != nil {
		return fmt.Errorf("invalid fixture ownership create response")
	}
	eventID, err := responseEventULID(created.ID)
	if err != nil {
		return fmt.Errorf("invalid fixture ownership event identity")
	}
	status, body = lt.executeRequest(ctx, workItem{method: "GET", path: "/api/v1/events/" + eventID + "?include_provenance=true", endpoint: "ownership_verify", expectedStatus: http.StatusOK, strictExpectation: true, apiKeyOverride: key})
	if status != http.StatusOK {
		return fmt.Errorf("fixture ownership detail is not publicly readable")
	}
	var detail struct {
		ID      string `json:"@id"`
		Name    string `json:"name"`
		URL     string `json:"url"`
		Sources []struct {
			URL     string `json:"url"`
			EventID string `json:"sel:sourceEventId"`
		} `json:"sources"`
	}
	if json.Unmarshal(body, &detail) != nil {
		return fmt.Errorf("invalid fixture ownership detail JSON")
	}
	gotID, err := responseEventULID(detail.ID)
	if err != nil || gotID != eventID || detail.Name != input.Name || detail.URL != input.URL || len(detail.Sources) != 1 || events.NormalizeFeedURL(detail.Sources[0].URL) != lt.fixtureRun.SourceURL || detail.Sources[0].EventID != input.Source.EventID {
		return fmt.Errorf("stored fixture ownership does not match the run")
	}
	return nil
}

func responseEventULID(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid event URI")
	}
	parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
	if len(parts) < 2 || parts[len(parts)-2] != "events" {
		return "", fmt.Errorf("invalid event URI path")
	}
	id := parts[len(parts)-1]
	parsed, err := ulid.ParseStrict(id)
	if err != nil || parsed.String() != id {
		return "", fmt.Errorf("invalid event ULID")
	}
	return id, nil
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
func (lt *LoadTester) executeRequest(ctx context.Context, work workItem) (int, []byte) {
	atomic.AddInt64(&lt.stats.totalRequests, 1)

	start := time.Now()

	// Build request
	var reqBody io.Reader
	if work.rawBody != "" {
		reqBody = strings.NewReader(work.rawBody)
	} else if work.body != nil {
		jsonData, err := json.Marshal(work.body)
		if err != nil {
			lt.recordError(0, work.endpoint)
			return 0, nil
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, work.method, lt.baseURL+work.path, reqBody)
	if err != nil {
		lt.recordError(0, work.endpoint)
		return 0, nil
	}

	if work.body != nil || work.rawBody != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Add authentication if available
	authMode := "none"
	if work.apiKeyOverride != nil {
		req.Header.Set("Authorization", "Bearer "+*work.apiKeyOverride)
		authMode = "api_key"
	} else if len(lt.apiKeys) > 0 {
		idx := atomic.AddUint32(&lt.apiKeyIndex, 1)
		key := lt.apiKeys[int(idx-1)%len(lt.apiKeys)]
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
			authMode = "api_key"
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
		return 0, nil
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body (to ensure full response time is measured)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(body) > 2*1024*1024 {
		lt.recordError(0, work.endpoint)
		return 0, nil
	}

	// Record statistics
	lt.recordResponse(resp.StatusCode, duration, work.endpoint, work.expectedStatus)
	if work.strictExpectation && resp.StatusCode != work.expectedStatus {
		lt.failExpectation(fmt.Sprintf("%s expected HTTP %d, got %d", work.endpoint, work.expectedStatus, resp.StatusCode))
	}
	if lt.debugAuth && resp.StatusCode == http.StatusUnauthorized {
		fmt.Fprintf(os.Stderr, "AUTH_DEBUG status=401 endpoint=%s method=%s path=%s auth=%s \n",
			work.endpoint, work.method, work.path, authMode,
		)
	}
	return resp.StatusCode, body
}

// recordResponse records a successful response.
func (lt *LoadTester) recordResponse(statusCode int, durationMs int64, endpoint string, expectedStatus int) {
	lt.stats.mu.Lock()
	defer lt.stats.mu.Unlock()

	lt.stats.responseTimes = append(lt.stats.responseTimes, durationMs)

	accepted := statusCode >= 200 && statusCode < 300
	if expectedStatus != 0 {
		accepted = statusCode == expectedStatus
	}
	if accepted {
		atomic.AddInt64(&lt.stats.successRequests, 1)
	} else {
		atomic.AddInt64(&lt.stats.failedRequests, 1)
		lt.stats.errors[statusCode]++
	}

	// Update endpoint stats
	if lt.stats.endpointStats[endpoint] == nil {
		lt.stats.endpointStats[endpoint] = &EndpointStats{
			statusCodes: make(map[int]int64),
			minTime:     durationMs,
			maxTime:     durationMs,
		}
	}

	epStats := lt.stats.endpointStats[endpoint]
	epStats.statusCodes[statusCode]++
	epStats.count++
	epStats.total += durationMs
	epStats.times = append(epStats.times, durationMs)

	if durationMs < epStats.minTime {
		epStats.minTime = durationMs
	}
	if durationMs > epStats.maxTime {
		epStats.maxTime = durationMs
	}

	if !accepted {
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
		lt.stats.endpointStats[endpoint] = &EndpointStats{statusCodes: make(map[int]int64)}
	}
	lt.stats.endpointStats[endpoint].count++
	lt.stats.endpointStats[endpoint].statusCodes[statusCode]++
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
	fmt.Fprintf(&report, "Duration:        %s\n", duration.Round(time.Second))
	fmt.Fprintf(&report, "Total Requests:  %d\n", totalReqs)
	fmt.Fprintf(&report, "Successful:      %d (%.1f%%)\n", s.successRequests, float64(s.successRequests)/float64(totalReqs)*100)
	fmt.Fprintf(&report, "Failed:          %d (%.1f%%)\n", s.failedRequests, float64(s.failedRequests)/float64(totalReqs)*100)
	fmt.Fprintf(&report, "Requests/sec:    %.2f\n\n", float64(totalReqs)/duration.Seconds())

	// Response time percentiles
	if len(s.responseTimes) > 0 {
		p50, p95, p99 := s.calculatePercentiles()
		avgTime := s.calculateAverage()

		report.WriteString("Response Times (ms):\n")
		fmt.Fprintf(&report, "  Average:  %d\n", avgTime)
		fmt.Fprintf(&report, "  p50:      %d\n", p50)
		fmt.Fprintf(&report, "  p95:      %d\n", p95)
		fmt.Fprintf(&report, "  p99:      %d\n\n", p99)
	}

	// Error breakdown
	if len(s.errors) > 0 {
		report.WriteString("Errors by Status Code:\n")
		for code, count := range s.errors {
			if code != 0 {
				fmt.Fprintf(&report, "  %d: %d\n", code, count)
			}
		}
		report.WriteString("\n")
	}

	// Per-endpoint statistics
	if len(s.endpointStats) > 0 {
		report.WriteString("Per-Endpoint Statistics:\n")
		report.WriteString("─────────────────────────────────────────────────────────────\n")
		fmt.Fprintf(&report, "%-20s %8s %8s %8s %8s %8s\n", "Endpoint", "Count", "Avg(ms)", "p95(ms)", "Min", "Max")
		report.WriteString("─────────────────────────────────────────────────────────────\n")

		for endpoint, stats := range s.endpointStats {
			if stats.count == 0 {
				continue
			}
			avg := stats.total / stats.count
			p95 := calculatePercentile(stats.times, 0.95)
			fmt.Fprintf(&report, "%-20s %8d %8d %8d %8d %8d\n",
				endpoint, stats.count, avg, p95, stats.minTime, stats.maxTime)
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

// Result is a machine-readable account of observed status codes and failed expectations.
type Result struct {
	TotalRequests      int64                     `json:"total_requests"`
	FailedRequests     int64                     `json:"failed_requests"`
	Passed             bool                      `json:"passed"`
	FailedExpectations []string                  `json:"failed_expectations"`
	Endpoints          map[string]EndpointResult `json:"endpoints"`
}
type EndpointResult struct {
	Count       int64         `json:"count"`
	StatusCodes map[int]int64 `json:"status_codes"`
}

func (s *Statistics) failedExpectations(maxErrorRate float64) []string {
	failures := append([]string{}, s.unconditionalFailures...)
	if s.totalRequests == 0 {
		failures = append(failures, "no requests completed")
	}
	for code, count := range s.errors {
		if count > 0 && (code < 400 || code == 401 || code == 403 || code >= 500) {
			failures = append(failures, fmt.Sprintf("unexpected status %d: %d requests (0 denotes transport/body failure)", code, count))
		}
	}
	if s.totalRequests > 0 && float64(s.failedRequests)/float64(s.totalRequests) > maxErrorRate {
		failures = append(failures, fmt.Sprintf("error rate exceeds budget %.4f", maxErrorRate))
	}
	return failures
}
func (s *Statistics) validate(maxErrorRate float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = s.failedExpectations(maxErrorRate)
	if len(s.failures) > 0 {
		return fmt.Errorf("load expectations failed: %v", s.failures)
	}
	return nil
}
func (s *Statistics) Summary() Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := Result{TotalRequests: s.totalRequests, FailedRequests: s.failedRequests, Passed: len(s.failures) == 0 && s.totalRequests > 0, FailedExpectations: append([]string{}, s.failures...), Endpoints: make(map[string]EndpointResult)}
	for name, ep := range s.endpointStats {
		codes := make(map[int]int64)
		for code, n := range ep.statusCodes {
			codes[code] = n
		}
		result.Endpoints[name] = EndpointResult{ep.count, codes}
	}
	return result
}
