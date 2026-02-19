package artsdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	// DefaultEndpoint is the public Artsdata W3C Reconciliation API
	DefaultEndpoint = "https://api.artsdata.ca/recon"
	// DefaultUserAgent identifies this client
	DefaultUserAgent = "Togather-SEL/1.0"
	// DefaultTimeout for HTTP requests
	DefaultTimeout = 10 * time.Second
	// DefaultRateLimit is 1 request per second
	DefaultRateLimit = rate.Limit(1.0)
	// MaxRetries for transient errors
	MaxRetries = 2
	// RetryBaseDelay is the initial backoff delay
	RetryBaseDelay = 1 * time.Second
)

// Client communicates with the Artsdata W3C Reconciliation API.
type Client struct {
	httpClient *http.Client
	endpoint   string
	userAgent  string
	limiter    *rate.Limiter
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithRateLimit sets a custom rate limit (requests per second).
func WithRateLimit(rps float64) Option {
	return func(c *Client) {
		c.limiter = rate.NewLimiter(rate.Limit(rps), 1)
	}
}

// WithUserAgent sets a custom User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		c.userAgent = ua
	}
}

// NewClient creates a new Artsdata Reconciliation API client.
func NewClient(endpoint string, opts ...Option) *Client {
	client := &Client{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		endpoint:  endpoint,
		userAgent: DefaultUserAgent,
		limiter:   rate.NewLimiter(DefaultRateLimit, 1),
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// ReconciliationQuery represents a W3C Reconciliation API query.
type ReconciliationQuery struct {
	Query      string          `json:"query"`
	Type       string          `json:"type,omitempty"`
	Properties []QueryProperty `json:"properties,omitempty"`
}

// QueryProperty represents a property constraint in a reconciliation query.
type QueryProperty struct {
	P string `json:"p"` // property path (e.g., "schema:address/schema:postalCode")
	V string `json:"v"` // value
}

// ReconciliationResult represents a single match result from the API.
type ReconciliationResult struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Score float64      `json:"score"`
	Match bool         `json:"match"`
	Type  []ResultType `json:"type,omitempty"`
}

// ResultType represents the type of a reconciliation result.
type ResultType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// resultWrapper wraps the result array.
type resultWrapper struct {
	Result []ReconciliationResult `json:"result"`
}

// Reconcile performs entity reconciliation against Artsdata.
// Returns a map of query IDs to their result arrays.
//
// Per W3C Reconciliation API v0.2 §4.3, queries are sent as form-encoded
// POST with the JSON batch in a "queries" form parameter.
func (c *Client) Reconcile(ctx context.Context, queries map[string]ReconciliationQuery) (map[string][]ReconciliationResult, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("queries cannot be empty")
	}

	// W3C Reconciliation API v0.2 expects queries as a flat batch object
	// (not wrapped in {"queries": ...}), serialized as the "queries" form parameter.
	reqJSON, err := json.Marshal(queries)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Build form-encoded body: queries=<url-encoded-json>
	formData := "queries=" + url.QueryEscape(string(reqJSON))

	// Execute request with retry
	var respBody []byte
	respBody, err = c.doWithRetry(ctx, http.MethodPost, c.endpoint, strings.NewReader(formData), "application/x-www-form-urlencoded", "application/json")
	if err != nil {
		return nil, fmt.Errorf("reconciliation request: %w", err)
	}

	// Parse response
	// The W3C spec returns a flat object where each query ID is a top-level key
	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(respBody, &rawResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	results := make(map[string][]ReconciliationResult)
	for queryID := range queries {
		raw, ok := rawResp[queryID]
		if !ok {
			continue // Query returned no results
		}

		var wrapper resultWrapper
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			return nil, fmt.Errorf("parse result for %s: %w", queryID, err)
		}

		results[queryID] = wrapper.Result
	}

	return results, nil
}

