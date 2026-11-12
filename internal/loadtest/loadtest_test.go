package loadtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func quickConfig() ProfileConfig {
	return ProfileConfig{RequestsPerSecond: 100, Duration: 40 * time.Millisecond, ReadWriteRatio: 1}
}

func TestPublicProbeNeverWritesOrRequestsMetrics(t *testing.T) {
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
	if _, err := NewLoadTester(server.URL).WithAPIKeys([]string{"secret"}).RunCustom(context.Background(), cfg); err == nil {
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
	var preflights, writes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if r.Header.Get("Authorization") != "Bearer secret" {
				w.WriteHeader(401)
				return
			}
			var event map[string]any
			if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
				preflights.Add(1)
				w.WriteHeader(400)
				return
			}
			writes.Add(1)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	cfg := quickConfig()
	cfg.ReadWriteRatio = 0
	stats, err := NewLoadTester(server.URL).WithAPIKeys([]string{"secret"}).RunCustom(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if preflights.Load() != 1 || writes.Load() == 0 {
		t.Fatalf("preflight=%d writes=%d", preflights.Load(), writes.Load())
	}
	if stats.Summary().Endpoints["auth_preflight"].StatusCodes[400] != 1 {
		t.Fatal("missing explicit preflight expectation")
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
