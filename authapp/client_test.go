package authapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	cinternalapi "suppa-ahg-stack/common-golang/serverutil/internalapi"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func testClient(t *testing.T, handler roundTripFunc) *Client {
	t.Helper()
	return newClient("http://auth.test", "secret", "test", cinternalapi.WithHTTPClient(&http.Client{Transport: handler}))
}

func TestValidateSessionCarriesCookiePolicy(t *testing.T) {
	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/internal/v1/sessions/validate" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "ApiKey secret" {
			t.Fatalf("authorization = %q", got)
		}
		var body strings.Builder
		_ = json.NewEncoder(&body).Encode(SessionValidateResponse{
			User:                      &Identity{ID: 42, Email: "user@example.com"},
			SessionToken:              "rotated",
			RefreshToken:              "refresh",
			SessionDomain:             ".example.com",
			SessionMaxAgeSeconds:      60,
			RefreshTokenMaxAgeSeconds: 120,
		})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body.String())), Header: make(http.Header)}, nil
	})

	resp, err := client.ValidateSession(context.Background(), "old", "refresh-old")
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if resp.User == nil || resp.User.ID != 42 || resp.CookiePolicy().Domain != ".example.com" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestMutationRequiresValidIdempotencyKey(t *testing.T) {
	client := NewClient("http://localhost", "key", "test")
	if _, err := client.CreateUser(context.Background(), CreateUserRequest{}); !errors.Is(err, ErrMissingIdempotencyKey) {
		t.Fatalf("missing key error = %v", err)
	}
	if _, err := client.CreateUser(context.Background(), CreateUserRequest{IdempotencyKey: "bad key"}); !errors.Is(err, cinternalapi.ErrInvalidIdempotencyKey) {
		t.Fatalf("invalid key error = %v", err)
	}
}

func TestErrorMappingUsesStatusCode(t *testing.T) {
	client := testClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("missing")), Header: make(http.Header)}, nil
	})

	_, err := client.GetUser(context.Background(), 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUser() error = %v, want ErrNotFound", err)
	}
}

func TestWireContracts(t *testing.T) {
	t.Run("session", func(t *testing.T) {
		var response SessionValidateResponse
		payload := `{"user":{"id":1,"email":"user@example.com","full_name":"User","status":"active","password_must_change":false},"session_token":"new-token","refresh_token":"refresh-token","session_domain":".example.test","session_max_age_seconds":60,"refresh_token_max_age_seconds":120}`
		if err := json.Unmarshal([]byte(payload), &response); err != nil {
			t.Fatal(err)
		}
		if response.User == nil || response.User.ID != 1 || response.SessionToken != "new-token" || response.RefreshToken != "refresh-token" {
			t.Fatalf("unexpected session response: %+v", response)
		}
		if got := response.CookiePolicy(); got.Domain != ".example.test" || got.SessionMaxAgeSeconds != 60 || got.RefreshTokenMaxAgeSeconds != 120 {
			t.Fatalf("unexpected cookie policy: %+v", got)
		}
	})

	t.Run("null session user", func(t *testing.T) {
		var response SessionValidateResponse
		if err := json.Unmarshal([]byte(`{"user":null,"session_token":"","refresh_token":""}`), &response); err != nil {
			t.Fatal(err)
		}
		if response.User != nil {
			t.Fatalf("expected nil user, got %+v", response.User)
		}
	})

	t.Run("user list", func(t *testing.T) {
		var response ListUsersResponse
		payload := `{"users":[{"id":1,"email":"a@example.com","full_name":"A","status":"active","password_must_change":false,"profile":{"first_name":"F","last_name":null,"phone":null,"email_verified_at":null,"phone_verified_at":null}}],"next":false}`
		if err := json.Unmarshal([]byte(payload), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Users) != 1 || response.Users[0].Email != "a@example.com" || response.Users[0].Profile.EmailVerifiedAt != nil {
			t.Fatalf("unexpected list response: %+v", response)
		}
	})

	t.Run("create response", func(t *testing.T) {
		for _, payload := range []string{
			`{"id":99,"created":true,"status":"pending","replayed":false}`,
			`{"id":99,"created":false,"status":"active","replayed":true}`,
		} {
			var response CreateUserResponse
			if err := json.Unmarshal([]byte(payload), &response); err != nil {
				t.Fatal(err)
			}
			if response.ID != 99 || response.Status == "" {
				t.Fatalf("unexpected create response: %+v", response)
			}
		}
	})

	t.Run("verification timestamps", func(t *testing.T) {
		var response UserResponse
		payload := `{"id":1,"email":"old@example.com","full_name":"User","status":"active","password_must_change":false,"profile":{"first_name":"F","last_name":"L","phone":"+123","email_verified_at":"2026-01-15T10:00:00Z","phone_verified_at":null}}`
		if err := json.Unmarshal([]byte(payload), &response); err != nil {
			t.Fatal(err)
		}
		if response.Profile.EmailVerifiedAt == nil || response.Profile.PhoneVerifiedAt != nil {
			t.Fatalf("unexpected verification timestamps: %+v", response.Profile)
		}
	})
}

