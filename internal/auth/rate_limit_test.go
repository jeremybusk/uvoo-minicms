package auth

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestRateLimiterLimitsPerClientIP(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute, false)
	called := 0
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/admin/", nil)
		req.RemoteAddr = "203.0.113.10:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("request %d: expected 204, got %d", i+1, rec.Code)
		}
		if rec.Header().Get("X-RateLimit-Limit") != "2" {
			t.Fatalf("request %d: expected rate limit header, got %#v", i+1, rec.Header())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Remaining") != "0" || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("expected exhausted rate limit headers, got %#v", rec.Header())
	}
	if called != 2 {
		t.Fatalf("expected downstream handler to be called twice, got %d", called)
	}
}

func TestRateLimiterUsesTrustedProxyClientIP(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute, true)
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, ip := range []string{"203.0.113.10", "203.0.113.11"} {
		req := httptest.NewRequest(http.MethodPost, "/admin/", nil)
		req.RemoteAddr = "10.0.0.2:12345"
		req.Header.Set("X-Forwarded-For", ip)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected distinct forwarded IP %s to be allowed, got %d", ip, rec.Code)
		}
	}
}

func TestRateLimiterDisabledForZeroOrNegativeLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		limiter := NewRateLimiter(limit, time.Minute, false)
		called := 0
		handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called++
			w.WriteHeader(http.StatusNoContent)
		}))
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodPost, "/admin/", nil)
			req.RemoteAddr = "203.0.113.10:12345"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("limit %d request %d: expected 204, got %d", limit, i+1, rec.Code)
			}
		}
		if called != 3 {
			t.Fatalf("limit %d: expected downstream handler to be called 3 times, got %d", limit, called)
		}
	}
}

func TestRateLimiterCapsClientBuckets(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute, false)
	now := time.Now()
	for i := 0; i < maxRateLimitClients; i++ {
		limiter.clients[strconv.Itoa(i)] = rateBucket{Count: 1, Reset: now.Add(time.Duration(i+1) * time.Minute)}
	}
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/admin/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if len(limiter.clients) != maxRateLimitClients {
		t.Fatalf("expected client bucket count to stay capped at %d, got %d", maxRateLimitClients, len(limiter.clients))
	}
	if _, ok := limiter.clients["0"]; ok {
		t.Fatal("expected oldest bucket to be evicted")
	}
}
