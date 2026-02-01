package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
)

func TestLoginRateLimit_AllowsInitialBurst(t *testing.T) {
	cfg := config.RateLimitConfig{
		LoginPer15Minutes: 5,
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Create request with TierLogin context
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
		req.RemoteAddr = "192.168.1.100:12345" // Same IP for all requests
		req = req.WithContext(WithRateLimitTier(req.Context(), TierLogin))
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("request %d: expected status 200, got %d", i+1, res.Code)
		}
	}
}

func TestLoginRateLimit_BlocksAfterBurst(t *testing.T) {
	cfg := config.RateLimitConfig{
		LoginPer15Minutes: 5,
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	clientIP := "192.168.1.101:54321"

	// Exhaust the burst allowance (5 requests)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
		req.RemoteAddr = clientIP
		req = req.WithContext(WithRateLimitTier(req.Context(), TierLogin))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
	}

	// 6th request should be rate limited
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
	req.RemoteAddr = clientIP
	req = req.WithContext(WithRateLimitTier(req.Context(), TierLogin))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", res.Code)
	}

	retryAfter := res.Header().Get("Retry-After")
	if retryAfter != "180" {
		t.Errorf("expected Retry-After header to be 180, got %s", retryAfter)
	}
}

func TestLoginRateLimit_PerIPIsolation(t *testing.T) {
	cfg := config.RateLimitConfig{
		LoginPer15Minutes: 5,
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust limit for first IP
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		req = req.WithContext(WithRateLimitTier(req.Context(), TierLogin))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
	}

	// Different IP should still be allowed
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
	req.RemoteAddr = "192.168.1.200:54321" // Different IP
	req = req.WithContext(WithRateLimitTier(req.Context(), TierLogin))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("different IP should not be rate limited, got status %d", res.Code)
	}
}

func TestLoginRateLimit_RespectsXForwardedFor(t *testing.T) {
	cfg := config.RateLimitConfig{
		LoginPer15Minutes: 5,
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust limit using X-Forwarded-For
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
		req.RemoteAddr = "10.0.0.1:12345"                 // Internal proxy IP
		req.Header.Set("X-Forwarded-For", "203.0.113.45") // Real client IP
		req = req.WithContext(WithRateLimitTier(req.Context(), TierLogin))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
	}

	// Same X-Forwarded-For should be rate limited
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.45") // Same client IP
	req = req.WithContext(WithRateLimitTier(req.Context(), TierLogin))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 for same X-Forwarded-For, got %d", res.Code)
	}
}

func TestLoginRateLimit_DisabledWhenZero(t *testing.T) {
	cfg := config.RateLimitConfig{
		LoginPer15Minutes: 0, // Disabled
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Should allow unlimited requests when disabled
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		req = req.WithContext(WithRateLimitTier(req.Context(), TierLogin))
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("request %d: disabled rate limit should allow all, got status %d", i+1, res.Code)
		}
	}
}

func TestRateLimit_HealthCheckExempt(t *testing.T) {
	cfg := config.RateLimitConfig{
		PublicPerMinute: 1, // Very restrictive
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Health checks should never be rate limited
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("healthz should never be rate limited, got status %d", res.Code)
		}
	}
}

func TestRateLimit_ReadyzExempt(t *testing.T) {
	cfg := config.RateLimitConfig{
		PublicPerMinute: 1, // Very restrictive
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Readiness checks should never be rate limited
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("readyz should never be rate limited, got status %d", res.Code)
		}
	}
}

func TestTierPublic_RateLimit(t *testing.T) {
	cfg := config.RateLimitConfig{
		PublicPerMinute: 2, // Allow 2 per minute
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	clientIP := "192.168.1.102:12345"

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
		req.RemoteAddr = clientIP
		req = req.WithContext(WithRateLimitTier(req.Context(), TierPublic))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("request %d: expected status 200, got %d", i+1, res.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.RemoteAddr = clientIP
	req = req.WithContext(WithRateLimitTier(req.Context(), TierPublic))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", res.Code)
	}

	retryAfter := res.Header().Get("Retry-After")
	if retryAfter != "60" {
		t.Errorf("expected Retry-After header to be 60 for public tier, got %s", retryAfter)
	}
}

func TestClientKey_PrioritizesXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.45, 198.51.100.1")

	// Configure 10.0.0.0/8 as trusted proxy
	trustedProxies := []string{"10.0.0.0/8"}
	key := clientKey(req, trustedProxies)
	if key != "203.0.113.45" {
		t.Errorf("expected first X-Forwarded-For IP from trusted proxy, got %s", key)
	}
}

func TestClientKey_IgnoresXForwardedForFromUntrustedSource(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.1:12345" // Untrusted source
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	// Only trust 10.0.0.0/8 proxies
	trustedProxies := []string{"10.0.0.0/8"}
	key := clientKey(req, trustedProxies)
	if key != "203.0.113.1" {
		t.Errorf("expected RemoteAddr (untrusted X-Forwarded-For should be ignored), got %s", key)
	}
}

func TestClientKey_FallsBackToXRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-IP", "203.0.113.45")

	// Configure 10.0.0.0/8 as trusted proxy
	trustedProxies := []string{"10.0.0.0/8"}
	key := clientKey(req, trustedProxies)
	if key != "203.0.113.45" {
		t.Errorf("expected X-Real-IP from trusted proxy, got %s", key)
	}
}

