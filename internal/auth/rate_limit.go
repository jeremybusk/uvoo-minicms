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
		if !l.allow(r) {
			w.Header().Set("Retry-After", strconv.Itoa(int(l.Window.Seconds())))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) allow(r *http.Request) bool {
	ip := ClientIP(r, l.TrustProxy)
	key := "unknown"
	if ip != nil {
		key = ip.String()
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.clients[key]
	if bucket.Reset.IsZero() || now.After(bucket.Reset) {
		bucket = rateBucket{Reset: now.Add(l.Window)}
	}
	bucket.Count++
	l.clients[key] = bucket
	if len(l.clients) > 4096 {
		l.pruneLocked(now)
	}
	return bucket.Count <= l.Limit
}

func (l *RateLimiter) pruneLocked(now time.Time) {
	for key, bucket := range l.clients {
		if now.After(bucket.Reset) {
			delete(l.clients, key)
		}
	}
}
