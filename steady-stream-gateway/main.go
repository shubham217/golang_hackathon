package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func FastService(w http.ResponseWriter, r *http.Request) {
	time.Sleep(10 * time.Millisecond)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Fast Service OK")
}

func SlowService(w http.ResponseWriter, r *http.Request) {
	time.Sleep(500 * time.Millisecond)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Slow Service OK")
}

func main() {
	// 1. Initialize IPRateLimiters
	// Fast service: 10 requests burst, 5 req/sec (200ms leak)
	fastLimiter := NewIPRateLimiter(10, 200*time.Millisecond)
	// Slow service: 2 requests burst, 0.5 req/sec (2s leak)
	slowLimiter := NewIPRateLimiter(2, 2*time.Second)

	// 2. Start background cleanup (Check every 1m, delete after 5m inactivity)
	fastLimiter.RunBackgroundCleanup(1*time.Minute, 5*time.Minute)
	slowLimiter.RunBackgroundCleanup(1*time.Minute, 5*time.Minute)

	mux := http.NewServeMux()

	// 3. Wrap handlers in Per-IP RateLimitMiddleware
	mux.Handle("/api/fast", RateLimitMiddleware(fastLimiter, http.HandlerFunc(FastService)))
	mux.Handle("/api/slow", RateLimitMiddleware(slowLimiter, http.HandlerFunc(SlowService)))

	fmt.Println("🚀 Steady Stream Gateway starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server crashed: %v", err)
	}
}
