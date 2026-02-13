package nominatim

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

const (
	// DefaultBaseURL is the public Nominatim API endpoint
	DefaultBaseURL = "https://nominatim.openstreetmap.org"
	// DefaultUserAgent follows OSM usage policy requirements
	DefaultUserAgent = "Togather/1.0"
	// DefaultTimeout for HTTP requests
	DefaultTimeout = 5 * time.Second
	// DefaultRateLimit is 1 request per second (OSM policy)
	DefaultRateLimit = rate.Limit(1.0)
	// MaxRetries for transient errors
	MaxRetries = 2
	// RetryBaseDelay is the initial backoff delay
	RetryBaseDelay = 1 * time.Second
)

// Client handles communication with the Nominatim geocoding API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
	email      string
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

// NewClient creates a new Nominatim API client.
// baseURL should be the Nominatim API endpoint (e.g., "https://nominatim.openstreetmap.org").
// email is included in the User-Agent header per OSM usage policy.
func NewClient(baseURL, email string, opts ...Option) *Client {
	client := &Client{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		baseURL:   baseURL,
		userAgent: fmt.Sprintf("%s (%s)", DefaultUserAgent, email),
		email:     email,
		limiter:   rate.NewLimiter(DefaultRateLimit, 1),
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// Search performs forward geocoding (query -> coordinates).
// Returns up to opts.Limit results (default: 1).
func (c *Client) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	// Build query parameters
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "jsonv2")
	params.Set("addressdetails", "1")

	if opts.CountryCodes != "" {
		params.Set("countrycodes", opts.CountryCodes)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}
	params.Set("limit", strconv.Itoa(limit))

	if opts.Viewbox != nil {
		viewbox := fmt.Sprintf("%f,%f,%f,%f",
			opts.Viewbox.MinLon, opts.Viewbox.MinLat,
			opts.Viewbox.MaxLon, opts.Viewbox.MaxLat)
		params.Set("viewbox", viewbox)
		params.Set("bounded", "1")
	}

	requestURL := fmt.Sprintf("%s/search?%s", c.baseURL, params.Encode())

	var results []SearchResult
	err := c.doWithRetry(ctx, requestURL, &results)
	if err != nil {
		return nil, fmt.Errorf("search geocoding: %w", err)
	}

	return results, nil
}

// Reverse performs reverse geocoding (coordinates -> address).
func (c *Client) Reverse(ctx context.Context, lat, lon float64) (*ReverseResult, error) {
	if lat < -90 || lat > 90 {
		return nil, fmt.Errorf("invalid latitude: %f (must be between -90 and 90)", lat)
	}
	if lon < -180 || lon > 180 {
		return nil, fmt.Errorf("invalid longitude: %f (must be between -180 and 180)", lon)
	}

	// Build query parameters
	params := url.Values{}
	params.Set("lat", strconv.FormatFloat(lat, 'f', 6, 64))
	params.Set("lon", strconv.FormatFloat(lon, 'f', 6, 64))
	params.Set("format", "jsonv2")
	params.Set("addressdetails", "1")

	requestURL := fmt.Sprintf("%s/reverse?%s", c.baseURL, params.Encode())

	var result ReverseResult
	err := c.doWithRetry(ctx, requestURL, &result)
	if err != nil {
		return nil, fmt.Errorf("reverse geocoding: %w", err)
	}

	return &result, nil
}

// doWithRetry executes an HTTP GET request with exponential backoff retry logic.
func (c *Client) doWithRetry(ctx context.Context, requestURL string, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, ...
			delay := RetryBaseDelay * time.Duration(1<<uint(attempt-1))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Wait for rate limiter
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter: %w", err)
		}

		// Create request
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("User-Agent", c.userAgent)

		// Execute request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http request: %w", err)
			continue // Retry on network errors
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
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
			lastErr = fmt.Errorf("server error (%d)", resp.StatusCode)
			continue // Retry server errors
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
		}

		// Parse JSON response
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("parse json: %w", err)
		}

		return nil // Success
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}
