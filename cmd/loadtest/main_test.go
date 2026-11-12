package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
