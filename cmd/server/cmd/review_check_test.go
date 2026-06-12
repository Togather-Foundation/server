package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func setupReviewCheckCmd(t *testing.T, args []string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
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

	testRoot := &cobra.Command{Use: "server"}
	testRoot.AddCommand(reviewCmd)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(errBuf)
	testRoot.SetArgs(args)

	return testRoot, buf, errBuf
}

func TestReviewCheckJSON(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	startTime := time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)
	src := "source-abc"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueDetail{
			ReviewQueueItem: ReviewQueueItem{
				ID:             42,
				EventID:        "evt_abc123",
				Status:         "pending",
				Warnings:       []Warning{{Code: "missing_description", Field: "description", Message: "no description"}},
				CreatedAt:      now.Add(-24 * time.Hour),
				EventName:      "Weekly Jazz Night",
				EventStartTime: &startTime,
				SourceID:       &src,
			},
			Original:   map[string]any{"name": "Weekly Jazz Night", "description": ""},
			Normalized: map[string]any{"name": "weekly jazz night", "description": ""},
			Changes: []ChangeDetail{
				{Field: "name", Original: "Weekly Jazz Night", Corrected: "weekly jazz night", Reason: "normalization"},
			},
			RelatedEvents: []RelatedEventDetail{
				{ULID: "evt_xyz789", Name: "Jazz Night Series", Similarity: ptrFloat(0.85)},
			},
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCheckCmd(t, []string{"review", "check", "42", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	var detail ReviewQueueDetail
	if err := json.Unmarshal([]byte(output), &detail); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if detail.ID != 42 {
		t.Errorf("expected ID 42, got %d", detail.ID)
	}
	if detail.EventName != "Weekly Jazz Night" {
		t.Errorf("unexpected event name: %s", detail.EventName)
	}
	if len(detail.Changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(detail.Changes))
	}
	if len(detail.RelatedEvents) != 1 {
		t.Errorf("expected 1 related event, got %d", len(detail.RelatedEvents))
	}
}

func TestReviewCheckText(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	now := time.Now()
	startTime := time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)
	src := "source-abc"
	rejectionReason := "spam content"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ReviewQueueDetail{
			ReviewQueueItem: ReviewQueueItem{
				ID:              42,
				EventID:         "evt_abc123",
				Status:          "rejected",
				Warnings:        []Warning{{Code: "missing_description", Field: "description", Message: "no description"}},
				CreatedAt:       now.Add(-24 * time.Hour),
				EventName:       "Weekly Jazz Night",
				EventStartTime:  &startTime,
				SourceID:        &src,
				RejectionReason: &rejectionReason,
			},
			ReviewNotes: ptrString("looks good"),
		})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewCheckCmd(t, []string{"review", "check", "42"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	checks := []string{
		"Review #42",
		"rejected",
		"Event: Weekly Jazz Night (evt_abc123)",
		"Date: 2026-07-15",
		"Source: source-abc",
		"Warnings (1):",
		"[missing_description] description: no description",
		"Notes: looks good",
		"Rejection reason: spam content",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\noutput:\n%s", check, output)
		}
	}
}

func TestReviewCheckNotFound(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"title": "Not Found", "status": 404}`))
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewCheckCmd(t, []string{"review", "check", "99"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status 404, got: %v", err)
	}
}

func TestReviewCheckAuthError(t *testing.T) {
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

	cmd, _, _ := setupReviewCheckCmd(t, []string{"review", "check", "42"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("error should mention authentication, got: %v", err)
	}
}

func ptrFloat(f float64) *float64 { return &f }
func ptrString(s string) *string  { return &s }
