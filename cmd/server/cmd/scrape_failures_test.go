package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/apitypes"
	"github.com/spf13/cobra"
)

var scrapeTestMu sync.Mutex

func setupFailuresCmd(t *testing.T, args []string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var origParent *cobra.Command
	if scrapeCmd.HasParent() {
		origParent = scrapeCmd.Parent()
		origParent.RemoveCommand(scrapeCmd)
	}

	t.Cleanup(func() {
		if origParent != nil {
			if scrapeCmd.HasParent() {
				scrapeCmd.Parent().RemoveCommand(scrapeCmd)
			}
			origParent.AddCommand(scrapeCmd)
		}
	})

	failuresSource = ""
	failuresLimit = 0
	failuresStatus = ""
	failuresJSON = false

	testRoot := &cobra.Command{Use: "server"}
	testRoot.AddCommand(scrapeCmd)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(errBuf)
	testRoot.SetArgs(args)

	return testRoot, buf, errBuf
}

func TestScrapeFailuresTable(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.AllDiagnosticsResponse{
			Items: []apitypes.ScraperRunResponse{
				{
					SourceName:   "source-a",
					Tier:         1,
					Status:       "error",
					EventsFound:  42,
					EventsNew:    10,
					EventsDup:    30,
					EventsFailed: 2,
					ErrorMessage: "connection timeout after 30 seconds",
					StartedAt:    &now,
				},
				{
					SourceName:   "source-b",
					Tier:         2,
					Status:       "error",
					EventsFound:  15,
					EventsNew:    0,
					EventsDup:    15,
					EventsFailed: 0,
					ErrorMessage: "a very long error message that should be truncated to approximately sixty characters in the table output",
					StartedAt:    &now,
				},
			},
			Total: 2,
		})
	}))
	defer server.Close()

	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	defer func() { scrapeServerURL = origServer; scrapeAPIKey = origKey }()
	scrapeServerURL = server.URL
	scrapeAPIKey = "test-key"

	cmd, buf, _ := setupFailuresCmd(t, []string{"scrape", "failures"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "SOURCE") || !strings.Contains(output, "TIER") || !strings.Contains(output, "STATUS") || !strings.Contains(output, "FAILED") {
		t.Error("table header missing")
	}
	if !strings.Contains(output, "source-a") {
		t.Error("source-a missing from table")
	}
	if !strings.Contains(output, "source-b") {
		t.Error("source-b missing from table")
	}
	if !strings.Contains(output, "Errors:") {
		t.Error("errors section missing")
	}
	if !strings.Contains(output, "source-a: connection timeout after 30 seconds") {
		t.Error("full error for source-a missing from errors section")
	}
	// The long error should appear in full (not truncated) in the errors section
	if !strings.Contains(output, "source-b: a very long error message that should be truncated to approximately sixty characters in the table output") {
		t.Error("full error for source-b missing from errors section")
	}
	if !strings.Contains(output, "2 sources error") {
		t.Errorf("summary line missing or wrong, got:\n%s", output)
	}
}

