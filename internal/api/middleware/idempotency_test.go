package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdempotencyAddsKeyToContext(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "abc-123", IdempotencyKey(r))
		w.WriteHeader(http.StatusOK)
	})

	middleware := Idempotency(handler)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	req.Header.Set(IdempotencyHeader, "abc-123")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestIdempotencyTrimsWhitespace(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "key-value", IdempotencyKey(r))
		w.WriteHeader(http.StatusOK)
	})

	middleware := Idempotency(handler)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	req.Header.Set(IdempotencyHeader, "  key-value  ")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestIdempotencyTruncatesLongKey(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := IdempotencyKey(r)
		assert.Len(t, value, trimmedIdempotencyKeyLimit)
		assert.Equal(t, strings.Repeat("a", trimmedIdempotencyKeyLimit), value)
		w.WriteHeader(http.StatusOK)
	})

	middleware := Idempotency(handler)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	req.Header.Set(IdempotencyHeader, strings.Repeat("a", maxIdempotencyKeyLength+10))
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestIdempotencyMissingKey(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "", IdempotencyKey(r))
		w.WriteHeader(http.StatusOK)
	})

	middleware := Idempotency(handler)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestIdempotencyNilRequest(t *testing.T) {
	assert.Equal(t, "", IdempotencyKey(nil))
}
