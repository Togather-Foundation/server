package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/identity"
	"github.com/spf13/cobra"
)

const identityTestULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

var identityTestMu sync.Mutex

func setupIdentityCmd(t *testing.T, args []string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var origParent *cobra.Command
	if identityCmd.HasParent() {
		origParent = identityCmd.Parent()
		origParent.RemoveCommand(identityCmd)
	}
	t.Cleanup(func() {
		if origParent != nil {
			if identityCmd.HasParent() {
				identityCmd.Parent().RemoveCommand(identityCmd)
			}
			origParent.AddCommand(identityCmd)
		}
	})

	identityJSON = false

	conflictsType = ""
	conflictsLimit = 50
	conflictsIncludeSuppressed = false

	linkAuthority = ""
	linkURI = ""
	linkSource = "manual"
	linkMethod = "manual"
	linkConfidence = 1.0

	identityRejectOther = ""
	identityRejectReason = ""

	testRoot := &cobra.Command{Use: "server"}
	testRoot.AddCommand(identityCmd)

	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(errBuf)
	testRoot.SetArgs(args)

	return testRoot, buf, errBuf
}

// identityGolden compares table output against a golden file under testdata/.
// Set UPDATE_GOLDEN=1 to regenerate.
func identityGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden: %s", path)
		return
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with UPDATE_GOLDEN=1 to generate)", path, err)
	}
	if got != string(expected) {
		t.Errorf("table output mismatch\ngot:\n%s\nexpected:\n%s", got, string(expected))
	}
}

func fixedIdentityTime() time.Time {
	return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
}

// --- check ----------------------------------------------------------------

func TestIdentityCheckJSON(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	fixed := fixedIdentityTime()
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(identity.IdentityView{
			Ref:     identity.IdentityRef{Type: identity.EntityTypePlace, ULID: identityTestULID},
			Primary: map[string]string{"artsdata": "https://kg.artsdata.ca/resource/K11-24"},
			Identifiers: []identity.IdentifierView{
				{Authority: "artsdata", URI: "https://kg.artsdata.ca/resource/K11-24", Method: "manual", Confidence: 1.0, IsPrimary: true, Source: "manual", ObservedAt: fixed},
			},
			Decisions: []identity.DecisionRecord{
				{ID: "idn-" + identityTestULID, Action: "link", CreatedAt: fixed, Actor: "admin"},
			},
		})
	}))
	defer server.Close()

	identityServerURL = server.URL
	identityTokenFlag = "test-jwt"

	cmd, buf, _ := setupIdentityCmd(t, []string{"identity", "check", "place", identityTestULID, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var view identity.IdentityView
	if err := json.Unmarshal(buf.Bytes(), &view); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if view.Ref.Type != identity.EntityTypePlace || view.Ref.ULID != identityTestULID {
		t.Errorf("unexpected ref: %+v", view.Ref)
	}
	if view.Primary["artsdata"] != "https://kg.artsdata.ca/resource/K11-24" {
		t.Errorf("unexpected primary map: %v", view.Primary)
	}
	if len(view.Identifiers) != 1 || !view.Identifiers[0].IsPrimary {
		t.Errorf("unexpected identifiers: %+v", view.Identifiers)
	}
	if len(view.Decisions) != 1 {
		t.Errorf("expected 1 decision, got %d", len(view.Decisions))
	}
	if gotAuth != "Bearer test-jwt" {
		t.Errorf("expected auth header, got %q", gotAuth)
	}
}

func TestIdentityCheckTable(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	fixed := fixedIdentityTime()
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(identity.IdentityView{
			Ref:     identity.IdentityRef{Type: identity.EntityTypePlace, ULID: identityTestULID},
			Primary: map[string]string{"artsdata": "https://kg.artsdata.ca/resource/K11-24"},
			Identifiers: []identity.IdentifierView{
				{Authority: "artsdata", URI: "https://kg.artsdata.ca/resource/K11-24", Method: "manual", Confidence: 1.0, IsPrimary: true, Source: "manual", ObservedAt: fixed},
			},
			Decisions: []identity.DecisionRecord{
				{ID: "idn-" + identityTestULID, Action: "link", CreatedAt: fixed, Actor: "admin"},
			},
		})
	}))
	defer server.Close()

	identityServerURL = server.URL
	identityTokenFlag = "test-jwt"

	cmd, buf, _ := setupIdentityCmd(t, []string{"identity", "check", "place", identityTestULID})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/api/v1/admin/identity/place/"+identityTestULID {
		t.Errorf("unexpected path: %s", gotPath)
	}
	identityGolden(t, "identity_check_table", buf.String())
}

