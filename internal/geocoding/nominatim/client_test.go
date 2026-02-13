package nominatim

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_Search_Success(t *testing.T) {
	// Mock Nominatim server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify User-Agent header
		userAgent := r.Header.Get("User-Agent")
		if !strings.Contains(userAgent, "Togather/1.0") {
			t.Errorf("unexpected User-Agent: %s", userAgent)
		}

		// Verify query parameters
		query := r.URL.Query()
		if query.Get("q") != "Toronto, ON" {
			t.Errorf("unexpected query: %s", query.Get("q"))
		}
		if query.Get("format") != "jsonv2" {
			t.Errorf("unexpected format: %s", query.Get("format"))
		}
		if query.Get("countrycodes") != "ca" {
			t.Errorf("unexpected countrycodes: %s", query.Get("countrycodes"))
		}

		// Return mock response
		results := []SearchResult{
			{
				PlaceID:     12345,
				Lat:         "43.6532",
				Lon:         "-79.3832",
				DisplayName: "Toronto, Ontario, Canada",
				Type:        "city",
				Class:       "place",
				Importance:  0.9,
				OSMID:       324211,
				OSMType:     "relation",
				Address: &Address{
					City:        "Toronto",
					State:       "Ontario",
					Country:     "Canada",
					CountryCode: "ca",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(results)
	}))
	defer mockServer.Close()

	// Create client pointing to mock server
	client := NewClient(mockServer.URL, "test@example.com", WithRateLimit(100))

	// Execute search
	ctx := context.Background()
	results, err := client.Search(ctx, "Toronto, ON", SearchOptions{
		CountryCodes: "ca",
		Limit:        1,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.DisplayName != "Toronto, Ontario, Canada" {
		t.Errorf("unexpected DisplayName: %s", result.DisplayName)
	}
	if result.Lat != "43.6532" {
		t.Errorf("unexpected Lat: %s", result.Lat)
	}
	if result.Lon != "-79.3832" {
		t.Errorf("unexpected Lon: %s", result.Lon)
	}
}

func TestClient_Search_EmptyQuery(t *testing.T) {
	client := NewClient(DefaultBaseURL, "test@example.com")

	ctx := context.Background()
	_, err := client.Search(ctx, "", SearchOptions{})

	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}

	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestClient_Reverse_Success(t *testing.T) {
	// Mock Nominatim server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query parameters
		query := r.URL.Query()
		if query.Get("lat") != "43.653200" {
			t.Errorf("unexpected lat: %s", query.Get("lat"))
		}
		if query.Get("lon") != "-79.383200" {
			t.Errorf("unexpected lon: %s", query.Get("lon"))
		}
		if query.Get("format") != "jsonv2" {
			t.Errorf("unexpected format: %s", query.Get("format"))
		}

		// Return mock response
		result := ReverseResult{
			PlaceID:     12345,
			Lat:         "43.6532",
			Lon:         "-79.3832",
			DisplayName: "Toronto City Hall, Toronto, Ontario, Canada",
			Type:        "building",
			Class:       "amenity",
			OSMID:       324211,
			OSMType:     "way",
			Address: Address{
				Road:        "Queen Street West",
				City:        "Toronto",
				State:       "Ontario",
				Postcode:    "M5H 2N2",
				Country:     "Canada",
				CountryCode: "ca",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer mockServer.Close()

	// Create client pointing to mock server
	client := NewClient(mockServer.URL, "test@example.com", WithRateLimit(100))

	// Execute reverse geocode
	ctx := context.Background()
	result, err := client.Reverse(ctx, 43.6532, -79.3832)

	if err != nil {
		t.Fatalf("Reverse failed: %v", err)
	}

	if result.DisplayName != "Toronto City Hall, Toronto, Ontario, Canada" {
		t.Errorf("unexpected DisplayName: %s", result.DisplayName)
	}
	if result.Address.City != "Toronto" {
		t.Errorf("unexpected City: %s", result.Address.City)
	}
}

func TestClient_Reverse_InvalidCoordinates(t *testing.T) {
	client := NewClient(DefaultBaseURL, "test@example.com")

	ctx := context.Background()

	// Test invalid latitude
	_, err := client.Reverse(ctx, 91.0, 0.0)
	if err == nil {
		t.Fatal("expected error for invalid latitude, got nil")
	}
	if !strings.Contains(err.Error(), "invalid latitude") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Test invalid longitude
	_, err = client.Reverse(ctx, 0.0, 181.0)
	if err == nil {
		t.Fatal("expected error for invalid longitude, got nil")
	}
	if !strings.Contains(err.Error(), "invalid longitude") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestClient_RateLimit(t *testing.T) {
	requestCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]SearchResult{})
	}))
	defer mockServer.Close()

	// Create client with 10 requests per second
	client := NewClient(mockServer.URL, "test@example.com", WithRateLimit(10))

	ctx := context.Background()
	start := time.Now()

	// Make 3 requests
	for i := 0; i < 3; i++ {
		_, err := client.Search(ctx, "test", SearchOptions{Limit: 1})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
	}

	duration := time.Since(start)

	if requestCount != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}

	// With 10 req/s, 3 requests should take at least 200ms (due to rate limiting)
	minDuration := 200 * time.Millisecond
	if duration < minDuration {
		t.Errorf("rate limiting not working: expected at least %v, got %v", minDuration, duration)
	}
}

func TestClient_Retry_ServerError(t *testing.T) {
	attemptCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			// Return 500 for first attempt
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Return success on second attempt
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]SearchResult{})
	}))
	defer mockServer.Close()

	client := NewClient(mockServer.URL, "test@example.com", WithRateLimit(100))

	ctx := context.Background()
	_, err := client.Search(ctx, "test", SearchOptions{Limit: 1})

	if err != nil {
		t.Fatalf("Search failed after retry: %v", err)
	}

	if attemptCount != 2 {
		t.Errorf("expected 2 attempts, got %d", attemptCount)
	}
}

func TestClient_Retry_MaxRetries(t *testing.T) {
	attemptCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		// Always return 500
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	client := NewClient(mockServer.URL, "test@example.com", WithRateLimit(100))

	ctx := context.Background()
	_, err := client.Search(ctx, "test", SearchOptions{Limit: 1})

	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}

	// Should attempt: initial + MaxRetries (2) = 3 total
	expectedAttempts := 3
	if attemptCount != expectedAttempts {
		t.Errorf("expected %d attempts, got %d", expectedAttempts, attemptCount)
	}
}

func TestClient_Retry_RateLimited(t *testing.T) {
	attemptCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			// Return 429 for first attempt
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		// Return success on second attempt
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]SearchResult{})
	}))
	defer mockServer.Close()

	client := NewClient(mockServer.URL, "test@example.com", WithRateLimit(100))

	ctx := context.Background()
	_, err := client.Search(ctx, "test", SearchOptions{Limit: 1})

	if err != nil {
		t.Fatalf("Search failed after retry: %v", err)
	}

	if attemptCount != 2 {
		t.Errorf("expected 2 attempts, got %d", attemptCount)
	}
}

func TestClient_Timeout(t *testing.T) {
	// Server that delays response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]SearchResult{})
	}))
	defer mockServer.Close()

	// Create client with short timeout
	httpClient := &http.Client{Timeout: 100 * time.Millisecond}
	client := NewClient(mockServer.URL, "test@example.com",
		WithHTTPClient(httpClient),
		WithRateLimit(100))

	ctx := context.Background()
	_, err := client.Search(ctx, "test", SearchOptions{Limit: 1})

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