func TestScrapeFailuresJSON(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.AllDiagnosticsResponse{
			Items: []apitypes.ScraperRunResponse{
				{
					SourceName:   "source-a",
					Tier:         1,
					Status:       "error",
					EventsFound:  42,
					EventsNew:    10,
					EventsDup:    30,
					EventsFailed: 2,
					ErrorMessage: "timeout",
					StartedAt:    &now,
				},
			},
			Total: 1,
		})
	}))
	defer server.Close()

	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	defer func() { scrapeServerURL = origServer; scrapeAPIKey = origKey }()
	scrapeServerURL = server.URL
	scrapeAPIKey = "test-key"

	cmd, buf, _ := setupFailuresCmd(t, []string{"scrape", "failures", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	var parsed apitypes.AllDiagnosticsResponse
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if parsed.Total != 1 || len(parsed.Items) != 1 {
		t.Errorf("unexpected parsed data: %+v", parsed)
	}
	if parsed.Items[0].SourceName != "source-a" {
		t.Errorf("unexpected source name: %s", parsed.Items[0].SourceName)
	}
}

func TestScrapeFailuresJSONWithSource(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.DiagnosticsResponse{
			SourceName: "mysource",
			LatestRun: &apitypes.ScraperRunResponse{
				SourceName:   "mysource",
				SourceURL:    "https://example.com",
				Tier:         1,
				Status:       "error",
				EventsFound:  5,
				EventsNew:    1,
				EventsDup:    4,
				EventsFailed: 0,
				ErrorMessage: "timeout",
				StartedAt:    &now,
			},
		})
	}))
	defer server.Close()

	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	defer func() { scrapeServerURL = origServer; scrapeAPIKey = origKey }()
	scrapeServerURL = server.URL
	scrapeAPIKey = "test-key"

	cmd, buf, _ := setupFailuresCmd(t, []string{"scrape", "failures", "--json", "--source", "mysource"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	var parsed apitypes.DiagnosticsResponse
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if parsed.SourceName != "mysource" {
		t.Errorf("unexpected source name: %s", parsed.SourceName)
	}
}

func TestScrapeFailuresDeepDive(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.DiagnosticsResponse{
			SourceName: "mysource",
			LatestRun: &apitypes.ScraperRunResponse{
				SourceName:   "mysource",
				SourceURL:    "https://example.com",
				Tier:         1,
				Status:       "error",
				StartedAt:    &now,
				CompletedAt:  &now,
				EventsFound:  42,
				EventsNew:    10,
				EventsDup:    30,
				EventsFailed: 2,
				ErrorMessage: "connection refused",
				EventFailures: []apitypes.EventFailureResponse{
					{Index: 0, Message: "invalid date format"},
					{Index: 1, Message: "missing required field: name"},
				},
			},
			LastSuccessfulRun: &apitypes.ScraperRunResponse{
				SourceName:  "mysource",
				SourceURL:   "https://example.com",
				Tier:        1,
				Status:      "completed",
				StartedAt:   &now,
				EventsFound: 40,
				EventsNew:   5,
				EventsDup:   35,
			},
		})
	}))
	defer server.Close()

	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	defer func() { scrapeServerURL = origServer; scrapeAPIKey = origKey }()
	scrapeServerURL = server.URL
	scrapeAPIKey = "test-key"

	cmd, buf, _ := setupFailuresCmd(t, []string{"scrape", "failures", "--source", "mysource"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	checks := []string{
		"Source: mysource",
		"URL: https://example.com",
		"Tier: 1",
		"Latest run",
		"error",
		"connection refused",
		"Events found: 42",
		"new: 10",
		"dup: 30",
		"failed: 2",
		"Event failures:",
		"[0] invalid date format",
		"[1] missing required field: name",
		"Last successful run",
		"completed",
		"Events found: 40",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\noutput:\n%s", check, output)
		}
	}
}

func TestScrapeFailuresDeepDiveNoRuns(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.DiagnosticsResponse{
			SourceName: "mysource",
		})
	}))
	defer server.Close()

	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	defer func() { scrapeServerURL = origServer; scrapeAPIKey = origKey }()
	scrapeServerURL = server.URL
	scrapeAPIKey = "test-key"

	cmd, buf, _ := setupFailuresCmd(t, []string{"scrape", "failures", "--source", "mysource"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Source: mysource") {
		t.Error("missing source name")
	}
	if !strings.Contains(output, "No runs found") {
		t.Errorf("expected 'No runs found', got:\n%s", output)
	}
}

