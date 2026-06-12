package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func setupReviewActionCmd(t *testing.T, args []string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
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

	approveNotes = ""
	approveRecordNotDup = false
	rejectReason = ""
	rejectNotes = ""
	fixNotes = ""
	fixStartDate = ""
	fixEndDate = ""

	testRoot := &cobra.Command{Use: "server"}
	testRoot.AddCommand(reviewCmd)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(errBuf)
	testRoot.SetArgs(args)

	return testRoot, buf, errBuf
}

func TestReviewApproveSuccess(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	var reqMethod, reqPath string
	var reqBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqMethod = r.Method
		reqPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved", "id": 42})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewActionCmd(t, []string{"review", "approve", "42", "--notes", "all good"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Approved review #42") {
		t.Errorf("expected 'Approved review #42', got: %s", output)
	}
	if reqMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", reqMethod)
	}
	if !strings.Contains(reqPath, "/approve") {
		t.Errorf("expected path to contain /approve, got: %s", reqPath)
	}
	if v, ok := reqBody["notes"]; !ok || v != "all good" {
		t.Errorf("expected notes='all good' in body, got: %v", reqBody)
	}
}

func TestReviewRejectSuccess(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	var reqBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "rejected", "id": 42})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewActionCmd(t, []string{"review", "reject", "42", "--reason", "spam"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Rejected review #42") {
		t.Errorf("expected 'Rejected review #42', got: %s", output)
	}
	if v, ok := reqBody["reason"]; !ok || v != "spam" {
		t.Errorf("expected reason='spam' in body, got: %v", reqBody)
	}
}

func TestReviewFixSuccess(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	var reqBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "fixed", "id": 42})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewActionCmd(t, []string{
		"review", "fix", "42",
		"--start-date", "2026-07-15T20:00:00Z",
		"--end-date", "2026-07-15T22:00:00Z",
		"--notes", "corrected dates",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Fixed review #42") {
		t.Errorf("expected 'Fixed review #42', got: %s", output)
	}
	if corrections, ok := reqBody["corrections"].(map[string]any); !ok {
		t.Errorf("expected corrections in body, got: %v", reqBody)
	} else if _, hasStart := corrections["startDate"]; !hasStart {
		t.Error("expected startDate in corrections")
	}
}

func TestReviewActionJSON(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved", "id": 42})
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, buf, _ := setupReviewActionCmd(t, []string{"review", "approve", "42", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["status"] != "approved" {
		t.Errorf("expected status=approved, got: %v", result["status"])
	}
}

func TestReviewActionJSONUnmarshalError(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not valid json at all {{{`))
	}))
	defer server.Close()

	origServer := reviewServerURL
	origToken := reviewTokenFlag
	defer func() { reviewServerURL = origServer; reviewTokenFlag = origToken }()
	reviewServerURL = server.URL
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewActionCmd(t, []string{"review", "approve", "42", "--json"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for corrupt JSON response")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("error should mention unmarshal, got: %v", err)
	}
}

func TestReviewActionAuthError(t *testing.T) {
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

	cmd, _, _ := setupReviewActionCmd(t, []string{"review", "approve", "42"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("error should mention authentication, got: %v", err)
	}
}

func TestReviewActionInvalidID(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origToken := reviewTokenFlag
	defer func() { reviewTokenFlag = origToken }()
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewActionCmd(t, []string{"review", "approve", "notanumber"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for non-integer ID")
	}
	if !strings.Contains(err.Error(), "invalid review ID") {
		t.Errorf("error should mention invalid review ID, got: %v", err)
	}
}

func TestReviewRejectMissingReason(t *testing.T) {
	t.Parallel()
	reviewTestMu.Lock()
	t.Cleanup(func() { reviewTestMu.Unlock() })

	origToken := reviewTokenFlag
	defer func() { reviewTokenFlag = origToken }()
	reviewTokenFlag = "test-jwt"

	cmd, _, _ := setupReviewActionCmd(t, []string{"review", "reject", "42"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for missing --reason")
	}
	if !strings.Contains(err.Error(), "reason is required") {
		t.Errorf("error should mention reason is required, got: %v", err)
	}
}
