package internalapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// MaxResponseBodyBytes limits the size of outbound JSON response bodies.
const MaxResponseBodyBytes = 1 << 20 // 1 MiB

// ErrDependencyUnavailable is returned when a downstream service cannot be
// reached or explicitly reports that it is unavailable. Callers can use it to
// fail closed without exposing internal errors.
var ErrDependencyUnavailable = errors.New("dependency unavailable")

// ResponseError carries the HTTP status code and response body for a failed
// internal API call. It is returned by DoJSON for non-2xx responses so callers
// can inspect StatusCode instead of parsing error strings.
type ResponseError struct {
	StatusCode int
	Body       string
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("auth_app returned %d: %s", e.StatusCode, e.Body)
}

// Logger is the minimal logging surface used by HTTPClient.
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// RetryPolicy configures retries for safe/idempotent outbound calls.
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryPolicy returns a conservative retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   2 * time.Second,
	}
}

// HTTPClient is a resilient JSON-over-HTTP client for internal service calls.
type HTTPClient struct {
	baseURL         string
	apiKey          string
	serviceName     string
	httpClient      *http.Client
	requestIDHeader string
	logger          Logger
	retry           RetryPolicy
}

// ClientOption customizes an HTTPClient.
type ClientOption func(*HTTPClient)

// NewHTTPClient creates a client. If no http.Client is supplied it uses a
// pooled transport with sensible defaults.
func NewHTTPClient(baseURL, apiKey, serviceName string, opts ...ClientOption) *HTTPClient {
	c := &HTTPClient{
		baseURL:         baseURL,
		apiKey:          apiKey,
		serviceName:     serviceName,
		requestIDHeader: RequestIDHeader,
		retry:           DefaultRetryPolicy(),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithHTTPClient replaces the underlying http.Client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *HTTPClient) { c.httpClient = hc }
}

// WithLogger sets the logger used for per-call metrics.
func WithLogger(l Logger) ClientOption {
	return func(c *HTTPClient) { c.logger = l }
}

// WithRetryPolicy sets the retry policy. Pass RetryPolicy{MaxRetries: 0} to
// disable retries.
func WithRetryPolicy(r RetryPolicy) ClientOption {
	return func(c *HTTPClient) { c.retry = r }
}

// WithTimeout sets the per-request timeout on the default http.Client.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *HTTPClient) {
		if c.httpClient == nil {
			c.httpClient = &http.Client{}
		}
		c.httpClient.Timeout = timeout
	}
}

// NewRequest builds a JSON request with the standard internal-API headers.
func (c *HTTPClient) NewRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+c.apiKey)
	}
	if id := RequestIDFromContext(ctx); id != "" {
		req.Header.Set(c.requestIDHeader, id)
	}
	return req, nil
}

// BaseURL returns the configured base URL.
func (c *HTTPClient) BaseURL() string { return c.baseURL }

// APIKey returns the configured API key.
func (c *HTTPClient) APIKey() string { return c.apiKey }

// ServiceName returns the configured service name.
func (c *HTTPClient) ServiceName() string { return c.serviceName }

// DoJSON executes the request, optionally decodes the response, and retries
// only safe/idempotent methods on transient failures.
func (c *HTTPClient) DoJSON(req *http.Request, dst any) error {
	start := time.Now()
	attempts := 0
	var lastErr error
	var resp *http.Response

	for attempts <= c.retry.MaxRetries {
		attempts++
		resp, lastErr = c.doOnce(req, dst)
		if lastErr == nil {
			c.logCall(req, start, attempts, resp, nil)
			return nil
		}
		if attempts > c.retry.MaxRetries || !c.shouldRetry(req.Method, lastErr) {
			break
		}
		time.Sleep(c.backoff(attempts - 1))
		req = req.Clone(req.Context())
	}

	c.logCall(req, start, attempts, resp, lastErr)
	return lastErr
}

func (c *HTTPClient) doOnce(req *http.Request, dst any) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, c.classifyTransportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusConflict {
			return resp, fmt.Errorf("conflict: %d: %s", resp.StatusCode, string(body))
		}
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout {
			return resp, fmt.Errorf("%w: %w", ErrDependencyUnavailable, &ResponseError{StatusCode: resp.StatusCode, Body: string(body)})
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return resp, fmt.Errorf("authentication failed: %w", &ResponseError{StatusCode: resp.StatusCode, Body: string(body)})
		}
		return resp, &ResponseError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	if dst != nil {
		limited := io.LimitReader(resp.Body, MaxResponseBodyBytes+1)
		if err := json.NewDecoder(limited).Decode(dst); err != nil {
			return resp, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp, nil
}

func (c *HTTPClient) shouldRetry(method string, err error) bool {
	if method != http.MethodGet && method != http.MethodPut && method != http.MethodDelete {
		return false
	}
	if errors.Is(err, ErrDependencyUnavailable) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}
	return false
}

func (c *HTTPClient) backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	d := c.retry.BaseDelay * time.Duration(1<<attempt)
	if d > c.retry.MaxDelay || d <= 0 {
		d = c.retry.MaxDelay
	}
	jitter := time.Duration(rand.Int63n(int64(d) / 2))
	if rand.Intn(2) == 0 {
		jitter = -jitter
	}
	return d + jitter
}

func (c *HTTPClient) classifyTransportError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
	}
	if isConnectionError(err) {
		return fmt.Errorf("%w: %w", ErrDependencyUnavailable, err)
	}
	return err
}

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return true
	}
	return false
}

func (c *HTTPClient) logCall(req *http.Request, start time.Time, attempts int, resp *http.Response, err error) {
	if c.logger == nil {
		return
	}
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	fields := []any{
		"api_target", c.serviceName,
		"api_method", req.Method,
		"api_path", req.URL.Path,
		"status_code", status,
		"duration_ms", time.Since(start).Milliseconds(),
		"attempts", attempts,
		"error_class", c.errorClass(err, status),
	}
	if err != nil {
		c.logger.Error("internal api call", append(fields, "error", err.Error())...)
	} else {
		c.logger.Debug("internal api call", fields...)
	}
}

func (c *HTTPClient) errorClass(err error, status int) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, ErrDependencyUnavailable) || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "auth_failure"
	}
	if status >= 500 {
		return "server_error"
	}
	return "client_error"
}