func TestIdentityCheckInvalidType(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	identityTokenFlag = "test-jwt"
	cmd, _, _ := setupIdentityCmd(t, []string{"identity", "check", "event", identityTestULID})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !strings.Contains(err.Error(), "invalid entity type") {
		t.Errorf("error should mention invalid entity type, got: %v", err)
	}
}

func TestIdentityCheckNotFound(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"title":"Not Found","status":404}`))
	}))
	defer server.Close()

	identityServerURL = server.URL
	identityTokenFlag = "test-jwt"

	cmd, _, _ := setupIdentityCmd(t, []string{"identity", "check", "place", identityTestULID})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404, got: %v", err)
	}
}

// --- conflicts ------------------------------------------------------------

func TestIdentityConflictsJSON(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	var gotPath string
	var gotQuery url.Values
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(identity.ConflictsResponse{
			Items: []identity.ConflictItem{
				{
					Ref:       identity.IdentityRef{Type: identity.EntityTypeOrganization, ULID: identityTestULID},
					Candidate: identity.IdentityRef{Type: identity.EntityTypeOrganization, ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAW"},
					Authority: "artsdata",
					URI:       "https://kg.artsdata.ca/resource/K11-24",
					Score:     0.99,
				},
			},
		})
	}))
	defer server.Close()

	identityServerURL = server.URL
	identityTokenFlag = "test-jwt"

	cmd, buf, _ := setupIdentityCmd(t, []string{"identity", "conflicts", "--type", "organization", "--limit", "25", "--include-suppressed", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp identity.ConflictsResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(resp.Items) != 1 || resp.Items[0].Authority != "artsdata" {
		t.Errorf("unexpected items: %+v", resp.Items)
	}
	if gotPath != "/api/v1/admin/identity/conflicts" {
		t.Errorf("unexpected path: %s", gotPath)
	}
	if gotQuery.Get("type") != "organization" {
		t.Errorf("unexpected type query: %v", gotQuery)
	}
	if gotQuery.Get("limit") != "25" {
		t.Errorf("unexpected limit query: %v", gotQuery)
	}
	if gotQuery.Get("include_suppressed") != "true" {
		t.Errorf("unexpected include_suppressed query: %v", gotQuery)
	}
	if gotAuth != "Bearer test-jwt" {
		t.Errorf("expected auth header, got %q", gotAuth)
	}
}

func TestIdentityConflictsTable(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(identity.ConflictsResponse{
			Items: []identity.ConflictItem{
				{
					Ref:       identity.IdentityRef{Type: identity.EntityTypePlace, ULID: identityTestULID},
					Candidate: identity.IdentityRef{Type: identity.EntityTypePlace, ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAW"},
					Authority: "artsdata",
					URI:       "https://kg.artsdata.ca/resource/K11-24",
					Score:     0.99,
				},
				{
					Ref:        identity.IdentityRef{Type: identity.EntityTypePlace, ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAX"},
					Candidate:  identity.IdentityRef{Type: identity.EntityTypePlace, ULID: "01ARZ3NDEKTSV4RRFFQ69G5FAY"},
					Authority:  "wikidata",
					URI:        "https://www.wikidata.org/wiki/Q1234",
					Score:      0.87,
					Suppressed: true,
				},
			},
		})
	}))
	defer server.Close()

	identityServerURL = server.URL
	identityTokenFlag = "test-jwt"

	cmd, buf, _ := setupIdentityCmd(t, []string{"identity", "conflicts", "--type", "place"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	identityGolden(t, "identity_conflicts_table", buf.String())
}

func TestIdentityConflictsMissingType(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	identityTokenFlag = "test-jwt"
	cmd, _, _ := setupIdentityCmd(t, []string{"identity", "conflicts"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --type")
	}
	if !strings.Contains(err.Error(), "--type is required") {
		t.Errorf("error should mention --type is required, got: %v", err)
	}
}

// --- link ----------------------------------------------------------------

func TestIdentityLink(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	var gotMethod, gotPath, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Logf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(identity.DecisionRecord{
			ID:         "idn-" + identityTestULID,
			EntityType: identity.EntityTypePlace,
			EntityID:   identityTestULID,
			Action:     identity.ActionLink,
		})
	}))
	defer server.Close()

	identityServerURL = server.URL
	identityTokenFlag = "test-jwt"

	cmd, buf, _ := setupIdentityCmd(t, []string{
		"identity", "link", "place", identityTestULID,
		"--authority", "artsdata",
		"--uri", "https://kg.artsdata.ca/resource/K11-24",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "Linked place") {
		t.Errorf("expected 'Linked place' output, got: %s", buf.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/admin/identity/link" {
		t.Errorf("unexpected method/path: %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer test-jwt" {
		t.Errorf("expected auth header, got %q", gotAuth)
	}
	if gotBody["entity_type"] != "place" || gotBody["entity_id"] != identityTestULID {
		t.Errorf("unexpected body ref: %v", gotBody)
	}
	if gotBody["authority"] != "artsdata" || gotBody["uri"] != "https://kg.artsdata.ca/resource/K11-24" {
		t.Errorf("unexpected body authority/uri: %v", gotBody)
	}
	if gotBody["method"] != "manual" || gotBody["source"] != "manual" {
		t.Errorf("unexpected body method/source defaults: %v", gotBody)
	}
	if gotBody["confidence"] != 1.0 {
		t.Errorf("unexpected default confidence: %v", gotBody["confidence"])
	}
}

func TestIdentityLinkJSON(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(identity.DecisionRecord{
			ID:         "idn-" + identityTestULID,
			EntityType: identity.EntityTypePlace,
			EntityID:   identityTestULID,
			Action:     identity.ActionLink,
		})
	}))
	defer server.Close()

	identityServerURL = server.URL
	identityTokenFlag = "test-jwt"

	cmd, buf, _ := setupIdentityCmd(t, []string{
		"identity", "link", "place", identityTestULID,
		"--authority", "artsdata",
		"--uri", "https://kg.artsdata.ca/resource/K11-24",
		"--confidence", "0.85",
		"--json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rec identity.DecisionRecord
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if rec.Action != identity.ActionLink || rec.EntityType != identity.EntityTypePlace {
		t.Errorf("unexpected decision: %+v", rec)
	}
	if gotBody["confidence"] != 0.85 {
		t.Errorf("expected confidence 0.85, got %v", gotBody["confidence"])
	}
}

func TestIdentityLinkMissingURI(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	identityTokenFlag = "test-jwt"
	cmd, _, _ := setupIdentityCmd(t, []string{
		"identity", "link", "place", identityTestULID, "--authority", "artsdata",
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --uri")
	}
	if !strings.Contains(err.Error(), "--uri is required") {
		t.Errorf("error should mention --uri is required, got: %v", err)
	}
}

// --- reject ---------------------------------------------------------------

func TestIdentityReject(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	var gotMethod, gotPath, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Logf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(identity.DecisionRecord{
			ID:         "idn-reject",
			EntityType: identity.EntityTypePlace,
			EntityID:   identityTestULID,
			Action:     identity.ActionReject,
		})
	}))
	defer server.Close()

	identityServerURL = server.URL
	identityTokenFlag = "test-jwt"

	cmd, buf, _ := setupIdentityCmd(t, []string{
		"identity", "reject", "place", identityTestULID,
		"--other", "01ARZ3NDEKTSV4RRFFQ69G5FAW",
		"--reason", "different operator",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "Rejected place") {
		t.Errorf("expected 'Rejected place' output, got: %s", buf.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/admin/identity/reject" {
		t.Errorf("unexpected method/path: %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer test-jwt" {
		t.Errorf("expected auth header, got %q", gotAuth)
	}
	if gotBody["entity_type"] != "place" || gotBody["entity_id"] != identityTestULID {
		t.Errorf("unexpected body ref: %v", gotBody)
	}
	if gotBody["counterpart_id"] != "01ARZ3NDEKTSV4RRFFQ69G5FAW" || gotBody["reason"] != "different operator" {
		t.Errorf("unexpected body counterpart/reason: %v", gotBody)
	}
}

func TestIdentityRejectMissingReason(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	identityTokenFlag = "test-jwt"
	cmd, _, _ := setupIdentityCmd(t, []string{
		"identity", "reject", "place", identityTestULID, "--other", "01ARZ3NDEKTSV4RRFFQ69G5FAW",
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --reason")
	}
	if !strings.Contains(err.Error(), "--reason is required") {
		t.Errorf("error should mention --reason is required, got: %v", err)
	}
}

func TestIdentityRejectJSON(t *testing.T) {
	t.Parallel()
	identityTestMu.Lock()
	t.Cleanup(func() { identityTestMu.Unlock() })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(identity.DecisionRecord{
			ID:         "idn-reject",
			EntityType: identity.EntityTypePlace,
			EntityID:   identityTestULID,
			Action:     identity.ActionReject,
		})
	}))
	defer server.Close()

	identityServerURL = server.URL
	identityTokenFlag = "test-jwt"

	cmd, buf, _ := setupIdentityCmd(t, []string{
		"identity", "reject", "place", identityTestULID,
		"--other", "01ARZ3NDEKTSV4RRFFQ69G5FAW",
		"--reason", "different operator",
		"--json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rec identity.DecisionRecord
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if rec.Action != identity.ActionReject {
		t.Errorf("unexpected action: %q", rec.Action)
	}
}
