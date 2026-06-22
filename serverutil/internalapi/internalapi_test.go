package internalapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestIDMiddlewarePropagatesHeader(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := RequestIDFromContext(r.Context()); id == "" {
			t.Error("expected request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("generates ID when missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Header().Get(RequestIDHeader) == "" {
			t.Error("expected response request ID header")
		}
	})

	t.Run("preserves provided ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(RequestIDHeader, "req-123")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if got := rr.Header().Get(RequestIDHeader); got != "req-123" {
			t.Errorf("expected request ID req-123, got %s", got)
		}
	})
}

func TestTimeoutMiddleware(t *testing.T) {
	handler := TimeoutMiddleware(50 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(200 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected timeout to abort handler, got %d", rr.Code)
	}
}

func TestParseJSON(t *testing.T) {
	t.Run("valid single object", func(t *testing.T) {
		body := strings.NewReader(`{"name":"alice"}`)
		req := httptest.NewRequest(http.MethodPost, "/", body)
		req.Header.Set("Content-Type", "application/json")
		var dst struct{ Name string }
		if err := ParseJSON(req, &dst); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dst.Name != "alice" {
			t.Errorf("expected alice, got %s", dst.Name)
		}
	})

	t.Run("rejects unknown fields", func(t *testing.T) {
		body := strings.NewReader(`{"name":"alice","extra":1}`)
		req := httptest.NewRequest(http.MethodPost, "/", body)
		var dst struct{ Name string }
		if err := ParseJSON(req, &dst); err == nil {
			t.Error("expected error for unknown field")
		}
	})

	t.Run("rejects multiple objects", func(t *testing.T) {
		body := strings.NewReader(`{"name":"alice"}{"name":"bob"}`)
		req := httptest.NewRequest(http.MethodPost, "/", body)
		var dst struct{ Name string }
		if err := ParseJSON(req, &dst); err == nil {
			t.Error("expected error for multiple JSON objects")
		}
	})

	t.Run("rejects oversized body", func(t *testing.T) {
		large := make([]byte, MaxRequestBodyBytes+1)
		body := bytes.NewReader(large)
		req := httptest.NewRequest(http.MethodPost, "/", body)
		var dst struct{ Name string }
		if err := ParseJSON(req, &dst); err == nil {
			t.Error("expected error for oversized body")
		}
	})
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteJSON(rr, http.StatusCreated, map[string]string{"status": "ok"})
	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("unexpected body %v", body)
	}
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()
	ctx := context.WithValue(context.Background(), requestIDKey{}, "req-42")
	WriteError(ctx, rr, http.StatusUnprocessableEntity, "invalid_email", "bad email")
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rr.Code)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Code != "invalid_email" || resp.Message != "bad email" || resp.RequestID != "req-42" {
		t.Errorf("unexpected response %+v", resp)
	}
}

func TestNewServiceAuth(t *testing.T) {
	_, err := NewServiceAuth([]ServiceCredential{
		{Name: "orgs", ActiveKeys: []string{""}, Scopes: []string{"users:read"}},
	})
	if err == nil {
		t.Error("expected error for empty key")
	}

	_, err = NewServiceAuth([]ServiceCredential{
		{Name: "", ActiveKeys: []string{"k"}, Scopes: []string{"users:read"}},
	})
	if err == nil {
		t.Error("expected error for empty service name")
	}

	_, err = NewServiceAuth([]ServiceCredential{
		{Name: "a", ActiveKeys: []string{"k"}, Scopes: []string{"users:read"}},
		{Name: "b", ActiveKeys: []string{"k"}, Scopes: []string{"users:read"}},
	})
	if err == nil {
		t.Error("expected error for duplicate active key")
	}
}

func TestServiceAuthMiddleware(t *testing.T) {
	auth, err := NewServiceAuth([]ServiceCredential{
		{
			Name:       "orgs_backoffice",
			ActiveKeys: []string{"valid-key", "rotated-key"},
			Scopes:     []string{"users:read", "users:write"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler := auth.Middleware("users:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))

	t.Run("missing header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("invalid key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "ApiKey wrong-key")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("missing scope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "ApiKey valid-key")
		required := auth.Middleware("entitlements:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		rr := httptest.NewRecorder()
		required.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rr.Code)
		}
	})

	t.Run("valid key and scope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "ApiKey valid-key")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("injects service name into context", func(t *testing.T) {
		var gotName string
		var gotOK bool
		nameHandler := auth.Middleware("users:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotName, gotOK = ServiceNameFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "ApiKey valid-key")
		rr := httptest.NewRecorder()
		nameHandler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !gotOK || gotName != "orgs_backoffice" {
			t.Errorf("expected service name orgs_backoffice, got %q, ok=%v", gotName, gotOK)
		}
	})

	t.Run("rotated key still works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "ApiKey rotated-key")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("expired key rejected", func(t *testing.T) {
		past := time.Now().Add(-time.Hour)
		expAuth, _ := NewServiceAuth([]ServiceCredential{
			{Name: "x", ActiveKeys: []string{"expired-key"}, Scopes: []string{"users:read"}, Expiry: &past},
		})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "ApiKey expired-key")
		rr := httptest.NewRecorder()
		expAuth.Middleware("users:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for expired key, got %d", rr.Code)
		}
	})
}

func TestServiceAuthClientKey(t *testing.T) {
	auth, _ := NewServiceAuth([]ServiceCredential{
		{Name: "orgs_backoffice", ActiveKeys: []string{"outbound-key"}, Scopes: []string{"users:write"}},
	})
	key, ok := auth.ClientKey("orgs_backoffice")
	if !ok || key != "outbound-key" {
		t.Errorf("expected outbound-key, got %s, ok=%v", key, ok)
	}
	if _, ok := auth.ClientKey("unknown"); ok {
		t.Error("expected no key for unknown service")
	}
}

func TestServiceAuthRotationOverlap(t *testing.T) {
	newKey := "new-key"
	oldKey := "old-key"

	auth, err := NewServiceAuth([]ServiceCredential{
		{Name: "service", ActiveKeys: []string{newKey, oldKey}, Scopes: []string{"read"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler := auth.Middleware("read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, key := range []string{newKey, oldKey} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "ApiKey "+key)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for key %q, got %d", key, rr.Code)
		}
	}

	// Rotate: drop old key.
	rotatedAuth, err := NewServiceAuth([]ServiceCredential{
		{Name: "service", ActiveKeys: []string{newKey}, Scopes: []string{"read"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rotatedHandler := rotatedAuth.Middleware("read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "ApiKey "+newKey)
	rr := httptest.NewRecorder()
	rotatedHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for retained key, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "ApiKey "+oldKey)
	rr = httptest.NewRecorder()
	rotatedHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for removed key, got %d", rr.Code)
	}
}

func TestConstantTimeCompareKeys(t *testing.T) {
	if !ConstantTimeCompareKeys("same-key", "same-key") {
		t.Error("expected identical keys to compare equal")
	}
	if ConstantTimeCompareKeys("key-one", "key-two") {
		t.Error("expected different keys to compare unequal")
	}
}
