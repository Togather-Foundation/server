package scraper

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// countingTransport wraps an http.RoundTripper and counts how many times
// RoundTrip is called.
type countingTransport struct {
	wrapped http.RoundTripper
	calls   int
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.calls++
	return c.wrapped.RoundTrip(req)
}

// TestCachingTransport_CacheMiss verifies that on the first request the real
// server is called and the response is written to the cache directory.
func TestCachingTransport_CacheMiss(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello from server"))
	}))
	defer srv.Close()

	counter := &countingTransport{wrapped: http.DefaultTransport}
	cacheDir := t.TempDir()
	transport := NewCachingTransport(counter, cacheDir, false)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(srv.URL + "/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if string(body) != "hello from server" {
		t.Errorf("body = %q, want %q", body, "hello from server")
	}
	if counter.calls != 1 {
		t.Errorf("real server calls = %d, want 1", counter.calls)
	}
}

// TestCachingTransport_CacheHit verifies that a second request for the same URL
// reads from cache and does not hit the real server.
func TestCachingTransport_CacheHit(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("cached content"))
	}))
	defer srv.Close()

	counter := &countingTransport{wrapped: http.DefaultTransport}
	cacheDir := t.TempDir()
	transport := NewCachingTransport(counter, cacheDir, false)
	client := &http.Client{Transport: transport}

	// First request — populates cache.
	resp1, err := client.Get(srv.URL + "/page")
	if err != nil {
		t.Fatalf("first request error: %v", err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()

	// Second request — should come from cache.
	resp2, err := client.Get(srv.URL + "/page")
	if err != nil {
		t.Fatalf("second request error: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	if string(body2) != string(body1) {
		t.Errorf("cached body = %q, want %q", body2, body1)
	}
	if counter.calls != 1 {
		t.Errorf("real server calls = %d, want 1 (second should be from cache)", counter.calls)
	}
}

// TestCachingTransport_Refresh verifies that with Refresh=true, cache reads are
// skipped but the response is still written to disk.
func TestCachingTransport_Refresh(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("fresh content"))
	}))
	defer srv.Close()

	counter := &countingTransport{wrapped: http.DefaultTransport}
	cacheDir := t.TempDir()

	// First pass: populate cache without refresh.
	populateTransport := NewCachingTransport(counter, cacheDir, false)
	populateClient := &http.Client{Transport: populateTransport}
	resp, err := populateClient.Get(srv.URL + "/page")
	if err != nil {
		t.Fatalf("populate error: %v", err)
	}
	_ = resp.Body.Close()

	// Second pass: use Refresh=true — should bypass cache and hit server again.
	refreshTransport := NewCachingTransport(counter, cacheDir, true)
	refreshClient := &http.Client{Transport: refreshTransport}
	resp2, err := refreshClient.Get(srv.URL + "/page")
	if err != nil {
		t.Fatalf("refresh error: %v", err)
	}
	body, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	if string(body) != "fresh content" {
		t.Errorf("body = %q, want %q", body, "fresh content")
	}
	// Both requests should have hit the real server (2 calls total).
	if counter.calls != 2 {
		t.Errorf("real server calls = %d, want 2 (refresh bypasses cache)", counter.calls)
	}
}

// TestCachingTransport_NonGetNotCached verifies that POST requests are passed
// through without caching.
func TestCachingTransport_NonGetNotCached(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	counter := &countingTransport{wrapped: http.DefaultTransport}
	cacheDir := t.TempDir()
	transport := NewCachingTransport(counter, cacheDir, false)
	client := &http.Client{Transport: transport}

	// First POST.
	resp1, err := client.Post(srv.URL+"/api", "application/json", nil)
	if err != nil {
		t.Fatalf("POST 1 error: %v", err)
	}
	_ = resp1.Body.Close()

	// Second POST — should also hit real server (not cached).
	resp2, err := client.Post(srv.URL+"/api", "application/json", nil)
	if err != nil {
		t.Fatalf("POST 2 error: %v", err)
	}
	_ = resp2.Body.Close()

	if counter.calls != 2 {
		t.Errorf("real server calls = %d, want 2 (POSTs not cached)", counter.calls)
	}
}

// TestCachingTransport_NonSuccessNotCached verifies that 404/500 responses are
// not written to the cache.
func TestCachingTransport_NonSuccessNotCached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	counter := &countingTransport{wrapped: http.DefaultTransport}
	cacheDir := t.TempDir()
	transport := NewCachingTransport(counter, cacheDir, false)
	client := &http.Client{Transport: transport}

	// First request — 404, should not be cached.
	resp1, err := client.Get(srv.URL + "/missing")
	if err != nil {
		t.Fatalf("first request error: %v", err)
	}
	_ = resp1.Body.Close()

	// Second request — should still hit real server (not cached).
	resp2, err := client.Get(srv.URL + "/missing")
	if err != nil {
		t.Fatalf("second request error: %v", err)
	}
	_ = resp2.Body.Close()

	if counter.calls != 2 {
		t.Errorf("real server calls = %d, want 2 (non-2xx not cached)", counter.calls)
	}
}

// TestCacheKey verifies that the same URL always produces the same key and
// different URLs produce different keys.
func TestCacheKey(t *testing.T) {
	key1a := cacheKey("https://example.com/events")
	key1b := cacheKey("https://example.com/events")
	key2 := cacheKey("https://example.com/other")

	if key1a != key1b {
		t.Errorf("same URL produced different keys: %q vs %q", key1a, key1b)
	}
	if key1a == key2 {
		t.Errorf("different URLs produced same key: %q", key1a)
	}
	if len(key1a) != 64 { // SHA256 hex = 64 chars
		t.Errorf("key length = %d, want 64", len(key1a))
	}
}
