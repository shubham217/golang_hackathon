package main

import (
	"net/http"
	"time"
)

// Limiter defines the behavior of a single rate limiter.
type Limiter interface {
	Allow() bool
	LastAccessed() time.Time
}

// IPRateLimiter manages a separate leaky bucket for each IP address.
type IPRateLimiter struct {
	// TODO: Add map for tracking IPs, a mutex for safety, and capacity/rate config
}

func NewIPRateLimiter(capacity int, rate time.Duration) *IPRateLimiter {
	return &IPRateLimiter{}
}

func (i *IPRateLimiter) GetLimiter(ip string) Limiter {
	// TODO: Retrieve the bucket for this IP, or create a new one if it doesn't exist.
	return nil
}

// RunBackgroundCleanup periodically removes stale IPs to prevent memory leaks.
func (i *IPRateLimiter) RunBackgroundCleanup(interval time.Duration, maxAge time.Duration) {
	// TODO: Implement a background goroutine that sweeps the map for old buckets.
}

// RateLimitMiddleware wraps an HTTP handler with Per-IP rate limiting.
func RateLimitMiddleware(ipLimiter *IPRateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Extract IP, check limit, return 429 if full, or call next.ServeHTTP
		next.ServeHTTP(w, r)
	})
}
