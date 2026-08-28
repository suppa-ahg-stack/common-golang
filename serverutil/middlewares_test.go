package serverutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNonceFromContext(t *testing.T) {
	if got := NonceFromContext(context.Background()); got != "" {
		t.Fatalf("NonceFromContext without middleware = %q, want empty", got)
	}

	var nonce string
	handler := CspMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		nonce = NonceFromContext(r.Context())
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if nonce == "" {
		t.Fatal("CspMiddleware did not expose a nonce in the request context")
	}
}
