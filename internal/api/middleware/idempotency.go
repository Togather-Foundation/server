package middleware

import (
	"context"
	"net/http"
	"strings"
)

type idempotencyKey string

const (
	IdempotencyHeader          = "Idempotency-Key"
	idempotencyContextKey      = idempotencyKey("idempotencyKey")
	maxIdempotencyKeyLength    = 128
	trimmedIdempotencyKeyLimit = 128
)

func Idempotency(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get(IdempotencyHeader))
		if key != "" {
			if len(key) > maxIdempotencyKeyLength {
				key = key[:trimmedIdempotencyKeyLimit]
			}
			ctx := context.WithValue(r.Context(), idempotencyContextKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func IdempotencyKey(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value, ok := r.Context().Value(idempotencyContextKey).(string); ok {
		return value
	}
	return ""
}
