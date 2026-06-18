package serverutil

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"suppa-ahg-stack/common-golang/logger"
)

type rateEntry struct {
	count       int
	windowStart time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateEntry
	limit   int
	window  time.Duration
	stopCh  chan struct{}
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*rateEntry),
		limit:   limit,
		window:  window,
		stopCh:  make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, exists := rl.entries[key]

	if !exists || now.Sub(entry.windowStart) > rl.window {
		rl.entries[key] = &rateEntry{count: 1, windowStart: now}
		return true
	}

	if entry.count >= rl.limit {
		return false
	}

	entry.count++
	return true
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window * 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-rl.window)
			for key, entry := range rl.entries {
				if entry.windowStart.Before(cutoff) {
					delete(rl.entries, key)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// ClientIP returns the host portion of r.RemoteAddr, stripping the ephemeral
// source port. If RemoteAddr cannot be parsed, the raw value is returned.
func ClientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func RateLimitMiddleware(next http.Handler, rl *RateLimiter, sessionName string, l *logger.FileLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r)
		ua := r.UserAgent()

		sessionID := ""
		if cookie, err := r.Cookie(sessionName); err == nil {
			sessionID = cookie.Value
		}

		key := fmt.Sprintf("%s|%s|%s", ip, sessionID, ua)
		if !rl.Allow(key) {
			l.Warn(fmt.Sprintf("Rate limit exceeded ip=%s path=%s", ip, r.URL.Path))
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
