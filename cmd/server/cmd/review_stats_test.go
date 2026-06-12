package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReviewStatsDefault(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", CreatedAt: now.Add(-48 * time.Hour), Warnings: []Warning{{Code: "missing_description"}}},
				{ID: 2, EventID: "evt_02", EventName: "Jazz Night", CreatedAt: now.Add(-72 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 3, EventID: "evt_03", EventName: "Blues Jam", CreatedAt: now.Add(-24 * time.Hour), Warnings: []Warning{{Code: "bad_date"}, {Code: "missing_description"}}},
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

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "stats"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Queue: 3 pending") {
		t.Error("missing queue summary line with count")
	}
	if !strings.Contains(output, "WARNING TYPE") {
		t.Error("missing warning breakdown section")
	}
	if !strings.Contains(output, "NAME GROUPS") {
		t.Error("missing name groups section")
	}
	if !strings.Contains(output, "AGE BUCKETS") {
		t.Error("missing age buckets section")
	}
}

func TestReviewStatsJSON(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", CreatedAt: now.Add(-48 * time.Hour), Warnings: []Warning{{Code: "missing_description"}}},
				{ID: 2, EventID: "evt_02", EventName: "Blues Jam", CreatedAt: now.Add(-24 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
			},
			Total: 2,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "stats", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	var items []ReviewQueueItem
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestReviewStatsEmpty(t *testing.T) {
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

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "stats"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Queue: 0 pending") {
		t.Errorf("expected 'Queue: 0 pending', got:\n%s", output)
	}
	if strings.Contains(output, "WARNING TYPE") {
		t.Error("should not show warning breakdown for empty queue")
	}
}

func TestReviewStatsAuthError(t *testing.T) {
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

	cmd, _, _ := setupReviewCmd(t, []string{"review", "stats"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("error should mention authentication, got: %v", err)
	}
}

func TestReviewStatsNameGroups(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Weekly Jazz Night", CreatedAt: now.Add(-120 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 2, EventID: "evt_02", EventName: "Weekly Jazz Night", CreatedAt: now.Add(-96 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 3, EventID: "evt_03", EventName: "Weekly Jazz Night", CreatedAt: now.Add(-72 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 4, EventID: "evt_04", EventName: "Piano Recital", CreatedAt: now.Add(-48 * time.Hour), Warnings: []Warning{{Code: "missing_description"}}},
			},
			Total: 4,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "stats"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Queue: 4 pending") {
		t.Error("missing queue summary with count 4")
	}
	if !strings.Contains(output, "NAME GROUPS") {
		t.Error("missing name groups section")
	}
	if !strings.Contains(output, "Weekly Jazz Night") {
		t.Error("missing Weekly Jazz Night group")
	}
	if !strings.Contains(output, "3") {
		t.Error("missing group count for Weekly Jazz Night")
	}
	// Piano Recital has only 1 instance, should NOT appear in name groups (≥2 instances)
	if strings.Contains(output, "Piano Recital") {
		t.Error("Piano Recital (1 instance) should not appear in name groups")
	}
}

func TestReviewStatsAgeBuckets(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Recent Event", CreatedAt: now.Add(-1 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 2, EventID: "evt_02", EventName: "Old Event", CreatedAt: now.Add(-800 * time.Hour), Warnings: []Warning{{Code: "missing_description"}}},
			},
			Total: 2,
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "stats"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Queue: 2 pending") {
		t.Error("missing queue summary with count 2")
	}
	if !strings.Contains(output, "0-9 days") {
		t.Error("missing 0-9 days bucket")
	}
	if !strings.Contains(output, "70+ days") {
		t.Error("missing 70+ days bucket")
	}
}
