package web

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type tokenBucket struct {
	mu       sync.Mutex
	tokens   int
	max      int
	lastFill time.Time
}

func newBucket(max int) *tokenBucket {
	return &tokenBucket{tokens: max, max: max, lastFill: time.Now()}
}

func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Since(b.lastFill) > time.Minute {
		b.tokens = b.max
		b.lastFill = time.Now()
	}
	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}

var loginBuckets sync.Map
var webhookBuckets sync.Map

// RateLimitWebhook limits webhook requests per IP.
func RateLimitWebhook(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/wh/tg/") {
			next.ServeHTTP(w, r)
			return
		}
		ip := r.RemoteAddr
		v, _ := webhookBuckets.LoadOrStore(ip, newBucket(120))
		if !v.(*tokenBucket).allow() {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimitLogin middleware limits login attempts per IP.
func RateLimitLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" || r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		ip := r.RemoteAddr
		v, _ := loginBuckets.LoadOrStore(ip, newBucket(10))
		if !v.(*tokenBucket).allow() {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
