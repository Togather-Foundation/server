package loadtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/loadtest/fixture"
	"github.com/oklog/ulid/v2"
)

func quickConfig() ProfileConfig {
	return ProfileConfig{RequestsPerSecond: 100, Duration: 40 * time.Millisecond, ReadWriteRatio: 1}
}

func TestPublicProbeNeverWritesOrRequestsMetrics(t *testing.T) {
	// Check the complete workload contract, independently of random sampling.
	expected := map[string]bool{
		"/health":               true,
		"/api/v1/events":        true,
		"/api/v1/places":        true,
		"/api/v1/organizations": true,
	}
	for _, operation := range publicReadOperations() {
		if operation.method != http.MethodGet || !expected[operation.path] {
			t.Fatalf("unexpected public operation %s %s", operation.method, operation.path)
		}
		delete(expected, operation.path)
	}
	if len(expected) != 0 {
		t.Fatalf("public workload missing operations: %v", expected)
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.URL.Path == "/metrics" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	for _, p := range LoadProfiles {
		if p.ReadWriteRatio != 1 {
			t.Fatal("profile permits writes by default")
		}
	}
	stats, err := NewLoadTester(server.URL).RunCustom(context.Background(), quickConfig())
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() == 0 {
		t.Fatal("no requests")
	}
	data, _ := json.Marshal(stats.Summary())
	if !strings.Contains(string(data), "status_codes") {
		t.Fatalf("missing distribution: %s", data)
	}
}

func TestUnexpectedFailuresFailProbe(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			stats, err := NewLoadTester(server.URL).RunCustom(context.Background(), quickConfig())
			if err == nil {
				t.Fatal("unexpected status accepted")
			}
			if stats.Summary().Passed {
				t.Fatal("summary passed failed run")
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	if _, err := NewLoadTester(server.URL).RunCustom(context.Background(), quickConfig()); err == nil {
		t.Fatal("transport failure accepted")
	}
}

func TestWritePreflightBeforeSustainedTraffic(t *testing.T) {
	var writes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writes.Add(1); w.WriteHeader(401) }))
	defer server.Close()
	cfg := quickConfig()
	cfg.ReadWriteRatio = 0
	if _, err := NewLoadTester(server.URL).WithoutAuth().RunCustom(context.Background(), cfg); err == nil {
		t.Fatal("unauthenticated write run accepted")
	}
	if writes.Load() != 0 {
		t.Fatal("sent unauthenticated preflight")
	}
	if _, err := NewLoadTester(server.URL).WithAPIKeys([]string{"secret"}).WithFixtureRun(fixtureManifest(t, server.URL)).RunCustom(context.Background(), cfg); err == nil {
		t.Fatal("failed preflight accepted")
	}
	if writes.Load() != 1 {
		t.Fatalf("sustained writes after failed preflight: %d", writes.Load())
	}
}

func TestErrorBudgetNeverMasksAuthOrServerFailures(t *testing.T) {
	for _, status := range []int{401, 403, 500, 429} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		cfg := quickConfig()
		cfg.MaxErrorRate = 1
		_, err := NewLoadTester(server.URL).RunCustom(context.Background(), cfg)
		server.Close()
		if (err == nil) != (status == 429) {
			t.Fatalf("status %d err %v", status, err)
		}
	}
}

func TestValidateConfiguration(t *testing.T) {
	for _, mutate := range []func(*ProfileConfig){func(c *ProfileConfig) { c.RequestsPerSecond = 0 }, func(c *ProfileConfig) { c.Duration = 0 }, func(c *ProfileConfig) { c.ReadWriteRatio = -1 }, func(c *ProfileConfig) { c.MaxErrorRate = 2 }} {
		cfg := quickConfig()
		mutate(&cfg)
		if _, err := NewLoadTester("http://unused.invalid").RunCustom(context.Background(), cfg); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}

func TestAuthenticatedPreflightCannotDecodeOrCreateEvent(t *testing.T) {
	server, state := ownedFixtureServer(t, nil)
	defer server.Close()
	cfg := quickConfig()
	cfg.ReadWriteRatio = 0
	stats, err := NewLoadTester(server.URL).WithAPIKeys([]string{"secret", "second"}).WithFixtureRun(fixtureManifest(t, server.URL)).RunCustom(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.preflights != 2 || len(state.inputs) <= 2 {
		t.Fatalf("preflight=%d writes=%d", state.preflights, len(state.inputs))
	}
	if len(state.ownershipKeys) != 2 || !state.ownershipKeys["Bearer secret"] || !state.ownershipKeys["Bearer second"] {
		t.Fatalf("credentials not independently proven: %v", state.ownershipKeys)
	}
	if stats.Summary().Endpoints["auth_preflight"].StatusCodes[400] != 2 || stats.Summary().Endpoints["ownership_verify"].StatusCodes[200] != 2 {
		t.Fatal("missing explicit preflight statistics")
	}
}

func TestRedirectIsNotSuccessfulProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(200)
			return
		}
		http.Redirect(w, r, "/ok", http.StatusFound)
	}))
	defer server.Close()
	if _, err := NewLoadTester(server.URL).RunCustom(context.Background(), func() ProfileConfig { cfg := quickConfig(); cfg.MaxErrorRate = 1; return cfg }()); err == nil {
		t.Fatal("redirect hid unexpected endpoint status")
	}
}

func TestEveryRunErrorHasFailedStructuredResult(t *testing.T) {
	cfg := quickConfig()
	cfg.RequestsPerSecond = 0
	stats, err := NewLoadTester("http://unused.invalid").RunCustom(context.Background(), cfg)
	if err == nil || stats.Summary().Passed || len(stats.Summary().FailedExpectations) == 0 {
		t.Fatal("invalid configuration did not produce failed result")
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); cancel() }))
	defer server.Close()
	stats, err = NewLoadTester(server.URL).RunCustom(ctx, quickConfig())
	if err == nil || stats.Summary().Passed || len(stats.Summary().FailedExpectations) == 0 {
		t.Fatal("cancellation did not produce failed result")
	}
}

