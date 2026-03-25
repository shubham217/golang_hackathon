package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestIPRateLimiter_Isolation(t *testing.T) {
	manager := NewShardedIPRateLimiter(4, 2, 100*time.Millisecond)
	ip1, ip2 := "192.168.1.1", "10.0.0.1"

	limiter1 := manager.GetLimiter(ip1)
	limiter1.Allow()
	limiter1.Allow()
	if allowed, _, _ := limiter1.Allow(); allowed {
		t.Errorf("IP1 should be rate limited")
	}

	limiter2 := manager.GetLimiter(ip2)
	if allowed, _, _ := limiter2.Allow(); !allowed {
		t.Errorf("IP2 should have an empty bucket and be allowed")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	manager := NewShardedIPRateLimiter(4, 1, 10*time.Second)
	handler := RateLimitMiddleware(manager, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %v", rr.Code)
	}

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429 Too Many Requests, got %v", rr2.Code)
	}
}

func TestIPRateLimiter_Concurrency(t *testing.T) {
	manager := NewShardedIPRateLimiter(16, 500, time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			limiter := manager.GetLimiter(ip)
			limiter.Allow()
		}("192.168.1." + string(rune(i)))
	}
	wg.Wait()
}