func TestScrapeFailuresEmpty(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.AllDiagnosticsResponse{
			Items: []apitypes.ScraperRunResponse{},
			Total: 0,
		})
	}))
	defer server.Close()

	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	defer func() { scrapeServerURL = origServer; scrapeAPIKey = origKey }()
	scrapeServerURL = server.URL
	scrapeAPIKey = "test-key"

	cmd, buf, _ := setupFailuresCmd(t, []string{"scrape", "failures"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No failed sources found") {
		t.Errorf("expected 'No failed sources found', got:\n%s", output)
	}
}

func TestScrapeFailuresServerError(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	defer func() { scrapeServerURL = origServer; scrapeAPIKey = origKey }()
	scrapeServerURL = server.URL
	scrapeAPIKey = "test-key"

	cmd, _, _ := setupFailuresCmd(t, []string{"scrape", "failures"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 500 response")
	} else if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500, got: %v", err)
	}
}

func TestScrapeFailuresAuthError(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "invalid token"}`))
	}))
	defer server.Close()

	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	defer func() { scrapeServerURL = origServer; scrapeAPIKey = origKey }()
	scrapeServerURL = server.URL
	scrapeAPIKey = "bad-key"

	cmd, _, _ := setupFailuresCmd(t, []string{"scrape", "failures"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 401 response")
	} else if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("error should mention authentication, got: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention 401, got: %v", err)
	}
}

func TestScrapeFailuresConnectionRefused(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	defer func() { scrapeServerURL = origServer; scrapeAPIKey = origKey }()
	scrapeServerURL = "http://127.0.0.1:1"
	scrapeAPIKey = "test-key"

	cmd, _, _ := setupFailuresCmd(t, []string{"scrape", "failures"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for connection refused")
	} else if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error should mention request failure, got: %v", err)
	}
}

func TestScrapeFailuresWithStatusFilter(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	now := time.Now()
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.AllDiagnosticsResponse{
			Items: []apitypes.ScraperRunResponse{
				{SourceName: "source-a", Tier: 1, Status: "failed", EventsFound: 10, ErrorMessage: "error", StartedAt: &now},
			},
			Total: 1,
		})
	}))
	defer server.Close()

	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	defer func() { scrapeServerURL = origServer; scrapeAPIKey = origKey }()
	scrapeServerURL = server.URL
	scrapeAPIKey = "test-key"

	cmd, buf, _ := setupFailuresCmd(t, []string{"scrape", "failures", "--status", "failed", "--limit", "5"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(requestPath, "status=failed") {
		t.Errorf("expected status=failed in request, got: %s", requestPath)
	}
	if !strings.Contains(requestPath, "limit=5") {
		t.Errorf("expected limit=5 in request, got: %s", requestPath)
	}

	output := buf.String()
	if !strings.Contains(output, "source-a") {
		t.Errorf("missing source in output:\n%s", output)
	}
}

func TestScrapeFailuresWithEnvToken(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	origToken := os.Getenv("TOGATHER_ADMIN_TOKEN")
	defer func() {
		if origToken != "" {
			_ = os.Setenv("TOGATHER_ADMIN_TOKEN", origToken)
		} else {
			_ = os.Unsetenv("TOGATHER_ADMIN_TOKEN")
		}
	}()
	_ = os.Setenv("TOGATHER_ADMIN_TOKEN", "env-test-token")

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.AllDiagnosticsResponse{
			Items: []apitypes.ScraperRunResponse{},
			Total: 0,
		})
	}))
	defer server.Close()

	origServer := scrapeServerURL
	origKey := scrapeAPIKey
	origSELKey := os.Getenv("SEL_API_KEY")
	defer func() {
		scrapeServerURL = origServer
		scrapeAPIKey = origKey
		if origSELKey != "" {
			_ = os.Setenv("SEL_API_KEY", origSELKey)
		} else {
			_ = os.Unsetenv("SEL_API_KEY")
		}
	}()
	_ = os.Unsetenv("SEL_API_KEY")
	scrapeServerURL = server.URL
	scrapeAPIKey = ""

	cmd, _, _ := setupFailuresCmd(t, []string{"scrape", "failures"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(authHeader, "env-test-token") {
		t.Errorf("expected Authorization header with env-test-token, got: %s", authHeader)
	}
}

func TestParseServerConfig(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	origBaseURL := os.Getenv("TOGATHER_BASE_URL")
	origAdminToken := os.Getenv("TOGATHER_ADMIN_TOKEN")
	origSELKey := os.Getenv("SEL_API_KEY")
	defer func() {
		restoreEnv(t, "TOGATHER_BASE_URL", origBaseURL)
		restoreEnv(t, "TOGATHER_ADMIN_TOKEN", origAdminToken)
		restoreEnv(t, "SEL_API_KEY", origSELKey)
	}()
	_ = os.Unsetenv("TOGATHER_BASE_URL")
	_ = os.Unsetenv("TOGATHER_ADMIN_TOKEN")
	_ = os.Unsetenv("SEL_API_KEY")

	origServerFlag := scrapeServerURL
	origKeyFlag := scrapeAPIKey
	defer func() {
		scrapeServerURL = origServerFlag
		scrapeAPIKey = origKeyFlag
	}()
	scrapeServerURL = ""
	scrapeAPIKey = ""

	t.Run("defaults", func(t *testing.T) {
		serverURL, authKey, err := parseServerConfig("", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if serverURL != "http://localhost:8080" {
			t.Errorf("expected default server, got: %s", serverURL)
		}
		if authKey != "" {
			t.Errorf("expected empty auth key, got: %s", authKey)
		}
	})

	t.Run("from flags", func(t *testing.T) {
		serverURL, authKey, err := parseServerConfig("http://example.com", "my-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if serverURL != "http://example.com" {
			t.Errorf("expected server from flag, got: %s", serverURL)
		}
		if authKey != "my-key" {
			t.Errorf("expected key from flag, got: %s", authKey)
		}
	})

	t.Run("from env TOGATHER_ADMIN_TOKEN", func(t *testing.T) {
		_ = os.Setenv("TOGATHER_ADMIN_TOKEN", "admin-token")
		_, authKey, err := parseServerConfig("", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if authKey != "admin-token" {
			t.Errorf("expected admin token from env, got: %s", authKey)
		}
		_ = os.Unsetenv("TOGATHER_ADMIN_TOKEN")
	})

	t.Run("from env SEL_API_KEY", func(t *testing.T) {
		_ = os.Setenv("SEL_API_KEY", "sel-key")
		_, authKey, err := parseServerConfig("", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if authKey != "sel-key" {
			t.Errorf("expected SEL key from env, got: %s", authKey)
		}
		_ = os.Unsetenv("SEL_API_KEY")
	})
}

func restoreEnv(t *testing.T, key, value string) {
	t.Helper()
	if value != "" {
		_ = os.Setenv(key, value)
	} else {
		_ = os.Unsetenv(key)
	}
}

func TestScrapeFailuresFlagRegistration(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	cmd := scrapeFailuresCmd
	flags := []string{"source", "limit", "status", "json"}
	for _, flag := range flags {
		if f := cmd.Flags().Lookup(flag); f == nil {
			t.Errorf("expected flag %q to be defined on failures command", flag)
		}
	}
	if scrapeFailuresCmd.Parent() != scrapeCmd {
		t.Error("failures command should be child of scrape command")
	}
}

func TestScrapeFailuresHasServerAndKeyPersistentFlags(t *testing.T) {
	t.Parallel()
	scrapeTestMu.Lock()
	t.Cleanup(func() { scrapeTestMu.Unlock() })
	pf := scrapeCmd.PersistentFlags()
	if pf.Lookup("server") == nil {
		t.Error("scrapeCmd missing --server persistent flag")
	}
	if pf.Lookup("key") == nil {
		t.Error("scrapeCmd missing --key persistent flag")
	}
}