func TestWriteRequiresFixtureManifest(t *testing.T) {
	var writes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]any
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			w.WriteHeader(400)
			return
		}
		writes.Add(1)
		w.WriteHeader(200)
	}))
	defer server.Close()
	cfg := quickConfig()
	cfg.ReadWriteRatio = 0
	_, err := NewLoadTester(server.URL).WithAPIKeys([]string{"secret"}).RunCustom(context.Background(), cfg)
	if err == nil || writes.Load() != 0 {
		t.Fatalf("unowned write run: err=%v writes=%d", err, writes.Load())
	}
}

func fixtureManifest(t *testing.T, target string) fixture.Manifest {
	t.Helper()
	m, err := fixture.New(target, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

type fixtureServerState struct {
	mu            sync.Mutex
	preflights    int
	inputs        []events.EventInput
	ownershipKeys map[string]bool
}

// Fault hooks run with state.mu held; return a nonzero status to override success.
func ownedFixtureServer(t *testing.T, fault func(*http.Request, map[string]any) int) (*httptest.Server, *fixtureServerState) {
	t.Helper()
	state := &fixtureServerState{ownershipKeys: map[string]bool{}}
	stored := map[string]events.EventInput{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		var reply map[string]any
		status := 200
		if r.Method == "POST" {
			var input events.EventInput
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				state.preflights++
				w.WriteHeader(400)
				return
			}
			state.inputs = append(state.inputs, input)
			id := ulid.Make().String()
			stored[id] = input
			reply = map[string]any{"@id": "https://canonical.invalid/api/v1/events/" + id}
			status = 201
		} else if strings.HasPrefix(r.URL.Path, "/api/v1/events/") {
			if r.URL.Query().Get("include_provenance") != "true" {
				t.Error("missing provenance query")
			}
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/events/")
			input, ok := stored[id]
			if !ok {
				w.WriteHeader(404)
				return
			}
			state.ownershipKeys[r.Header.Get("Authorization")] = true
			reply = map[string]any{"@id": "https://canonical.invalid/api/v1/events/" + id, "name": input.Name, "url": input.URL, "sources": []map[string]any{{"url": input.Source.URL, "sel:sourceEventId": input.Source.EventID}}}
		}
		if fault != nil {
			if override := fault(r, reply); override != 0 {
				status = override
			}
		}
		w.WriteHeader(status)
		if reply != nil {
			_ = json.NewEncoder(w).Encode(reply)
		}
	}))
	return server, state
}

func TestOwnershipPreflightRefusesMismatchesBeforeLoad(t *testing.T) {
	for _, name := range []string{"accepted", "duplicate", "invalid-create-id", "missing-sources", "extra-sources", "wrong-source", "wrong-source-id", "wrong-name", "wrong-url", "wrong-detail-id", "hidden", "redirect"} {
		t.Run(name, func(t *testing.T) {
			server, state := ownedFixtureServer(t, func(r *http.Request, reply map[string]any) int {
				if r.Method == "POST" {
					switch name {
					case "accepted":
						return 202
					case "duplicate":
						return 409
					case "invalid-create-id":
						reply["@id"] = "https://evil.invalid/nope"
					}
				} else if strings.HasPrefix(r.URL.Path, "/api/v1/events/") {
					switch name {
					case "missing-sources":
						delete(reply, "sources")
					case "extra-sources":
						reply["sources"] = []map[string]any{{"url": "foreign"}, {"url": "another"}}
					case "wrong-source":
						reply["sources"].([]map[string]any)[0]["url"] = "https://foreign.invalid/feed"
					case "wrong-source-id":
						reply["sources"].([]map[string]any)[0]["sel:sourceEventId"] = "wrong"
					case "wrong-name":
						reply["name"] = "real event"
					case "wrong-url":
						reply["url"] = "https://real.invalid/event"
					case "wrong-detail-id":
						reply["@id"] = "https://canonical.invalid/api/v1/events/" + ulid.Make().String()
					case "hidden":
						return 404
					case "redirect":
						return 302
					}
				}
				return 0
			})
			defer server.Close()
			cfg := quickConfig()
			cfg.ReadWriteRatio = 0
			cfg.MaxErrorRate = 1
			stats, err := NewLoadTester(server.URL).WithAPIKeys([]string{"secret"}).WithFixtureRun(fixtureManifest(t, server.URL)).RunCustom(context.Background(), cfg)
			if err == nil || stats.Summary().Passed {
				t.Fatal("unproven ownership accepted")
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			if len(state.inputs) != 1 {
				t.Fatalf("sustained traffic after failed proof: %d", len(state.inputs))
			}
		})
	}
}

func TestSustainedWriteRequiresCreatedRegardlessOfErrorBudget(t *testing.T) {
	for _, status := range []int{202, 409, 422} {
		writes := 0
		server, _ := ownedFixtureServer(t, func(r *http.Request, _ map[string]any) int {
			if r.Method == "POST" {
				writes++
				if writes > 1 {
					return status
				}
			}
			return 0
		})
		cfg := quickConfig()
		cfg.ReadWriteRatio = 0
		cfg.MaxErrorRate = 1
		stats, err := NewLoadTester(server.URL).WithAPIKeys([]string{"secret"}).WithFixtureRun(fixtureManifest(t, server.URL)).RunCustom(context.Background(), cfg)
		server.Close()
		if err == nil || stats.Summary().Passed {
			t.Fatalf("status %d accepted", status)
		}
	}
}
