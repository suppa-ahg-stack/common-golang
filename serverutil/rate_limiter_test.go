package serverutil

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"suppa-ahg-stack/common-golang/logger"
)

func newTestLogger(t *testing.T) *logger.FileLogger {
	t.Helper()
	l, err := logger.NewFileLogger(logger.LogConfig{
		Filename: filepath.Join(t.TempDir(), "test.log"),
		Level:    0,
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func TestClientIP_StripsPort(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"IPv4 with port", "192.0.2.1:12345", "192.0.2.1"},
		{"IPv6 with port", "[::1]:12345", "::1"},
		{"no port falls back", "192.0.2.1", "192.0.2.1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if got := ClientIP(req); got != tc.want {
				t.Fatalf("ClientIP(%q) = %q, want %q", tc.remoteAddr, got, tc.want)
			}
		})
	}
}

func TestRateLimitMiddleware_GroupsByIPIgnoringPort(t *testing.T) {
	// Allow 2 requests per hour.
	rl := NewRateLimiter(2, time.Hour)
	defer rl.Stop()

	l := newTestLogger(t)
	handler := RateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), rl, "session", l)

	// First connection from 192.0.2.1:1000.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "192.0.2.1:1000"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request rejected: %d", rr1.Code)
	}

	// Second connection from same IP, different source port.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.0.2.1:1001"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second request from same IP (different port) rejected: %d", rr2.Code)
	}

	// Third connection from same IP should be rate limited.
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "192.0.2.1:1002"
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusTooManyRequests {
		t.Fatalf("third request from same IP should be rate limited, got: %d", rr3.Code)
	}
}
