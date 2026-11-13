package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Togather-Foundation/server/internal/loadtest/fixture"
)

func TestRunPropagatesFailuresAndReportsDistribution(t *testing.T) {
	for _, status := range []int{200, 401, 403, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		var out, errOut bytes.Buffer
		exit := run(context.Background(), []string{"--url", server.URL, "--rps", "100", "--duration", "40ms", "--no-ramp", "--json"}, &out, &errOut)
		server.Close()
		if (exit == 0) != (status == 200) {
			t.Fatalf("status %d exit %d: %s", status, exit, errOut.String())
		}
		var result struct {
			Passed    bool `json:"passed"`
			Endpoints map[string]struct {
				StatusCodes map[int]int `json:"status_codes"`
			} `json:"endpoints"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Passed != (status == 200) || len(result.Endpoints) == 0 {
			t.Fatalf("bad report: %s", out.String())
		}
	}
}

func TestCustomFlagsRetainSelectedProfile(t *testing.T) {
	var out, errOut bytes.Buffer
	if exit := run(context.Background(), []string{"--profile", "unknown", "--no-ramp"}, &out, &errOut); exit == 0 {
		t.Fatal("custom flag hid invalid profile")
	}
	if exit := run(context.Background(), []string{"--rps", "0", "--no-ramp"}, &out, &errOut); exit == 0 {
		t.Fatal("invalid explicit RPS accepted")
	}
}

func TestWriteRunRequiresFreshPrivateManifestBeforeRequests(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	args := []string{"--url", server.URL, "--rps", "10", "--duration", "1ms", "--no-ramp", "--read-ratio", "0", "--api-key", "private-test-key", "--json"}
	var out, errOut bytes.Buffer
	if exit := run(context.Background(), args, &out, &errOut); exit == 0 || requests.Load() != 0 {
		t.Fatal("missing manifest permitted requests")
	}
	path := filepath.Join(t.TempDir(), "run.json")
	withManifest := append(append([]string{}, args...), "--run-manifest", path)
	out.Reset()
	errOut.Reset()
	if exit := run(context.Background(), withManifest, &out, &errOut); exit == 0 {
		t.Fatal("failed authentication accepted")
	}
	manifest, err := fixture.Read(path)
	if err != nil || requests.Load() != 1 {
		t.Fatalf("failed preflight did not preserve private capability: err=%v requests=%d", err, requests.Load())
	}
	if strings.Contains(out.String()+errOut.String(), manifest.OwnershipKey) || strings.Contains(out.String()+errOut.String(), "private-test-key") {
		t.Fatal("credential or ownership key leaked")
	}
	out.Reset()
	errOut.Reset()
	if exit := run(context.Background(), withManifest, &out, &errOut); exit == 0 || requests.Load() != 1 {
		t.Fatal("existing manifest reused or HTTP request sent")
	}
	retained, err := fixture.Read(path)
	if err != nil || retained != manifest {
		t.Fatal("existing capability was changed")
	}
}
