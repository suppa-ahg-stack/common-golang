package internalapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientRetry503ThenSuccess(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "key", "test", WithRetryPolicy(RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
	}))

	ctx := WithRequestID(context.Background(), "req-retry")
	req, err := client.NewRequest(ctx, http.MethodGet, "/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	var dst struct{ Status string }
	if err := client.DoJSON(req, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.Status != "ok" {
		t.Errorf("expected ok, got %s", dst.Status)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestHTTPClient503NeverRecovers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "key", "test", WithRetryPolicy(RetryPolicy{
		MaxRetries: 2,
		BaseDelay:  5 * time.Millisecond,
		MaxDelay:   20 * time.Millisecond,
	}))

	req, err := client.NewRequest(context.Background(), http.MethodGet, "/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	err = client.DoJSON(req, nil)
	if !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("expected ErrDependencyUnavailable, got %v", err)
	}
}

func TestHTTPClientOversizedResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		large := bytes.Repeat([]byte("a"), MaxResponseBodyBytes+1024)
		_, _ = fmt.Fprintf(w, `{"payload":"%s"}`, large)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "key", "test", WithRetryPolicy(RetryPolicy{
		MaxRetries: 0,
	}))

	req, err := client.NewRequest(context.Background(), http.MethodGet, "/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	var dst struct{ Payload string }
	err = client.DoJSON(req, &dst)
	if err == nil {
		t.Fatal("expected error for oversized response body")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("expected decode response error, got %v", err)
	}
}

func TestHTTPClientNonJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "key", "test", WithRetryPolicy(RetryPolicy{
		MaxRetries: 0,
	}))

	req, err := client.NewRequest(context.Background(), http.MethodGet, "/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	var dst struct{}
	err = client.DoJSON(req, &dst)
	if err == nil {
		t.Fatal("expected error for non-JSON response")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("expected decode response error, got %v", err)
	}
}

func TestHTTPClientPost503NoRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "key", "test", WithRetryPolicy(RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  200 * time.Millisecond,
		MaxDelay:   500 * time.Millisecond,
	}))

	req, err := client.NewRequest(context.Background(), http.MethodPost, "/resource", map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	start := time.Now()
	err = client.DoJSON(req, nil)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("expected ErrDependencyUnavailable, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt for POST, got %d", attempts)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("POST 503 should not retry, took %v", elapsed)
	}
}

func TestHTTPClientPatch503NoRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "key", "test", WithRetryPolicy(RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  200 * time.Millisecond,
		MaxDelay:   500 * time.Millisecond,
	}))

	req, err := client.NewRequest(context.Background(), http.MethodPatch, "/resource", map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	start := time.Now()
	err = client.DoJSON(req, nil)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("expected ErrDependencyUnavailable, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt for PATCH, got %d", attempts)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("PATCH 503 should not retry, took %v", elapsed)
	}
}

func TestHTTPClientAuthErrorsNotDependencyUnavailable(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"unauthorized", http.StatusUnauthorized},
		{"forbidden", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"error":"auth"}`))
			}))
			defer server.Close()

			client := NewHTTPClient(server.URL, "key", "test", WithRetryPolicy(RetryPolicy{
				MaxRetries: 3,
				BaseDelay:  50 * time.Millisecond,
				MaxDelay:   100 * time.Millisecond,
			}))

			req, err := client.NewRequest(context.Background(), http.MethodGet, "/resource", nil)
			if err != nil {
				t.Fatalf("NewRequest failed: %v", err)
			}

			err = client.DoJSON(req, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if errors.Is(err, ErrDependencyUnavailable) {
				t.Errorf("expected error not to be ErrDependencyUnavailable, got %v", err)
			}
		})
	}
}

func TestHTTPClientRequestIDHeader(t *testing.T) {
	var receivedID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedID = r.Header.Get(RequestIDHeader)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "key", "test")
	ctx := WithRequestID(context.Background(), "req-abc-123")
	req, err := client.NewRequest(ctx, http.MethodGet, "/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	if err := client.DoJSON(req, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedID != "req-abc-123" {
		t.Errorf("expected request ID req-abc-123, got %s", receivedID)
	}
}

func TestHTTPClientAPIKeyHeader(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "super-secret-key", "test")
	req, err := client.NewRequest(context.Background(), http.MethodGet, "/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	if err := client.DoJSON(req, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedAuth != "ApiKey super-secret-key" {
		t.Errorf("expected Authorization ApiKey super-secret-key, got %s", receivedAuth)
	}
}

func TestHTTPClientTimeoutCausesDependencyUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "key", "test",
		WithTimeout(30*time.Millisecond),
		WithRetryPolicy(RetryPolicy{
			MaxRetries: 2,
			BaseDelay:  5 * time.Millisecond,
			MaxDelay:   10 * time.Millisecond,
		}),
	)

	req, err := client.NewRequest(context.Background(), http.MethodGet, "/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	err = client.DoJSON(req, nil)
	if !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("expected ErrDependencyUnavailable, got %v", err)
	}
}

func TestHTTPClientConnectionCloseCausesDependencyUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("hijacking not supported")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("hijack failed: %v", err)
		}
		_ = conn.Close()
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "key", "test", WithRetryPolicy(RetryPolicy{
		MaxRetries: 2,
		BaseDelay:  5 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
	}))

	req, err := client.NewRequest(context.Background(), http.MethodGet, "/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	err = client.DoJSON(req, nil)
	if !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("expected ErrDependencyUnavailable, got %v", err)
	}
}

// WithRequestID returns a context with the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
