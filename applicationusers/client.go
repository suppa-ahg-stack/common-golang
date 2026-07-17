package applicationusers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	cinternalapi "suppa-ahg-stack/common-golang/serverutil/internalapi"
)

// ErrNotFound is returned when orgs_backoffice reports that the requested
// application user does not exist (HTTP 404).
var ErrNotFound = errors.New("not found")

// ErrUnavailable is returned when orgs_backoffice cannot be reached or reports
// that it is unavailable.
var ErrUnavailable = cinternalapi.ErrDependencyUnavailable

// Client calls orgs_backoffice's application-user endpoints.
type Client struct {
	http *cinternalapi.HTTPClient
}

// NewClient builds a Client for the given orgs_backoffice base URL. The apiKey
// is sent as the Authorization: ApiKey header on every request.
func NewClient(baseURL, apiKey, serviceName string) *Client {
	if serviceName == "" {
		serviceName = "cfm_planning"
	}
	return &Client{
		http: cinternalapi.NewHTTPClient(
			baseURL,
			apiKey,
			serviceName,
			cinternalapi.WithTimeout(5*time.Second),
		),
	}
}

// NewClientWithLogger builds a Client that logs per-call metrics.
func NewClientWithLogger(baseURL, apiKey, serviceName string, l cinternalapi.Logger) *Client {
	c := NewClient(baseURL, apiKey, serviceName)
	c.http = cinternalapi.NewHTTPClient(
		baseURL,
		apiKey,
		serviceName,
		cinternalapi.WithTimeout(5*time.Second),
		cinternalapi.WithLogger(l),
	)
	return c
}

// GetRolesForUser returns the active role names for the given user on the
// named application. It uses the application-user endpoint so that callers only
// need the application_users:read scope. If the user is not attached to the
// application, an empty role list is returned instead of an error.
func (c *Client) GetRolesForUser(ctx context.Context, userID int64, applicationName string) ([]string, error) {
	path := fmt.Sprintf("/internal/v1/applications/%s/users/%d", url.PathEscape(applicationName), userID)
	req, err := c.http.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Roles []string `json:"roles"`
	}
	if err := c.http.DoJSON(req, &resp); err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, mapError(err)
	}
	return resp.Roles, nil
}

// ListApplicationUsers calls GET
// /internal/v1/applications/{applicationName}/users.
func (c *Client) ListApplicationUsers(ctx context.Context, applicationName string, filter ApplicationUserFilter) (ApplicationUserListResponse, error) {
	u, err := url.Parse(c.http.BaseURL() + fmt.Sprintf("/internal/v1/applications/%s/users", url.PathEscape(applicationName)))
	if err != nil {
		return ApplicationUserListResponse{}, err
	}
	q := u.Query()
	if filter.Search != "" {
		q.Set("search", filter.Search)
	}
	if filter.RoleName != "" {
		q.Set("role_name", filter.RoleName)
	}
	if filter.Limit > 0 {
		q.Set("limit", strconv.Itoa(filter.Limit))
	}
	if filter.Offset > 0 {
		q.Set("offset", strconv.Itoa(filter.Offset))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ApplicationUserListResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	if key := c.http.APIKey(); key != "" {
		req.Header.Set("Authorization", "ApiKey "+key)
	}
	if id := cinternalapi.RequestIDFromContext(ctx); id != "" {
		req.Header.Set(cinternalapi.RequestIDHeader, id)
	}

	var resp ApplicationUserListResponse
	if err := c.http.DoJSON(req, &resp); err != nil {
		return ApplicationUserListResponse{}, mapError(err)
	}
	return resp, nil
}

// GetApplicationUser calls GET
// /internal/v1/applications/{applicationName}/users/{userId}.
func (c *Client) GetApplicationUser(ctx context.Context, applicationName string, userID int64) (ApplicationUserDetailResponse, error) {
	path := fmt.Sprintf("/internal/v1/applications/%s/users/%d", url.PathEscape(applicationName), userID)
	req, err := c.http.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return ApplicationUserDetailResponse{}, err
	}

	var resp ApplicationUserDetailResponse
	if err := c.http.DoJSON(req, &resp); err != nil {
		return ApplicationUserDetailResponse{}, mapError(err)
	}
	return resp, nil
}

// actorUserIDPayload is the request body used by toggle and password-reset
// endpoints that accept an optional actor override.
type actorUserIDPayload struct {
	ActorUserID int64 `json:"actor_user_id"`
}

// CreateOrAttachApplicationUser calls POST
// /internal/v1/applications/{applicationName}/users.
func (c *Client) CreateOrAttachApplicationUser(ctx context.Context, applicationName, email string, roles []string, actorUserID int64) (ApplicationUserDetailResponse, error) {
	path := fmt.Sprintf("/internal/v1/applications/%s/users", url.PathEscape(applicationName))
	req, err := c.http.NewRequest(ctx, http.MethodPost, path, CreateOrAttachApplicationUserRequest{
		Email:       email,
		Roles:       roles,
		ActorUserID: actorUserID,
	})
	if err != nil {
		return ApplicationUserDetailResponse{}, err
	}

	var resp ApplicationUserDetailResponse
	if err := c.http.DoJSON(req, &resp); err != nil {
		return ApplicationUserDetailResponse{}, mapError(err)
	}
	return resp, nil
}

