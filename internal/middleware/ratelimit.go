// internal/middleware/ratelimit.go
package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

// bucket is a token bucket for a single IP address.
type bucket struct {
	tokens     float64   // current token count — float so refill math is precise
	lastRefill time.Time // when we last added tokens
	mu         sync.Mutex
}

// rateLimiter holds one bucket per IP and the global config.
type rateLimiter struct {
	buckets     map[string]*bucket
	mu          sync.RWMutex
	maxTokens   float64       // capacity — matches RATE_LIMIT_REQUESTS
	refillRate  float64       // tokens per second
	cleanupTick *time.Ticker
}

// newRateLimiter constructs the limiter from config values.
// It also starts a background goroutine that prunes stale IP buckets
// every 5 minutes to prevent unbounded memory growth.
func newRateLimiter(cfg *config.Config) *rateLimiter {
	max := float64(cfg.RateLimitRequests)
	windowSecs := cfg.RateLimitWindow.Seconds()

	rl := &rateLimiter{
		buckets:     make(map[string]*bucket),
		maxTokens:   max,
		refillRate:  max / windowSecs, // e.g. 60 tokens / 60s = 1 token/sec
		cleanupTick: time.NewTicker(5 * time.Minute),
	}

	go rl.cleanup()
	return rl
}

// cleanup removes buckets that haven't been touched in over 10 minutes.
// Runs in a background goroutine for the server's lifetime.
func (rl *rateLimiter) cleanup() {
	for range rl.cleanupTick.C {
		rl.mu.Lock()
		for ip, b := range rl.buckets {
			b.mu.Lock()
			idle := time.Since(b.lastRefill)
			b.mu.Unlock()
			if idle > 10*time.Minute {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// allow returns true if the IP has tokens remaining, false if rate limited.
// It also returns how many seconds until a token is available (for Retry-After).
func (rl *rateLimiter) allow(ip string) (bool, int) {
	// Fast path: get existing bucket with read lock.
	rl.mu.RLock()
	b, ok := rl.buckets[ip]
	rl.mu.RUnlock()

	if !ok {
		// Slow path: create a new full bucket for this IP.
		rl.mu.Lock()
		// Double-check after acquiring write lock.
		if b, ok = rl.buckets[ip]; !ok {
			b = &bucket{
				tokens:     rl.maxTokens,
				lastRefill: time.Now(),
			}
			rl.buckets[ip] = b
		}
		rl.mu.Unlock()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Refill: calculate how many tokens to add since last refill.
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * rl.refillRate
	if b.tokens > rl.maxTokens {
		b.tokens = rl.maxTokens // cap at maximum
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens--
		return true, 0
	}

	// No tokens — calculate seconds until next token arrives.
	retryAfter := int((1.0-b.tokens)/rl.refillRate) + 1
	return false, retryAfter
}

// extractIP pulls the real client IP from X-Forwarded-For (set by proxies
// like Fly.io's load balancer) or falls back to RemoteAddr.
func extractIP(r *http.Request) string {
	// X-Forwarded-For can be a comma-separated list — take the first entry.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// RemoteAddr is "ip:port" — strip the port.
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// RateLimit returns a Gin middleware that enforces per-IP token bucket limiting.
// Call once at startup — the returned HandlerFunc is safe for concurrent use.
func RateLimit(cfg *config.Config) gin.HandlerFunc {
	rl := newRateLimiter(cfg)

	return func(c *gin.Context) {
		ip := extractIP(c.Request)

		allowed, retryAfter := rl.allow(ip)
		if !allowed {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			utils.ErrorResponse(c, http.StatusTooManyRequests, "RATE_LIMITED",
				"too many requests — slow down")
			c.Abort()
			return
		}

		c.Next()
	}
}