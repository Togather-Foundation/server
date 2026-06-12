package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReviewBatchDryRun(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", CreatedAt: now.Add(-48 * time.Hour)},
				{ID: 2, EventID: "evt_02", EventName: "Jazz Night Late", CreatedAt: now.Add(-24 * time.Hour)},
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

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "batch", "--name", "Jazz", "--action", "approve", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Would approved") {
		t.Errorf("expected 'Would approved' in dry-run output, got:\n%s", output)
	}
	if !strings.Contains(output, "Jazz Night") {
		t.Error("Jazz Night missing from dry-run list")
	}
	if !strings.Contains(output, "Jazz Night Late") {
		t.Error("Jazz Night Late missing from dry-run list")
	}
}

func TestReviewBatchExecute(t *testing.T) {
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	t.Setenv("REVIEW_BATCH_DELAY_MS", "0")

	var approveCalls int
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/approve") {
			approveCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved"})
			return
		}
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", CreatedAt: now.Add(-48 * time.Hour)},
				{ID: 2, EventID: "evt_02", EventName: "Jazz Night Late", CreatedAt: now.Add(-24 * time.Hour)},
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

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "batch", "--name", "Jazz", "--action", "approve"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if approveCalls != 2 {
		t.Errorf("expected 2 approve calls, got %d", approveCalls)
	}
	if !strings.Contains(output, "2 approved") {
		t.Errorf("expected summary '2 approved', got:\n%s", output)
	}
}

func TestReviewBatchJSON(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", CreatedAt: now.Add(-48 * time.Hour)},
				{ID: 2, EventID: "evt_02", EventName: "Jazz Night Late", CreatedAt: now.Add(-24 * time.Hour)},
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

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "batch", "--name", "Jazz", "--json"})
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

func TestReviewBatchJSONNoMatches(t *testing.T) {
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

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "batch", "--name", "Nonexistent", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	var items []ReviewQueueItem
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(items) != 0 {
		t.Errorf("expected empty array, got %d items", len(items))
	}
}

func TestReviewBatchAuthError(t *testing.T) {
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	t.Setenv("REVIEW_BATCH_DELAY_MS", "0")

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/approve") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error": "invalid token"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", CreatedAt: now.Add(-48 * time.Hour)},
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

	cmd, _, _ := setupReviewCmd(t, []string{"review", "batch", "--name", "Jazz", "--action", "approve"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 401 during batch processing")
	}
	if !strings.Contains(err.Error(), "authentication error") {
		t.Errorf("error should mention authentication error, got: %v", err)
	}
}

func TestReviewBatchNoAction(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", CreatedAt: now.Add(-48 * time.Hour)},
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

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "batch", "--name", "Jazz"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Would processed") {
		t.Errorf("expected 'Would processed' when no --action, got:\n%s", output)
	}
	if !strings.Contains(output, "Jazz Night") {
		t.Error("Jazz Night missing from list")
	}
}

func TestReviewBatchWithSourceFilter(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	srcA := "source-a"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", SourceID: &srcA, CreatedAt: now.Add(-48 * time.Hour)},
				{ID: 2, EventID: "evt_02", EventName: "Blues Jam", CreatedAt: now.Add(-24 * time.Hour)},
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

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "batch", "--source", "source-a", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Jazz Night") {
		t.Error("Jazz Night (source-a) should match source filter")
	}
	if strings.Contains(output, "Blues Jam") {
		t.Error("Blues Jam (no source) should be filtered out")
	}
}

func TestReviewBatchWithWarningFilter(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueListResponse{
			Items: []ReviewQueueItem{
				{ID: 1, EventID: "evt_01", EventName: "Jazz Night", CreatedAt: now.Add(-48 * time.Hour), Warnings: []Warning{{Code: "bad_date"}}},
				{ID: 2, EventID: "evt_02", EventName: "Blues Jam", CreatedAt: now.Add(-24 * time.Hour), Warnings: []Warning{{Code: "missing_description"}}},
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

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "batch", "--warning", "bad_date", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Jazz Night") {
		t.Error("Jazz Night should match bad_date warning")
	}
	if strings.Contains(output, "Blues Jam") {
		t.Error("Blues Jam should be filtered out by warning")
	}
}

func TestReviewBatchNoFilter(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origToken := reviewTokenFlag
	defer func() { reviewTokenFlag = origToken }()
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewCmd(t, []string{"review", "batch"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no filter is provided")
	}
	if !strings.Contains(err.Error(), "at least one filter is required") {
		t.Errorf("error should mention filter requirement, got: %v", err)
	}
}
