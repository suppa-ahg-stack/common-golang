package authapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	cinternalapi "suppa-ahg-stack/common-golang/serverutil/internalapi"
)

var (
	ErrConflict              = cinternalapi.ErrConflict
	ErrNotFound              = cinternalapi.ErrNotFound
	ErrUnavailable           = cinternalapi.ErrDependencyUnavailable
	ErrMissingIdempotencyKey = errors.New("idempotency key is required")
)

type Client struct {
	http *cinternalapi.HTTPClient
}

func NewClient(baseURL, apiKey, serviceName string) *Client {
	return newClient(baseURL, apiKey, serviceName)
}

func NewClientWithLogger(baseURL, apiKey, serviceName string, l cinternalapi.Logger) *Client {
	return newClient(baseURL, apiKey, serviceName, cinternalapi.WithLogger(l))
}

// NewClientWithHTTPClient uses the supplied transport and timeout policy. It
// is useful when an application already owns a pooled client shared with other
// internal dependencies.
func NewClientWithHTTPClient(baseURL, apiKey, serviceName string, httpClient *http.Client) *Client {
	return newClient(baseURL, apiKey, serviceName, cinternalapi.WithHTTPClient(httpClient))
}

func newClient(baseURL, apiKey, serviceName string, opts ...cinternalapi.ClientOption) *Client {
	if serviceName == "" {
		serviceName = "application"
	}
	opts = append([]cinternalapi.ClientOption{cinternalapi.WithTimeout(5 * time.Second)}, opts...)
	return &Client{http: cinternalapi.NewHTTPClient(strings.TrimRight(baseURL, "/"), apiKey, serviceName, opts...)}
}

func (c *Client) ValidateSession(ctx context.Context, sessionToken, refreshToken string) (*SessionValidateResponse, error) {
	req, err := c.http.NewRequest(ctx, http.MethodPost, "/internal/v1/sessions/validate", SessionValidateRequest{
		SessionToken: sessionToken,
		RefreshToken: refreshToken,
	})
	if err != nil {
		return nil, err
	}
	var resp SessionValidateResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListUsers(ctx context.Context, query string, limit int) (*ListUsersResponse, error) {
	u, err := url.Parse(c.http.BaseURL() + "/internal/v1/users")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", query)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	u.RawQuery = q.Encode()

	req, err := c.requestWithHeaders(ctx, http.MethodGet, u.String())
	if err != nil {
		return nil, err
	}
	var resp ListUsersResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetUser(ctx context.Context, id int64) (*UserResponse, error) {
	req, err := c.http.NewRequest(ctx, http.MethodGet, fmt.Sprintf("/internal/v1/users/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var resp UserResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) BatchGetUsers(ctx context.Context, ids []int64, emails []string) ([]UserResponse, error) {
	req, err := c.http.NewRequest(ctx, http.MethodPost, "/internal/v1/users:batchGet", BatchGetUsersRequest{IDs: ids, Emails: emails})
	if err != nil {
		return nil, err
	}
	var resp ListUsersResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return resp.Users, nil
}

func (c *Client) CreateUser(ctx context.Context, payload CreateUserRequest) (*CreateUserResponse, error) {
	req, err := c.idempotentRequest(ctx, http.MethodPost, "/internal/v1/users", payload, payload.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	var resp CreateUserResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) UpdateUser(ctx context.Context, id int64, payload UpdateUserRequest) (*UserResponse, error) {
	req, err := c.idempotentRequest(ctx, http.MethodPatch, fmt.Sprintf("/internal/v1/users/%d", id), payload, payload.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	var resp UserResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CreatePasswordSetupInvitation(ctx context.Context, id int64, key string) (*CreatePasswordSetupInvitationResponse, error) {
	req, err := c.idempotentRequest(ctx, http.MethodPost, fmt.Sprintf("/internal/v1/users/%d/password-setup-invitations", id), nil, key)
	if err != nil {
		return nil, err
	}
	var resp CreatePasswordSetupInvitationResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CreatePasswordResetInvitation(ctx context.Context, id int64, key string) (*CreatePasswordResetInvitationResponse, error) {
	req, err := c.idempotentRequest(ctx, http.MethodPost, fmt.Sprintf("/internal/v1/users/%d/password-reset-invitations", id), nil, key)
	if err != nil {
		return nil, err
	}
	var resp CreatePasswordResetInvitationResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) HealthCheck(ctx context.Context) (bool, time.Duration, error) {
	start := time.Now()
	req, err := c.http.NewRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return false, 0, err
	}
	var resp struct {
		Status string `json:"status"`
	}
	err = c.http.DoJSON(req, &resp)
	latency := time.Since(start)
	if err != nil {
		return false, latency, mapError(err)
	}
	return resp.Status == "ok", latency, nil
}

func (c *Client) idempotentRequest(ctx context.Context, method, path string, payload any, key string) (*http.Request, error) {
	if key == "" {
		return nil, ErrMissingIdempotencyKey
	}
	if err := cinternalapi.ValidateIdempotencyKey(key); err != nil {
		return nil, err
	}
	req, err := c.http.NewRequest(ctx, method, path, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Idempotency-Key", key)
	return req, nil
}

func (c *Client) requestWithHeaders(ctx context.Context, method, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if key := c.http.APIKey(); key != "" {
		req.Header.Set("Authorization", "ApiKey "+key)
	}
	if id := cinternalapi.RequestIDFromContext(ctx); id != "" {
		req.Header.Set(cinternalapi.RequestIDHeader, id)
	}
	return req, nil
}

func (c *Client) doJSON(req *http.Request, dst any) error {
	if err := c.http.DoJSON(req, dst); err != nil {
		return mapError(err)
	}
	return nil
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, cinternalapi.ErrDependencyUnavailable) {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var responseErr *cinternalapi.ResponseError
	if errors.As(err, &responseErr) {
		switch responseErr.StatusCode {
		case http.StatusConflict:
			return fmt.Errorf("%w: %v", ErrConflict, err)
		case http.StatusNotFound:
			return fmt.Errorf("%w: %v", ErrNotFound, err)
		}
	}
	return err
}
