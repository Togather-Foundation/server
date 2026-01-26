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
	TierPublic RateLimitTier = "public"
	TierAgent  RateLimitTier = "agent"
	TierAdmin  RateLimitTier = "admin"
	TierLogin  RateLimitTier = "login" // Aggressive rate limiting for login attempts
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

			limiter := store.limiter(tier, clientKey(r))
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
	mu        sync.Mutex
	limiters  map[string]*rate.Limiter
	perMinute map[RateLimitTier]int
}

func newLimiterStore(cfg config.RateLimitConfig) *limiterStore {
	return &limiterStore{
		limiters: make(map[string]*rate.Limiter),
		perMinute: map[RateLimitTier]int{
			TierPublic: cfg.PublicPerMinute,
			TierAgent:  cfg.AgentPerMinute,
			TierAdmin:  cfg.AdminPerMinute,
			TierLogin:  cfg.LoginPer15Minutes, // Special handling below
		},
	}
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

	if limiter, ok := s.limiters[lookup]; ok {
		return limiter
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

	s.limiters[lookup] = limiter
	return limiter
}

func clientKey(r *http.Request) string {
	if r == nil {
		return ""
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
