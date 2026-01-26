package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestSize(t *testing.T) {
	tests := []struct {
		name           string
		maxBytes       int64
		bodySize       int
		expectStatus   int
		expectBodyRead bool
	}{
		{
			name:           "small request accepted",
			maxBytes:       1024,
			bodySize:       512,
			expectStatus:   http.StatusOK,
			expectBodyRead: true,
		},
		{
			name:           "exact limit accepted",
			maxBytes:       1024,
			bodySize:       1024,
			expectStatus:   http.StatusOK,
			expectBodyRead: true,
		},
		{
			name:           "oversized request rejected",
			maxBytes:       1024,
			bodySize:       2048,
			expectStatus:   http.StatusRequestEntityTooLarge,
			expectBodyRead: false,
		},
		{
			name:           "1MB limit for public endpoints",
			maxBytes:       DefaultMaxBodySize,
			bodySize:       int(DefaultMaxBodySize) + 1,
			expectStatus:   http.StatusRequestEntityTooLarge,
			expectBodyRead: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler that reads the body
			bodyRead := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					// MaxBytesReader returns error for oversized bodies
					assert.Contains(t, err.Error(), "http: request body too large")
					w.WriteHeader(http.StatusRequestEntityTooLarge)
					return
				}
				bodyRead = true
				assert.Len(t, body, tt.bodySize, "body size should match")
				w.WriteHeader(http.StatusOK)
			})

			// Wrap with RequestSize middleware
			middleware := RequestSize(tt.maxBytes)(handler)

			// Create request with body
			body := bytes.Repeat([]byte("x"), tt.bodySize)
			req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			// Execute request
			middleware.ServeHTTP(rec, req)

			// Verify response
			assert.Equal(t, tt.expectStatus, rec.Code, "status code should match")
			assert.Equal(t, tt.expectBodyRead, bodyRead, "body read status should match")
		})
	}
}

func TestPublicRequestSize(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := PublicRequestSize()(handler)

	t.Run("1MB accepted", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), int(DefaultMaxBodySize))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("1MB+1 rejected", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), int(DefaultMaxBodySize)+1)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})
}

func TestFederationRequestSize(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := FederationRequestSize()(handler)

	t.Run("10MB accepted", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), int(FederationMaxBodySize))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/sync", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("10MB+1 rejected", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), int(FederationMaxBodySize)+1)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/sync", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})
}

func TestAdminRequestSize(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := AdminRequestSize()(handler)

	t.Run("5MB accepted", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), int(AdminMaxBodySize))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("5MB+1 rejected", func(t *testing.T) {
		body := bytes.Repeat([]byte("x"), int(AdminMaxBodySize)+1)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})
}

func TestRequestSizeWithNoBody(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestSize(1024)(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "GET request with no body should succeed")
}

func TestRequestSizeWithMultipleReads(t *testing.T) {
	// Verify that MaxBytesReader correctly limits even when body is read in chunks
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 512)
		totalRead := 0
		for {
			n, err := r.Body.Read(buf)
			totalRead += n
			if err == io.EOF {
				break
			}
			if err != nil {
				assert.Contains(t, err.Error(), "http: request body too large")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestSize(1024)(handler)

	// Send 2KB body (exceeds limit)
	body := strings.NewReader(strings.Repeat("x", 2048))
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, "should reject oversized body even with chunked reads")
}
