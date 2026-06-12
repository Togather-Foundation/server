package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReviewEditSuccess(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	var putReqMethod, putReqPath string
	var putReqBody map[string]any
	editCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		editCalls++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			putReqMethod = r.Method
			putReqPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&putReqBody); err != nil {
				t.Logf("failed to decode request body: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"updated": true})
			return
		}
		_ = json.NewEncoder(w).Encode(ReviewQueueDetail{
			ReviewQueueItem: ReviewQueueItem{
				ID:        42,
				EventID:   "evt_abc123",
				EventName: "Weekly Jazz Night",
				CreatedAt: now.Add(-24 * time.Hour),
			},
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{
		"review", "edit", "42",
		"--name", "Updated Name",
		"--description", "New description",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Updated event evt_abc123") {
		t.Errorf("expected 'Updated event evt_abc123', got:\n%s", output)
	}
	if !strings.Contains(output, "Weekly Jazz Night") {
		t.Error("expected event name 'Weekly Jazz Night' in output")
	}
	if !strings.Contains(output, "name and description") {
		t.Errorf("expected 'name and description' in output, got:\n%s", output)
	}
	if putReqMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", putReqMethod)
	}
	if !strings.Contains(putReqPath, "evt_abc123") {
		t.Errorf("expected PUT path to contain evt_abc123, got: %s", putReqPath)
	}
	if putReqBody["name"] != "Updated Name" {
		t.Errorf("expected name='Updated Name', got: %v", putReqBody["name"])
	}
	if putReqBody["description"] != "New description" {
		t.Errorf("expected description='New description', got: %v", putReqBody["description"])
	}
	if editCalls != 2 {
		t.Errorf("expected 2 calls (1 fetch + 1 PUT), got %d", editCalls)
	}
}

func TestReviewEditJSON(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			_ = json.NewEncoder(w).Encode(map[string]any{"updated": true, "event_id": "evt_abc123"})
			return
		}
		_ = json.NewEncoder(w).Encode(ReviewQueueDetail{
			ReviewQueueItem: ReviewQueueItem{
				ID:        42,
				EventID:   "evt_abc123",
				EventName: "Weekly Jazz Night",
				CreatedAt: now.Add(-24 * time.Hour),
			},
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "edit", "42", "--name", "JSON Name", "--json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output == "" {
		t.Fatal("expected non-empty JSON output")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, output)
	}

	updated, ok := result["updated"].(bool)
	if !ok || !updated {
		t.Errorf("expected updated=true, got: %v", result["updated"])
	}

	eventID, ok := result["event_id"].(string)
	if !ok || eventID != "evt_abc123" {
		t.Errorf("expected event_id=evt_abc123, got: %v", result["event_id"])
	}
}

func TestReviewEditDryRun(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueDetail{
			ReviewQueueItem: ReviewQueueItem{
				ID:        42,
				EventID:   "evt_abc123",
				EventName: "Weekly Jazz Night",
				CreatedAt: now.Add(-24 * time.Hour),
			},
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{
		"review", "edit", "42",
		"--name", "Updated Name",
		"--description", "New description",
		"--image", "https://example.com/img.jpg",
		"--url", "https://example.com/event",
		"--domain", "music",
		"--dry-run",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Would update event evt_abc123") {
		t.Errorf("expected 'Would update event', got:\n%s", output)
	}
	if !strings.Contains(output, "Weekly Jazz Night") {
		t.Error("expected event name in dry-run output")
	}
	checks := []string{"name", "description", "image", "url", "domain"}
	for _, c := range checks {
		if !strings.Contains(output, c) {
			t.Errorf("patch field %q missing from dry-run output", c)
		}
	}
	if !strings.Contains(output, "Run again without --dry-run to execute") {
		t.Error("expected hint to run without --dry-run")
	}
}

func TestReviewEditDryRunJSON(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueDetail{
			ReviewQueueItem: ReviewQueueItem{
				ID:        42,
				EventID:   "evt_abc123",
				EventName: "Weekly Jazz Night",
				CreatedAt: now.Add(-24 * time.Hour),
			},
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{
		"review", "edit", "42",
		"--name", "Updated Name",
		"--dry-run",
		"--json",
	})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output == "" {
		t.Fatal("expected non-empty JSON output")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, output)
	}

	dryRun, ok := result["dry_run"].(bool)
	if !ok || !dryRun {
		t.Errorf("expected dry_run=true, got: %v", result["dry_run"])
	}

	eventID, ok := result["event_id"].(string)
	if !ok || eventID != "evt_abc123" {
		t.Errorf("expected event_id=evt_abc123, got: %v", result["event_id"])
	}

	patches, ok := result["patches"].([]interface{})
	if !ok || len(patches) != 1 {
		t.Errorf("expected patches=[\"name\"], got: %v", result["patches"])
	} else if patches[0] != "name" {
		t.Errorf("expected patches[0]=\"name\", got: %v", patches[0])
	}
}

func TestReviewEditAuthError(t *testing.T) {
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

	cmd, _, _ := setupReviewCmd(t, []string{"review", "edit", "42", "--name", "New Name"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("error should mention authentication, got: %v", err)
	}
}

func TestReviewEditNoPatches(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origToken := reviewTokenFlag
	defer func() { reviewTokenFlag = origToken }()
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewCmd(t, []string{"review", "edit", "42"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no edit flags are provided")
	}
	if !strings.Contains(err.Error(), "at least one of") {
		t.Errorf("error should mention required flags, got: %v", err)
	}
}

func TestReviewEditInvalidID(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origToken := reviewTokenFlag
	defer func() { reviewTokenFlag = origToken }()
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewCmd(t, []string{"review", "edit", "notanumber", "--name", "Test"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for non-integer ID")
	}
	if !strings.Contains(err.Error(), "invalid review ID") {
		t.Errorf("error should mention invalid review ID, got: %v", err)
	}
}
