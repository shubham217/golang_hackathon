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
	// 64 shards is a good default for high-concurrency
	fastLimiter := NewShardedIPRateLimiter(64, 10, 200*time.Millisecond)
	slowLimiter := NewShardedIPRateLimiter(64, 2, 2*time.Second)

	fastLimiter.RunBackgroundCleanup(1*time.Minute, 5*time.Minute)
	slowLimiter.RunBackgroundCleanup(1*time.Minute, 5*time.Minute)

	mux := http.NewServeMux()

	mux.Handle("/api/fast", RateLimitMiddleware(fastLimiter, http.HandlerFunc(FastService)))
	mux.Handle("/api/slow", RateLimitMiddleware(slowLimiter, http.HandlerFunc(SlowService)))

	fmt.Println("🚀 Optimized Steady Stream Gateway starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server crashed: %v", err)
	}
}
