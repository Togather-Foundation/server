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

	"github.com/spf13/cobra"
)

var reviewTestMu sync.Mutex

func setupReviewCmd(t *testing.T, args []string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var origParent *cobra.Command
	if reviewCmd.HasParent() {
		origParent = reviewCmd.Parent()
		origParent.RemoveCommand(reviewCmd)
	}

	t.Cleanup(func() {
		if origParent != nil {
			if reviewCmd.HasParent() {
				reviewCmd.Parent().RemoveCommand(reviewCmd)
			}
			origParent.AddCommand(reviewCmd)
		}
	})

	reviewJSON = false

	queueStatus = "pending"
	queueLimit = 50
	queueWarning = ""
	queueName = ""
	queueSource = ""
	queueGroupBy = ""
	queueOffset = 0
	queueOutput = ""

	approveNotes = ""
	approveRecordNotDup = false
	rejectReason = ""
	rejectNotes = ""
	fixNotes = ""
	fixStartDate = ""
	fixEndDate = ""

	batchName = ""
	batchSource = ""
	batchWarning = ""
	batchAction = ""
	batchReason = ""
	batchPrimaryID = ""
	batchDryRun = false
	batchLimit = 200
	batchNotes = ""
	batchStartDate = ""
	batchEndDate = ""

	editName = ""
	editDescription = ""
	editImage = ""
	editURL = ""
	editDomain = ""
	editDryRun = false

	mergeTransferOccurrences = false
	mergeName = ""
	mergeDescription = ""
	mergeImage = ""
	mergeURL = ""
	mergeDomain = ""
	mergeDryRun = false

	statsOutput = ""

	testRoot := &cobra.Command{Use: "server"}
	testRoot.AddCommand(reviewCmd)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(errBuf)
	testRoot.SetArgs(args)

	return testRoot, buf, errBuf
}

func TestReviewQueueTable(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	startTime := time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)
	now := time.Now()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{
					ID: 42, EventID: "evt_abc123", Status: "pending",
					Warnings:       []Warning{{Code: "missing_description", Field: "description", Message: "event has no description"}},
					CreatedAt:      now.Add(-24 * time.Hour),
					EventName:      "Weekly Jazz Night",
					EventStartTime: &startTime,
				},
			},
			Total: 1,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "queue"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ID") || !strings.Contains(output, "ULID") || !strings.Contains(output, "NAME") || !strings.Contains(output, "START DATE") {
		t.Error("table header missing")
	}
	if !strings.Contains(output, "42") {
		t.Error("review ID missing")
	}
	if !strings.Contains(output, "evt_abc123") {
		t.Error("event ULID missing")
	}
	if !strings.Contains(output, "Weekly Jazz Night") {
		t.Error("event name missing")
	}
	if !strings.Contains(output, "2026-07-15") {
		t.Error("start date missing")
	}
	if !strings.Contains(output, "missing_description") {
		t.Error("warning code missing")
	}
}

func TestReviewQueueJSON(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	startTime := time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)
	now := time.Now()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{
					ID: 42, EventID: "evt_abc123", Status: "pending",
					Warnings:       []Warning{{Code: "missing_description", Field: "description", Message: "event has no description"}},
					CreatedAt:      now.Add(-24 * time.Hour),
					EventName:      "Weekly Jazz Night",
					EventStartTime: &startTime,
				},
			},
			Total: 1,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "queue", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	var items []ReviewQueueItem
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != 42 {
		t.Errorf("expected ID 42, got %d", items[0].ID)
	}
	if items[0].EventName != "Weekly Jazz Night" {
		t.Errorf("unexpected event name: %s", items[0].EventName)
	}
}

func TestReviewQueueEmpty(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{},
			Total: 0,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "queue"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No review queue items found.") {
		t.Errorf("expected 'No review queue items found.', got:\n%s", output)
	}
}

func TestReviewQueueGroupBy(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", CreatedAt: now.Add(-48 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 2, EventID: "evt_02", EventName: "Jazz Night", CreatedAt: now.Add(-24 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 3, EventID: "evt_03", EventName: "Blues Jam", CreatedAt: now.Add(-12 * time.Hour), Warnings: []Warning{{Code: "missing_description"}}},
			},
			Total: 3,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "queue", "--group-by", "name"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NAME") || !strings.Contains(output, "COUNT") {
		t.Error("grouped table header missing")
	}
	if !strings.Contains(output, "Jazz Night") {
		t.Error("Jazz Night group missing")
	}
	if !strings.Contains(output, "Blues Jam") {
		t.Error("Blues Jam group missing")
	}
	// Jazz Night has 2 items, should be listed first (sorted by count desc)
	jazzNightIdx := strings.Index(output, "Jazz Night")
	bluesJamIdx := strings.Index(output, "Blues Jam")
	if jazzNightIdx > bluesJamIdx {
		t.Error("Jazz Night (2 items) should appear before Blues Jam (1 item)")
	}
}

func TestReviewQueueGroupByWarning(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Event A", CreatedAt: now.Add(-48 * time.Hour), Warnings: []Warning{{Code: "bad_date"}, {Code: "missing_description"}}},
			},
			Total: 1,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "queue", "--group-by", "warning"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "WARNING") || !strings.Contains(output, "COUNT") {
		t.Error("grouped-by-warning table header missing")
	}
	if !strings.Contains(output, "bad_date") {
		t.Error("bad_date warning group missing")
	}
	if !strings.Contains(output, "missing_description") {
		t.Error("missing_description warning group missing")
	}
}

