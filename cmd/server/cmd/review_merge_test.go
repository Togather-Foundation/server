package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReviewMergeDryRun(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origToken := reviewTokenFlag
	defer func() { reviewTokenFlag = origToken }()
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "merge", "evt_primary", "evt_duplicate", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Would consolidate 1 event(s) into evt_primary") {
		t.Errorf("expected dry-run consolidation message, got:\n%s", output)
	}
	if !strings.Contains(output, "retire evt_duplicate") {
		t.Error("expected retire evt_duplicate line")
	}
	if !strings.Contains(output, "Run again without --dry-run to execute") {
		t.Error("expected hint to run without --dry-run")
	}
}

func TestReviewMergeDryRunJSON(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origToken := reviewTokenFlag
	defer func() { reviewTokenFlag = origToken }()
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "merge", "evt_primary", "evt_duplicate", "--dry-run", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["event_ulid"] != "evt_primary" {
		t.Errorf("expected event_ulid=evt_primary, got: %v", result["event_ulid"])
	}
	if result["dry_run"] != true {
		t.Errorf("expected dry_run=true, got: %v", result["dry_run"])
	}
	retire, ok := result["retire"].([]any)
	if !ok || len(retire) != 1 || retire[0] != "evt_duplicate" {
		t.Errorf("expected retire=[evt_duplicate], got: %v", result["retire"])
	}
}

func TestReviewMergeExecute(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	var reqBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Logf("failed to decode request body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"consolidated": true})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "merge", "evt_primary", "evt_duplicate"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Consolidated 1 event(s) into evt_primary") {
		t.Errorf("expected consolidation success message, got:\n%s", output)
	}
	if reqBody["event_ulid"] != "evt_primary" {
		t.Errorf("expected event_ulid=evt_primary, got: %v", reqBody["event_ulid"])
	}
}

func TestReviewMergeExecuteJSON(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"consolidated": true, "retired": []string{"evt_duplicate"}})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "merge", "evt_primary", "evt_duplicate", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["consolidated"] != true {
		t.Errorf("expected consolidated=true, got: %v", result["consolidated"])
	}
}

func TestReviewMergeWithPatches(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	var reqBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Logf("failed to decode request body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"consolidated": true})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewCmd(t, []string{
		"review", "merge", "evt_primary", "evt_duplicate",
		"--name", "Updated Name",
		"--description", "Updated description",
		"--image", "https://example.com/img.jpg",
		"--url", "https://example.com/event",
		"--domain", "music",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	event, ok := reqBody["event"].(map[string]any)
	if !ok {
		t.Fatalf("expected event in body, got: %v", reqBody)
	}
	if event["name"] != "Updated Name" {
		t.Errorf("expected name='Updated Name', got: %v", event["name"])
	}
	if event["description"] != "Updated description" {
		t.Errorf("expected description='Updated description', got: %v", event["description"])
	}
	if event["image"] != "https://example.com/img.jpg" {
		t.Errorf("expected image URL, got: %v", event["image"])
	}
	if event["url"] != "https://example.com/event" {
		t.Errorf("expected URL, got: %v", event["url"])
	}
	if event["eventDomain"] != "music" {
		t.Errorf("expected eventDomain='music', got: %v", event["eventDomain"])
	}
}

func TestReviewMergeTransferOccurrences(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	var reqBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Logf("failed to decode request body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"consolidated": true})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewCmd(t, []string{
		"review", "merge", "evt_primary", "evt_duplicate",
		"--transfer-occurrences",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reqBody["transfer_occurrences"] != true {
		t.Errorf("expected transfer_occurrences=true, got: %v", reqBody["transfer_occurrences"])
	}
}

func TestReviewMergeTransferOccurrencesDryRun(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origToken := reviewTokenFlag
	defer func() { reviewTokenFlag = origToken }()
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{
		"review", "merge", "evt_primary", "evt_duplicate",
		"--transfer-occurrences", "--dry-run",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "transfer occurrences from retired events") {
		t.Error("expected transfer occurrences note in dry-run output")
	}
}

func TestReviewMergeAuthError(t *testing.T) {
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

	cmd, _, _ := setupReviewCmd(t, []string{"review", "merge", "evt_primary", "evt_duplicate"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("error should mention authentication, got: %v", err)
	}
}

func TestReviewConsolidate(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	var reqBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Logf("failed to decode request body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"consolidated": true})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "consolidate", "evt_canon", "dup1", "dup2", "dup3"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Consolidated 3 event(s) into evt_canon") {
		t.Errorf("expected consolidation success message, got:\n%s", output)
	}
	retire, ok := reqBody["retire"].([]any)
	if !ok || len(retire) != 3 {
		t.Errorf("expected 3 retire IDs, got: %v", reqBody["retire"])
	}
}

func TestReviewConsolidateDryRun(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origToken := reviewTokenFlag
	defer func() { reviewTokenFlag = origToken }()
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{"review", "consolidate", "evt_canon", "dup1", "dup2", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Would consolidate 2 event(s) into evt_canon") {
		t.Errorf("expected dry-run message, got:\n%s", output)
	}
	if !strings.Contains(output, "retire dup1") {
		t.Error("expected retire dup1")
	}
	if !strings.Contains(output, "retire dup2") {
		t.Error("expected retire dup2")
	}
}

func TestReviewConsolidateWithPatchesDryRun(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origToken := reviewTokenFlag
	defer func() { reviewTokenFlag = origToken }()
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCmd(t, []string{
		"review", "consolidate", "evt_canon", "dup1",
		"--name", "Merged Name",
		"--transfer-occurrences",
		"--dry-run",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Would consolidate 1 event(s) into evt_canon") {
		t.Errorf("expected dry-run message, got:\n%s", output)
	}
	if !strings.Contains(output, "transfer occurrences") {
		t.Error("expected transfer occurrences note")
	}
	if !strings.Contains(output, "patch fields") {
		t.Error("expected patch fields note")
	}
	if !strings.Contains(output, "name") {
		t.Error("expected name in patch list")
	}
}