func TestMutationsValidateIdempotencyKeys(t *testing.T) {
	client := NewClient("http://localhost", "key", "test")
	newName := "New name"
	tests := []struct {
		name string
		call func(string) error
	}{
		{"create user", func(key string) error {
			_, err := client.CreateUser(context.Background(), CreateUserRequest{Email: "a@example.com", FullName: "A", IdempotencyKey: key})
			return err
		}},
		{"update user", func(key string) error {
			_, err := client.UpdateUser(context.Background(), 1, UpdateUserRequest{FullName: &newName, IdempotencyKey: key})
			return err
		}},
		{"setup invitation", func(key string) error {
			_, err := client.CreatePasswordSetupInvitation(context.Background(), 1, key)
			return err
		}},
		{"reset invitation", func(key string) error {
			_, err := client.CreatePasswordResetInvitation(context.Background(), 1, key)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name+" missing", func(t *testing.T) {
			if err := test.call(""); !errors.Is(err, ErrMissingIdempotencyKey) {
				t.Fatalf("error = %v, want ErrMissingIdempotencyKey", err)
			}
		})
		t.Run(test.name+" invalid", func(t *testing.T) {
			if err := test.call("bad key"); !errors.Is(err, cinternalapi.ErrInvalidIdempotencyKey) {
				t.Fatalf("error = %v, want ErrInvalidIdempotencyKey", err)
			}
		})
	}
}

func TestClientRequestContracts(t *testing.T) {
	var requests []*http.Request
	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.Clone(r.Context()))
		if r.Method == http.MethodGet {
			return jsonResponse(http.StatusOK, `{"users":[],"next":false}`), nil
		}
		return jsonResponse(http.StatusOK, `{"id":7,"created":true,"status":"pending","replayed":false}`), nil
	})

	var ctx context.Context
	contextRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	contextRequest.Header.Set(cinternalapi.RequestIDHeader, "req-42")
	cinternalapi.RequestIDMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	})).ServeHTTP(httptest.NewRecorder(), contextRequest)
	if _, err := client.ListUsers(ctx, "alice@example.com", 12); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateUser(ctx, CreateUserRequest{Email: "alice@example.com", FullName: "Alice", IdempotencyKey: "create-user-alice-0001"}); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	query, _ := url.ParseQuery(requests[0].URL.RawQuery)
	if requests[0].URL.Path != "/internal/v1/users" || query.Get("query") != "alice@example.com" || query.Get("limit") != "12" {
		t.Fatalf("unexpected list request: %s", requests[0].URL.String())
	}
	for _, request := range requests {
		if request.Header.Get("Authorization") != "ApiKey secret" || request.Header.Get(cinternalapi.RequestIDHeader) != "req-42" {
			t.Fatalf("unexpected headers: %v", request.Header)
		}
	}
	if got := requests[1].Header.Get("Idempotency-Key"); got != "create-user-alice-0001" {
		t.Fatalf("Idempotency-Key = %q", got)
	}
}

func TestClientMapsCanonicalErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"conflict", http.StatusConflict, ErrConflict},
		{"not found", http.StatusNotFound, ErrNotFound},
		{"unavailable", http.StatusServiceUnavailable, ErrUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := testClient(t, func(*http.Request) (*http.Response, error) {
				return jsonResponse(test.status, `{"error":"failed"}`), nil
			})
			_, err := client.GetUser(context.Background(), 1)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestHealthCheckUsesHealthEndpoint(t *testing.T) {
	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			t.Fatalf("unexpected health request: %s %s", r.Method, r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"status":"ok"}`), nil
	})
	healthy, latency, err := client.HealthCheck(context.Background())
	if err != nil || !healthy || latency < 0 || latency > time.Second {
		t.Fatalf("HealthCheck() = %v, %v, %v", healthy, latency, err)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