func TestReviewQueueGroupBySource(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	srcA := "source-a"
	srcB := "source-b"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Event A", SourceID: &srcA, CreatedAt: now.Add(-48 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 2, EventID: "evt_02", EventName: "Event B", SourceID: &srcA, CreatedAt: now.Add(-24 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 3, EventID: "evt_03", EventName: "Event C", SourceID: &srcB, CreatedAt: now.Add(-12 * time.Hour), Warnings: []Warning{{Code: "missing_description"}}},
			},
			Total: 3,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "queue", "--group-by", "source"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "SOURCE") || !strings.Contains(output, "COUNT") {
		t.Error("grouped-by-source table header missing")
	}
	if !strings.Contains(output, "source-a") {
		t.Error("source-a group missing")
	}
	if !strings.Contains(output, "source-b") {
		t.Error("source-b group missing")
	}
}

func TestReviewQueueWithOffset(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Event A", CreatedAt: now.Add(-48 * time.Hour)},
				{ID: 2, EventID: "evt_02", EventName: "Event B", CreatedAt: now.Add(-24 * time.Hour)},
				{ID: 3, EventID: "evt_03", EventName: "Event C", CreatedAt: now.Add(-12 * time.Hour)},
			},
			Total: 3,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "queue", "--offset", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "Event A") {
		t.Error("Event A should be skipped by offset=1")
	}
	if !strings.Contains(output, "Event B") {
		t.Error("Event B should be present")
	}
	if !strings.Contains(output, "Event C") {
		t.Error("Event C should be present")
	}
}

func TestReviewQueueClientSideFilters(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	srcA := "source-a"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", SourceID: &srcA, CreatedAt: now, Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 2, EventID: "evt_02", EventName: "Blues Jam", CreatedAt: now, Warnings: []Warning{{Code: "missing_description"}}},
				{ID: 3, EventID: "evt_03", EventName: "Jazz Night Late", SourceID: &srcA, CreatedAt: now, Warnings: []Warning{{Code: "bad_date"}, {Code: "near_duplicate"}}},
			},
			Total: 3,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	t.Run("filter by name", func(t *testing.T) {
		cmd, buf, _ := setupReviewCmd(t, []string{"review", "queue", "--name", "Jazz"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "Jazz Night") {
			t.Error("Jazz Night missing (name filter)")
		}
		if !strings.Contains(output, "Jazz Night Late") {
			t.Error("Jazz Night Late missing (name filter)")
		}
		if strings.Contains(output, "Blues Jam") {
			t.Error("Blues Jam should be filtered out by name")
		}
	})

	t.Run("filter by warning", func(t *testing.T) {
		cmd, buf, _ := setupReviewCmd(t, []string{"review", "queue", "--warning", "bad_date"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		if strings.Contains(output, "Blues Jam") {
			t.Error("Blues Jam should be filtered out by warning=bad_date")
		}
		if !strings.Contains(output, "Jazz Night") {
			t.Error("Jazz Night should match bad_date warning")
		}
	})

	t.Run("filter by source", func(t *testing.T) {
		cmd, buf, _ := setupReviewCmd(t, []string{"review", "queue", "--source", "source-a"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		if strings.Contains(output, "Blues Jam") {
			t.Error("Blues Jam (no source) should be filtered out")
		}
		if !strings.Contains(output, "Jazz Night") {
			t.Error("Jazz Night (source-a) should match")
		}
	})
}

func TestReviewQueueServerError(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewCmd(t, []string{"review", "queue"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500, got: %v", err)
	}
}

func TestReviewQueueUnauthorized(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewCmd(t, []string{"review", "queue"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("error should mention authentication, got: %v", err)
	}
}

func TestReviewQueueAuthViaTokenFlag(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{Items: []ReviewQueueItem{}, Total: 0})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "my-direct-token"

	cmd, _, _ := setupReviewCmd(t, []string{"review", "queue"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(authHeader, "my-direct-token") {
		t.Errorf("expected Authorization header with my-direct-token, got: %s", authHeader)
	}
}

func TestReviewQueueAuthViaEnvToken(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origEnv := os.Getenv("TOGATHER_ADMIN_API_KEY")
	defer func() {
		if origEnv != "" {
			_ = os.Setenv("TOGATHER_ADMIN_API_KEY", origEnv)
		} else {
			_ = os.Unsetenv("TOGATHER_ADMIN_API_KEY")
		}
	}()
	_ = os.Setenv("TOGATHER_ADMIN_API_KEY", "env-admin-key")

	var authHeader string
	var reqPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "exchanged-jwt", "expires_at": "2026-12-01T00:00:00Z"})
			return
		}
		authHeader = r.Header.Get("Authorization")
		reqPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{Items: []ReviewQueueItem{}, Total: 0})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = ""

	cmd, _, _ := setupReviewCmd(t, []string{"review", "queue"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(authHeader, "exchanged-jwt") {
		t.Errorf("expected Authorization header with exchanged-jwt, got: %s", authHeader)
	}
	if reqPath != "/api/v1/admin/review-queue" {
		t.Errorf("expected request to /api/v1/admin/review-queue, got: %s", reqPath)
	}
}
