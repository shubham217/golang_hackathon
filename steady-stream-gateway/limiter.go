package main

import (
	"fmt"
	"hash/fnv"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Limiter defines the behavior of a single rate limiter bucket.
type Limiter interface {
	// Allow checks if the request is permitted. Returns (allowed, remaining, retryAfter)
	Allow() (bool, int, time.Duration)
	// LastAccessed returns the timestamp of the last request for this bucket.
	LastAccessed() time.Time
}

// AtomicLeakyBucket implements the Generic Cell Rate Algorithm (GCRA).
// It tracks Theoretical Arrival Time (TAT) to enforce rates without timers or tickers.
type AtomicLeakyBucket struct {
	nextFreeTick int64 // Theoretical Arrival Time (TAT) in nanoseconds
	interval     int64 // Time increment per request (1/rate)
	maxBurst     int64 // Burst tolerance (capacity * interval)
	lastAccessed int64 // UnixNano timestamp for cleanup tracking
}

// NewAtomicBucket initializes a new lock-free bucket.
func NewAtomicBucket(capacity int, rate time.Duration) *AtomicLeakyBucket {
	interval := int64(rate)
	// GCRA: maxBurst = (capacity-1) * interval. 
	// This represents the maximum "debt" we can take from the future.
	burst := int64(capacity-1) * interval
	if burst < 0 {
		burst = 0
	}
	return &AtomicLeakyBucket{
		interval:     interval,
		maxBurst:     burst,
		nextFreeTick: time.Now().UnixNano(),
		lastAccessed: time.Now().UnixNano(),
	}
}

// Allow is the core of the GCRA algorithm. It is 100% lock-free using Atomic CAS.
func (b *AtomicLeakyBucket) Allow() (bool, int, time.Duration) {
	now := time.Now().UnixNano()
	atomic.StoreInt64(&b.lastAccessed, now) // Update for cleanup tracker

	for {
		next := atomic.LoadInt64(&b.nextFreeTick)

		// Calculate the arrival time we want to assign to this request.
		// If 'now' is later than 'next', the bucket is empty; we start from 'now'.
		targetTime := next
		if now > next {
			targetTime = now
		}

		// Check if the target time is too far in the future (Limit Exceeded).
		diff := targetTime - now
		if diff > b.maxBurst {
			// Calculate wait time until the next token is ready.
			wait := time.Duration(diff - b.maxBurst)
			return false, 0, wait
		}

		// Attempt to "claim" the time slot atomically.
		newNext := targetTime + b.interval
		if atomic.CompareAndSwapInt64(&b.nextFreeTick, next, newNext) {
			// Calculate remaining tokens based on current time debt.
			remaining := int((b.maxBurst - diff) / b.interval)
			return true, remaining, 0
		}
		// If CAS failed, another request won the race. Loop and try again.
	}
}

func (b *AtomicLeakyBucket) LastAccessed() time.Time {
	return time.Unix(0, atomic.LoadInt64(&b.lastAccessed))
}

// ShardedIPRateLimiter implements "Lock Striping" to minimize mutex contention.
type ShardedIPRateLimiter struct {
	shards    []*shard
	numShards int
	capacity  int
	rate      time.Duration
}

type shard struct {
	ips map[string]Limiter
	mu  sync.RWMutex
}

func NewShardedIPRateLimiter(numShards int, capacity int, rate time.Duration) *ShardedIPRateLimiter {
	s := &ShardedIPRateLimiter{
		shards:    make([]*shard, numShards),
		numShards: numShards,
		capacity:  capacity,
		rate:      rate,
	}
	for i := 0; i < numShards; i++ {
		s.shards[i] = &shard{ips: make(map[string]Limiter)}
	}
	return s
}

// getShard determines which lock shard an IP belongs to using FNV-1a hashing.
func (s *ShardedIPRateLimiter) getShard(ip string) *shard {
	h := fnv.New32a()
	h.Write([]byte(ip))
	return s.shards[uint32(h.Sum32())%uint32(s.numShards)]
}

func (s *ShardedIPRateLimiter) GetLimiter(ip string) Limiter {
	shard := s.getShard(ip)
	
	shard.mu.RLock()
	limiter, exists := shard.ips[ip]
	shard.mu.RUnlock()

	if exists {
		return limiter
	}

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Double-check pattern for thread safety.
	if limiter, exists = shard.ips[ip]; exists {
		return limiter
	}

	// DEBUG: Track memory growth.
	log.Printf("[DEBUG] Creating new bucket for IP: %s", ip)

	limiter = NewAtomicBucket(s.capacity, s.rate)
	shard.ips[ip] = limiter
	return limiter
}

// RunBackgroundCleanup sweeps the shards to remove inactive IPs.
func (s *ShardedIPRateLimiter) RunBackgroundCleanup(interval time.Duration, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			purgedTotal := 0
			for i := 0; i < s.numShards; i++ {
				shard := s.shards[i]
				shard.mu.Lock()
				now := time.Now()
				for ip, limiter := range shard.ips {
					if now.Sub(limiter.LastAccessed()) > maxAge {
						delete(shard.ips, ip)
						purgedTotal++
					}
				}
				shard.mu.Unlock()
				// Prevent CPU spikes by yielding during large cleanup cycles.
				time.Sleep(time.Millisecond * 2)
			}
			if purgedTotal > 0 {
				log.Printf("[DEBUG] Cleanup Cycle: Purged %d stale IP buckets", purgedTotal)
			}
		}
	}()
}

// GetClientIP handles extraction of the real user IP, prioritizing X-Forwarded-For.
func GetClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// RateLimitMiddleware intercepts requests to apply the rate limiting logic.
func RateLimitMiddleware(ipLimiter *ShardedIPRateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := GetClientIP(r)
		limiter := ipLimiter.GetLimiter(ip)
		
		allowed, remaining, retryAfter := limiter.Allow()
		
		// Set informational headers.
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", ipLimiter.capacity))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		if !allowed {
			// DEBUG: Monitor rate-limit events.
			log.Printf("[LIMIT] IP: %s | Route: %s | Retry-After: %ds", ip, r.URL.Path, int(retryAfter.Seconds()))
			
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