// UpdateApplicationUserRoles calls PUT
// /internal/v1/applications/{applicationName}/users/{userId}/roles.
func (c *Client) UpdateApplicationUserRoles(ctx context.Context, applicationName string, userID int64, roles []string, actorUserID int64) (ApplicationUserDetailResponse, error) {
	path := fmt.Sprintf("/internal/v1/applications/%s/users/%d/roles", url.PathEscape(applicationName), userID)
	req, err := c.http.NewRequest(ctx, http.MethodPut, path, UpdateApplicationUserRolesRequest{
		Roles:       roles,
		ActorUserID: actorUserID,
	})
	if err != nil {
		return ApplicationUserDetailResponse{}, err
	}

	var resp ApplicationUserDetailResponse
	if err := c.http.DoJSON(req, &resp); err != nil {
		return ApplicationUserDetailResponse{}, mapError(err)
	}
	return resp, nil
}

// ToggleApplicationUserActive calls POST
// /internal/v1/applications/{applicationName}/users/{userId}/toggle-active.
func (c *Client) ToggleApplicationUserActive(ctx context.Context, applicationName string, userID int64, actorUserID int64) (bool, error) {
	return c.toggleState(ctx, applicationName, userID, "toggle-active", actorUserID)
}

// ToggleApplicationUserBlocked calls POST
// /internal/v1/applications/{applicationName}/users/{userId}/toggle-blocked.
func (c *Client) ToggleApplicationUserBlocked(ctx context.Context, applicationName string, userID int64, actorUserID int64) (bool, error) {
	return c.toggleState(ctx, applicationName, userID, "toggle-blocked", actorUserID)
}

func (c *Client) toggleState(ctx context.Context, applicationName string, userID int64, action string, actorUserID int64) (bool, error) {
	path := fmt.Sprintf("/internal/v1/applications/%s/users/%d/%s", url.PathEscape(applicationName), userID, action)
	req, err := c.http.NewRequest(ctx, http.MethodPost, path, actorUserIDPayload{ActorUserID: actorUserID})
	if err != nil {
		return false, err
	}

	var resp ToggleStateResponse
	if err := c.http.DoJSON(req, &resp); err != nil {
		return false, mapError(err)
	}
	return resp.NewState, nil
}

// SendApplicationUserPasswordReset calls POST
// /internal/v1/applications/{applicationName}/users/{userId}/password-reset.
func (c *Client) SendApplicationUserPasswordReset(ctx context.Context, applicationName string, userID int64, actorUserID int64) (PasswordResetResponse, error) {
	path := fmt.Sprintf("/internal/v1/applications/%s/users/%d/password-reset", url.PathEscape(applicationName), userID)
	req, err := c.http.NewRequest(ctx, http.MethodPost, path, actorUserIDPayload{ActorUserID: actorUserID})
	if err != nil {
		return PasswordResetResponse{}, err
	}

	var resp PasswordResetResponse
	if err := c.http.DoJSON(req, &resp); err != nil {
		return PasswordResetResponse{}, mapError(err)
	}
	return resp, nil
}

// GetApplicationUserAudit calls GET
// /internal/v1/applications/{applicationName}/users/{userId}/audit.
func (c *Client) GetApplicationUserAudit(ctx context.Context, applicationName string, userID int64) (ApplicationUserAuditResponse, error) {
	path := fmt.Sprintf("/internal/v1/applications/%s/users/%d/audit", url.PathEscape(applicationName), userID)
	req, err := c.http.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return ApplicationUserAuditResponse{}, err
	}

	var resp ApplicationUserAuditResponse
	if err := c.http.DoJSON(req, &resp); err != nil {
		return ApplicationUserAuditResponse{}, mapError(err)
	}
	return resp, nil
}

// HealthCheck performs a lightweight call to orgs_backoffice /health and
// reports latency.
func (c *Client) HealthCheck(ctx context.Context) (healthy bool, latency time.Duration, err error) {
	start := time.Now()
	req, err := c.http.NewRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return false, 0, err
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := c.http.DoJSON(req, &resp); err != nil {
		return false, time.Since(start), err
	}
	return resp.Status == "ok", time.Since(start), nil
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if isNotFound(err) {
		return ErrNotFound
	}
	if errors.Is(err, ErrUnavailable) {
		return ErrUnavailable
	}
	var respErr *cinternalapi.ResponseError
	if errors.As(err, &respErr) {
		return respErr
	}
	return fmt.Errorf("orgs_backoffice call failed: %w", err)
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var respErr *cinternalapi.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}