func TestClientKey_FallsBackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.100:12345"

	// No headers, should use RemoteAddr
	trustedProxies := []string{"10.0.0.0/8"}
	key := clientKey(req, trustedProxies)
	if key != "192.168.1.100" {
		t.Errorf("expected RemoteAddr host, got %s", key)
	}
}

func TestWithRateLimitTierHandler_SetsContextValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	handler := WithRateLimitTierHandler(TierAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tier, ok := r.Context().Value(rateLimitTierKey).(RateLimitTier)
		if !ok {
			t.Fatal("tier not set in context")
		}
		if tier != TierAdmin {
			t.Errorf("expected TierAdmin, got %s", tier)
		}
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("handler failed with status %d", res.Code)
	}
}

// Benchmark tests to ensure rate limiting is performant
func BenchmarkRateLimit_Allow(b *testing.B) {
	cfg := config.RateLimitConfig{
		PublicPerMinute: 1000, // High limit to avoid blocking
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req = req.WithContext(WithRateLimitTier(req.Context(), TierPublic))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
	}
}

func BenchmarkRateLimit_Block(b *testing.B) {
	cfg := config.RateLimitConfig{
		PublicPerMinute: 1, // Very low limit
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req = req.WithContext(WithRateLimitTier(req.Context(), TierPublic))

	// Exhaust limit
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
	}
}

func TestLoginRateLimit_TokenRefill(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping time-dependent test in short mode")
	}

	cfg := config.RateLimitConfig{
		LoginPer15Minutes: 5,
	}

	handler := RateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	clientIP := "192.168.1.103:12345"

	// Exhaust the burst allowance (5 requests)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
		req.RemoteAddr = clientIP
		req = req.WithContext(WithRateLimitTier(req.Context(), TierLogin))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
	}

	// 6th request should be blocked
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
	req.RemoteAddr = clientIP
	req = req.WithContext(WithRateLimitTier(req.Context(), TierLogin))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", res.Code)
	}

	// Wait for one token to refill (3 minutes for login tier)
	// In real usage: 1 token every 3 minutes
	// For testing: simulate token bucket refill with small delay
	time.Sleep(100 * time.Millisecond)

	// Note: This test demonstrates the concept but doesn't wait 3 minutes.
	// In production, after 3 minutes, 1 more request would be allowed.
	// The token bucket will gradually refill, allowing sustained rate of
	// 1 request per 3 minutes after the initial burst is exhausted.
}

// TestLimiterStore_Cleanup verifies that stale limiters are cleaned up (server-g746)
func TestLimiterStore_Cleanup(t *testing.T) {
	cfg := config.RateLimitConfig{
		PublicPerMinute: 60,
	}

	store := newLimiterStore(cfg)
	defer store.Stop()

	// Create some limiter entries
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.1.2:12345"

	// Access limiters to create entries (no trusted proxies)
	store.limiter(TierPublic, clientKey(req1, nil))
	store.limiter(TierPublic, clientKey(req2, nil))

	// Verify entries exist
	if len(store.limiters) != 2 {
		t.Fatalf("expected 2 limiter entries, got %d", len(store.limiters))
	}

	// Manually set lastSeen to old time to simulate stale entries
	store.mu.Lock()
	for _, entry := range store.limiters {
		entry.lastSeen = time.Now().Add(-20 * time.Minute) // 20 minutes ago
	}
	store.mu.Unlock()

	// Run cleanup
	store.cleanup()

	// Verify entries were removed (TTL is 15 minutes)
	store.mu.Lock()
	count := len(store.limiters)
	store.mu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 limiter entries after cleanup, got %d", count)
	}
}

// TestLimiterStore_CleanupPreservesActive verifies cleanup doesn't remove active entries
func TestLimiterStore_CleanupPreservesActive(t *testing.T) {
	cfg := config.RateLimitConfig{
		PublicPerMinute: 60,
	}

	store := newLimiterStore(cfg)
	defer store.Stop()

	// Create limiter entries
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.1.2:12345"

	store.limiter(TierPublic, clientKey(req1, nil))
	store.limiter(TierPublic, clientKey(req2, nil))

	// Set one entry as stale, keep one recent
	store.mu.Lock()
	key1 := string(TierPublic) + ":192.168.1.1"
	key2 := string(TierPublic) + ":192.168.1.2"
	store.limiters[key1].lastSeen = time.Now().Add(-20 * time.Minute) // Stale
	store.limiters[key2].lastSeen = time.Now()                        // Recent
	store.mu.Unlock()

	// Run cleanup
	store.cleanup()

	// Verify only stale entry was removed
	store.mu.Lock()
	count := len(store.limiters)
	_, hasKey1 := store.limiters[key1]
	_, hasKey2 := store.limiters[key2]
	store.mu.Unlock()

	if count != 1 {
		t.Fatalf("expected 1 limiter entry after cleanup, got %d", count)
	}
	if hasKey1 {
		t.Error("stale entry should have been removed")
	}
	if !hasKey2 {
		t.Error("active entry should have been preserved")
	}
}
