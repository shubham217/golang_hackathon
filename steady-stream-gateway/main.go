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
	// TODO: Initialize your IPRateLimiters here
	mux := http.NewServeMux()

	// TODO: Wrap these handlers in your RateLimitMiddleware
	mux.Handle("/api/fast", http.HandlerFunc(FastService))
	mux.Handle("/api/slow", http.HandlerFunc(SlowService))

	fmt.Println("🚀 API Gateway starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server crashed: %v", err)
	}
}
