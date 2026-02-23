package scraper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// cachedResponse stores the HTTP response metadata and body on disk.
type cachedResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
}

// CachingTransport is an http.RoundTripper that caches responses to disk.
// When Refresh is true, cached reads are skipped but responses are still written.
type CachingTransport struct {
	Wrapped  http.RoundTripper
	CacheDir string
	Refresh  bool
}

// NewCachingTransport creates a caching transport wrapping the given transport.
// cacheDir is created automatically if it doesn't exist.
func NewCachingTransport(wrapped http.RoundTripper, cacheDir string, refresh bool) *CachingTransport {
	return &CachingTransport{
		Wrapped:  wrapped,
		CacheDir: cacheDir,
		Refresh:  refresh,
	}
}

// RoundTrip implements http.RoundTripper. Only GET requests are cached;
// non-2xx responses are not cached. When Refresh is true, cache reads are
// skipped but successful responses are still written to disk.
func (t *CachingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only cache GET requests.
	if req.Method != http.MethodGet {
		return t.Wrapped.RoundTrip(req)
	}

	key := cacheKey(req.URL.String())
	cachePath := filepath.Join(t.CacheDir, key+".json")

	// Try reading from cache (unless refresh mode).
	if !t.Refresh {
		if resp, err := t.readCache(cachePath, req); err == nil {
			log.Debug().Str("url", req.URL.String()).Str("cache_key", key).Msg("scrape cache: hit")
			return resp, nil
		}
	}

	// Cache miss or refresh — make the real request.
	resp, err := t.Wrapped.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Only cache successful responses (2xx).
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if writeErr := t.writeCache(cachePath, resp); writeErr != nil {
			log.Warn().Err(writeErr).Str("url", req.URL.String()).Msg("scrape cache: failed to write")
		} else {
			log.Debug().Str("url", req.URL.String()).Str("cache_key", key).Msg("scrape cache: written")
		}
	}

	return resp, nil
}

func (t *CachingTransport) readCache(path string, req *http.Request) (*http.Response, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cached cachedResponse
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("parse cache: %w", err)
	}

	// Reconstruct http.Response.
	header := make(http.Header)
	for k, v := range cached.Headers {
		header.Set(k, v)
	}

	return &http.Response{
		StatusCode: cached.StatusCode,
		Status:     fmt.Sprintf("%d %s", cached.StatusCode, http.StatusText(cached.StatusCode)),
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(string(cached.Body))),
		Request:    req,
	}, nil
}

func (t *CachingTransport) writeCache(path string, resp *http.Response) error {
	// Ensure cache directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	// Read body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	resp.Body.Close()

	// Replace body so caller can still read it.
	resp.Body = io.NopCloser(strings.NewReader(string(body)))

	// Store headers (flatten to single values).
	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	cached := cachedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

// cacheKey returns a hex-encoded SHA256 hash of the URL string, used as the
// cache filename.
func cacheKey(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:])
}
