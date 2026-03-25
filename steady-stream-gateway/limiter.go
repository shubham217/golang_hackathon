package main

import (
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Limiter defines the behavior of a single rate limiter.
type Limiter interface {
	Allow() (bool, int, time.Duration)
	LastAccessed() time.Time
}

// AtomicLeakyBucket implements the Generic Cell Rate Algorithm (GCRA).
// It is completely lock-free for the Allow() operation.
type AtomicLeakyBucket struct {
	nextFreeTick int64 // Nanoseconds (UnixNano)
	interval     int64 // Nanoseconds per request
	maxBurst     int64 // Max nanoseconds we can "borrow" from the future
	lastAccessed int64 // UnixNano for cleanup tracking
}

func NewAtomicBucket(capacity int, rate time.Duration) *AtomicLeakyBucket {
	interval := int64(rate)
	// GCRA: tau (maxBurst) = (limit - 1) * interval
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

func (b *AtomicLeakyBucket) Allow() (bool, int, time.Duration) {
	now := time.Now().UnixNano()
	atomic.StoreInt64(&b.lastAccessed, now)

	for {
		next := atomic.LoadInt64(&b.nextFreeTick)

		// Calculate when this request would theoretically be allowed
		targetTime := next
		if now > next {
			targetTime = now
		}

		// If the target time is too far in the future, the "bucket" is full
		diff := targetTime - now
		if diff > b.maxBurst {
			wait := time.Duration(diff - b.maxBurst)
			return false, 0, wait
		}

		// Attempt to update. If next has changed since we loaded it, loop and try again.
		newNext := targetTime + b.interval
		if atomic.CompareAndSwapInt64(&b.nextFreeTick, next, newNext) {
			remaining := int((b.maxBurst - diff) / b.interval)
			return true, remaining, 0
		}
	}
}

func (b *AtomicLeakyBucket) LastAccessed() time.Time {
	return time.Unix(0, atomic.LoadInt64(&b.lastAccessed))
}

// ShardedIPRateLimiter manages shards of atomic buckets.
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

	if limiter, exists = shard.ips[ip]; exists {
		return limiter
	}

	limiter = NewAtomicBucket(s.capacity, s.rate)
	shard.ips[ip] = limiter
	return limiter
}

func (s *ShardedIPRateLimiter) RunBackgroundCleanup(interval time.Duration, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			for i := 0; i < s.numShards; i++ {
				shard := s.shards[i]
				shard.mu.Lock()
				now := time.Now()
				for ip, limiter := range shard.ips {
					if now.Sub(limiter.LastAccessed()) > maxAge {
						delete(shard.ips, ip)
					}
				}
				shard.mu.Unlock()
				time.Sleep(time.Millisecond * 2)
			}
		}
	}()
}

func GetClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

func RateLimitMiddleware(ipLimiter *ShardedIPRateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := GetClientIP(r)
		limiter := ipLimiter.GetLimiter(ip)
		
		allowed, remaining, retryAfter := limiter.Allow()
		
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", ipLimiter.capacity))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
