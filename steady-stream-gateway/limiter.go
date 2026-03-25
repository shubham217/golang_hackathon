package main

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter defines the behavior of a single rate limiter.
type Limiter interface {
	Allow() bool
	LastAccessed() time.Time
}

// leakyBucket implements the Limiter interface using a time-based token refill.
// This is mathematically equivalent to a leaky bucket used for rate limiting.
type leakyBucket struct {
	capacity     float64
	refillRate   time.Duration // Time to add 1 token (leak interval)
	tokens       float64
	lastUpdate   time.Time
	lastAccessed time.Time
	mu           sync.Mutex
}

func (b *leakyBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.lastAccessed = now

	// If it's the first time, start with a full bucket
	if b.lastUpdate.IsZero() {
		b.tokens = b.capacity
	} else {
		// Calculate how many tokens "leaked" (refilled) since last check
		elapsed := now.Sub(b.lastUpdate)
		b.tokens += float64(elapsed) / float64(b.refillRate)
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
	}

	b.lastUpdate = now

	// If we have at least one token, allow the request and consume the token
	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	return false
}

func (b *leakyBucket) LastAccessed() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastAccessed
}

// IPRateLimiter manages a separate leaky bucket for each IP address.
type IPRateLimiter struct {
	ips      map[string]Limiter
	mu       sync.RWMutex
	capacity int
	rate     time.Duration
}

func NewIPRateLimiter(capacity int, rate time.Duration) *IPRateLimiter {
	return &IPRateLimiter{
		ips:      make(map[string]Limiter),
		capacity: capacity,
		rate:     rate,
	}
}

func (i *IPRateLimiter) GetLimiter(ip string) Limiter {
	// Optimistic Read Lock
	i.mu.RLock()
	limiter, exists := i.ips[ip]
	i.mu.RUnlock()

	if exists {
		return limiter
	}

	// Write Lock for creating a new bucket
	i.mu.Lock()
	defer i.mu.Unlock()

	// Double check pattern for thread safety
	if limiter, exists = i.ips[ip]; exists {
		return limiter
	}

	limiter = &leakyBucket{
		capacity:   float64(i.capacity),
		refillRate: i.rate,
	}
	i.ips[ip] = limiter
	return limiter
}

// RunBackgroundCleanup periodically removes stale IPs to prevent memory leaks.
func (i *IPRateLimiter) RunBackgroundCleanup(interval time.Duration, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			i.mu.Lock()
			now := time.Now()
			for ip, limiter := range i.ips {
				if now.Sub(limiter.LastAccessed()) > maxAge {
					delete(i.ips, ip)
				}
			}
			i.mu.Unlock()
		}
	}()
}

// RateLimitMiddleware wraps an HTTP handler with Per-IP rate limiting.
func RateLimitMiddleware(ipLimiter *IPRateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract IP address, stripping the port
		ip := r.RemoteAddr
		if pos := strings.LastIndex(ip, ":"); pos != -1 {
			ip = ip[:pos]
		}

		limiter := ipLimiter.GetLimiter(ip)
		if !limiter.Allow() {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("429 Too Many Requests: Rate limit exceeded\n"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
