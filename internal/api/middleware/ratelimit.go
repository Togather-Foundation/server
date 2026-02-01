package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"golang.org/x/time/rate"
)

type RateLimitTier string

const (
	TierPublic     RateLimitTier = "public"
	TierAgent      RateLimitTier = "agent"
	TierAdmin      RateLimitTier = "admin"
	TierLogin      RateLimitTier = "login"      // Aggressive rate limiting for login attempts
	TierFederation RateLimitTier = "federation" // Federation sync endpoints
)

type rateLimitKey string

const rateLimitTierKey rateLimitKey = "rateLimitTier"

func WithRateLimitTier(ctx context.Context, tier RateLimitTier) context.Context {
	return context.WithValue(ctx, rateLimitTierKey, tier)
}

func WithRateLimitTierHandler(tier RateLimitTier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithRateLimitTier(r.Context(), tier)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RateLimit(cfg config.RateLimitConfig) func(http.Handler) http.Handler {
	store := newLimiterStore(cfg)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}

			tier := TierPublic
			if value, ok := r.Context().Value(rateLimitTierKey).(RateLimitTier); ok {
				tier = value
			}

			limiter := store.limiter(tier, clientKey(r, cfg.TrustedProxyCIDRs))
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}

			if !limiter.Allow() {
				// Set appropriate Retry-After header based on tier
				retryAfter := "60" // Default: 1 minute
				if tier == TierLogin {
					retryAfter = "180" // Login: 3 minutes (token refill rate)
				}
				w.Header().Set("Retry-After", retryAfter)
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type limiterStore struct {
	mu          sync.Mutex
	limiters    map[string]*limiterEntry
	perMinute   map[RateLimitTier]int
	stopCleanup chan struct{}
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newLimiterStore(cfg config.RateLimitConfig) *limiterStore {
	store := &limiterStore{
		limiters: make(map[string]*limiterEntry),
		perMinute: map[RateLimitTier]int{
			TierPublic:     cfg.PublicPerMinute,
			TierAgent:      cfg.AgentPerMinute,
			TierAdmin:      cfg.AdminPerMinute,
			TierLogin:      cfg.LoginPer15Minutes, // Special handling below
			TierFederation: cfg.FederationPerMinute,
		},
		stopCleanup: make(chan struct{}),
	}

	// Start background cleanup goroutine
	// Removes entries not accessed in 15 minutes to prevent unbounded memory growth
	go store.cleanupLoop()

	return store
}

func (s *limiterStore) limiter(tier RateLimitTier, key string) *rate.Limiter {
	limit := s.perMinute[tier]
	if limit <= 0 {
		return nil
	}

	lookup := string(tier) + ":" + key
	if key == "" {
		lookup = string(tier)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.limiters[lookup]; ok {
		// Update last seen time for TTL-based cleanup
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	// Special handling for login tier: 5 attempts per 15 minutes
	// Uses token bucket: rate = 5/(15 min) = 1 token every 3 minutes
	var limiter *rate.Limiter
	if tier == TierLogin {
		// Allow burst of 5 requests, refill at 1 token per 3 minutes
		limiter = rate.NewLimiter(rate.Every(3*time.Minute), limit)
	} else {
		// Standard per-minute rate limiting for other tiers
		interval := time.Minute / time.Duration(limit)
		limiter = rate.NewLimiter(rate.Every(interval), limit)
	}

	s.limiters[lookup] = &limiterEntry{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

// cleanupLoop runs a background goroutine that periodically removes stale limiter entries
// to prevent unbounded memory growth under attack scenarios (server-g746)
func (s *limiterStore) cleanupLoop() {
	// Cleanup every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCleanup:
			return
		}
	}
}

// cleanup removes limiter entries that haven't been accessed in 15 minutes
func (s *limiterStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	ttl := 15 * time.Minute

	for key, entry := range s.limiters {
		if now.Sub(entry.lastSeen) > ttl {
			delete(s.limiters, key)
		}
	}
}

// Stop gracefully shuts down the cleanup goroutine
func (s *limiterStore) Stop() {
	close(s.stopCleanup)
}

// clientKey extracts the client identifier for rate limiting, with protection against
// X-Forwarded-For spoofing by only trusting the header from configured proxy CIDRs (server-chgh)
func clientKey(r *http.Request, trustedProxyCIDRs []string) string {
	if r == nil {
		return ""
	}

	// Get the immediate connection's remote address
	remoteIP := ""
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remoteIP = host
	} else {
		remoteIP = r.RemoteAddr
	}

	// Only trust X-Forwarded-For if the request comes from a trusted proxy
	if isTrustedProxy(remoteIP, trustedProxyCIDRs) {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			return strings.TrimSpace(realIP)
		}
	}

	// Fallback to direct connection IP (not trusting headers from untrusted sources)
	return remoteIP
}

// isTrustedProxy checks if the given IP is within any of the trusted proxy CIDRs
func isTrustedProxy(ip string, trustedCIDRs []string) bool {
	if len(trustedCIDRs) == 0 {
		// No trusted proxies configured - don't trust any headers
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, cidrStr := range trustedCIDRs {
		_, cidr, err := net.ParseCIDR(cidrStr)
		if err != nil {
			continue
		}
		if cidr.Contains(parsedIP) {
			return true
		}
	}

	return false
}