// EntityData is the JSON-LD data returned when dereferencing an Artsdata URI.
type EntityData struct {
	ID          string      `json:"@id"`
	Type        interface{} `json:"@type"`  // can be string or []string
	Name        interface{} `json:"name"`   // can be string or localized object
	SameAs      interface{} `json:"sameAs"` // can be string, object, or array
	Address     *Address    `json:"address,omitempty"`
	Description interface{} `json:"description,omitempty"`
	URL         interface{} `json:"url,omitempty"`
	RawJSON     []byte      `json:"-"` // full response for storage
}

// Address represents a schema:PostalAddress in JSON-LD.
type Address struct {
	Type            interface{} `json:"@type,omitempty"`
	StreetAddress   string      `json:"streetAddress,omitempty"`
	AddressLocality string      `json:"addressLocality,omitempty"`
	AddressRegion   string      `json:"addressRegion,omitempty"`
	PostalCode      string      `json:"postalCode,omitempty"`
	AddressCountry  string      `json:"addressCountry,omitempty"`
}

// Dereference fetches the full JSON-LD entity data from an Artsdata URI.
func (c *Client) Dereference(ctx context.Context, uri string) (*EntityData, error) {
	if uri == "" {
		return nil, fmt.Errorf("uri cannot be empty")
	}

	// Execute request with Accept: application/ld+json
	respBody, err := c.doWithRetry(ctx, http.MethodGet, uri, nil, "", "application/ld+json")
	if err != nil {
		return nil, fmt.Errorf("dereference %s: %w", uri, err)
	}

	// Parse JSON-LD response
	var entity EntityData
	if err := json.Unmarshal(respBody, &entity); err != nil {
		return nil, fmt.Errorf("parse entity data: %w", err)
	}

	// Store raw JSON for provenance
	entity.RawJSON = respBody

	return &entity, nil
}

// ExtractSameAsURIs extracts sameAs URIs from JSON-LD entity data.
// Handles string, []string, and []map[string]interface{} variants.
func ExtractSameAsURIs(data *EntityData) []string {
	if data == nil || data.SameAs == nil {
		return nil
	}

	var uris []string

	switch v := data.SameAs.(type) {
	case string:
		// Single URI as string
		uris = append(uris, v)

	case []interface{}:
		// Array of URIs (strings or objects with @id)
		for _, item := range v {
			switch itemVal := item.(type) {
			case string:
				uris = append(uris, itemVal)
			case map[string]interface{}:
				if id, ok := itemVal["@id"].(string); ok {
					uris = append(uris, id)
				}
			}
		}

	case map[string]interface{}:
		// Single object with @id
		if id, ok := v["@id"].(string); ok {
			uris = append(uris, id)
		}
	}

	return uris
}

// doWithRetry executes an HTTP request with exponential backoff retry logic.
// For POST requests, contentType specifies the Content-Type header.
// The accept parameter sets the Accept header.
func (c *Client) doWithRetry(ctx context.Context, method, reqURL string, body io.Reader, contentType, accept string) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, ...
			delay := RetryBaseDelay * time.Duration(1<<uint(attempt-1))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Wait for rate limiter
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		// Create request (need to recreate body for retries)
		var reqBody io.Reader
		if body != nil {
			// For POST requests with JSON body, we need to recreate the reader
			// This is safe because body is always a bytes.Reader from Reconcile()
			if seeker, ok := body.(io.Seeker); ok {
				if _, err := seeker.Seek(0, io.SeekStart); err != nil {
					return nil, fmt.Errorf("reset request body: %w", err)
				}
				reqBody = body
			} else {
				reqBody = body
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("User-Agent", c.userAgent)
		if method == http.MethodPost && contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}

		// Execute request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http request: %w", err)
			continue // Retry on network errors
		}

		// Read response body
		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue // Retry on read errors
		}

		// Check status code
		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limited (429)")
			continue // Retry rate limits
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBody))
			continue // Retry server errors
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
		}

		return respBody, nil // Success
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}
