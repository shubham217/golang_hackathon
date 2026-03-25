package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// FastService simulates a quick database read.
func FastService(w http.ResponseWriter, r *http.Request) {
	time.Sleep(10 * time.Millisecond)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Fast Service OK")
}

// SlowService simulates a heavy CPU-bound task.
func SlowService(w http.ResponseWriter, r *http.Request) {
	time.Sleep(500 * time.Millisecond)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Slow Service OK")
}

func main() {
	log.Println("🛠️ Initializing Steady Stream Gateway...")

	// Configure Limiters:
	// - Fast service: 10 burst capacity, refills 1 token every 200ms (5 req/sec)
	// - Slow service: 2 burst capacity, refills 1 token every 2s (0.5 req/sec)
	fastLimiter := NewShardedIPRateLimiter(64, 10, 200*time.Millisecond)
	slowLimiter := NewShardedIPRateLimiter(64, 2, 2*time.Second)

	// Start memory management (Background Cleanup):
	// Scans every 1 minute, removes IPs inactive for 5 minutes.
	log.Println("🧹 Starting background cleanup routines...")
	fastLimiter.RunBackgroundCleanup(1*time.Minute, 5*time.Minute)
	slowLimiter.RunBackgroundCleanup(1*time.Minute, 5*time.Minute)

	mux := http.NewServeMux()

	// Apply Rate Limit Middleware to routes
	mux.Handle("/api/fast", RateLimitMiddleware(fastLimiter, http.HandlerFunc(FastService)))
	mux.Handle("/api/slow", RateLimitMiddleware(slowLimiter, http.HandlerFunc(SlowService)))

	// Readiness check
	log.Println("🚀 Steady Stream Gateway is ready on :8080")
	log.Println("   Routes: /api/fast (5 req/s), /api/slow (0.5 req/s)")
	
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("❌ Server crashed: %v", err)
	}
}
