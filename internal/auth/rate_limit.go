package auth

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

type RateLimiter struct {
	Limit      int
	Window     time.Duration
	TrustProxy bool

	mu      sync.Mutex
	clients map[string]rateBucket
}

type rateBucket struct {
	Count int
	Reset time.Time
}

const maxRateLimitClients = 4096

func NewRateLimiter(limit int, window time.Duration, trustProxy bool) *RateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{Limit: limit, Window: window, TrustProxy: trustProxy, clients: map[string]rateBucket{}}
}

func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	if l == nil || l.Limit <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, remaining, reset := l.check(r)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(l.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(l.Window.Seconds())))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) check(r *http.Request) (bool, int, time.Time) {
	ip := ClientIP(r, l.TrustProxy)
	key := "unknown"
	if ip != nil {
		key = ip.String()
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.clients[key]; !ok && len(l.clients) >= maxRateLimitClients {
		l.pruneLocked(now)
		if len(l.clients) >= maxRateLimitClients {
			l.dropOldestLocked()
		}
	}
	bucket := l.clients[key]
	if bucket.Reset.IsZero() || now.After(bucket.Reset) {
		bucket = rateBucket{Reset: now.Add(l.Window)}
	}
	bucket.Count++
	l.clients[key] = bucket
	remaining := l.Limit - bucket.Count
	if remaining < 0 {
		remaining = 0
	}
	return bucket.Count <= l.Limit, remaining, bucket.Reset
}

func (l *RateLimiter) pruneLocked(now time.Time) {
	for key, bucket := range l.clients {
		if now.After(bucket.Reset) {
			delete(l.clients, key)
		}
	}
}

func (l *RateLimiter) dropOldestLocked() {
	var oldestKey string
	var oldestReset time.Time
	for key, bucket := range l.clients {
		if oldestKey == "" || bucket.Reset.Before(oldestReset) {
			oldestKey = key
			oldestReset = bucket.Reset
		}
	}
	if oldestKey != "" {
		delete(l.clients, oldestKey)
	}
}
