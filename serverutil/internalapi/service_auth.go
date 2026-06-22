package internalapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ServiceCredential describes an authorized caller of the internal API.
type ServiceCredential struct {
	Name       string     `json:"name"`
	ActiveKeys []string   `json:"active_keys"`
	Scopes     []string   `json:"scopes"`
	Expiry     *time.Time `json:"expiry"`
}

type serviceCredential struct {
	name    string
	rawKeys []string
	scopes  map[string]bool
	expiry  *time.Time
}

type serviceNameKey struct{}

// WithServiceName attaches a caller service name to the context. It is used by
// downstream handlers that need to know which authenticated service made the
// request (for example, to scope idempotency records).
func WithServiceName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, serviceNameKey{}, name)
}

// ServiceNameFromContext returns the caller service name previously attached
// by ServiceAuth.Middleware, or false if none is present.
func ServiceNameFromContext(ctx context.Context) (string, bool) {
	if name, ok := ctx.Value(serviceNameKey{}).(string); ok && name != "" {
		return name, true
	}
	return "", false
}

// ServiceAuth validates API keys and required scopes for internal endpoints.
type ServiceAuth struct {
	byKeyHash map[string]*serviceCredential
	byName    map[string]*serviceCredential
}

// NewServiceAuth builds a ServiceAuth from the provided credentials. Raw keys
// are hashed immediately and compared in constant time.
func NewServiceAuth(creds []ServiceCredential) (*ServiceAuth, error) {
	auth := &ServiceAuth{
		byKeyHash: make(map[string]*serviceCredential),
		byName:    make(map[string]*serviceCredential),
	}
	for _, c := range creds {
		if c.Name == "" {
			return nil, errors.New("service credential name is required")
		}
		if _, exists := auth.byName[c.Name]; exists {
			return nil, fmt.Errorf("duplicate service credential name: %s", c.Name)
		}
		sc := &serviceCredential{
			name:    c.Name,
			rawKeys: make([]string, 0, len(c.ActiveKeys)),
			scopes:  make(map[string]bool, len(c.Scopes)),
			expiry:  c.Expiry,
		}
		for _, key := range c.ActiveKeys {
			if key == "" {
				return nil, errors.New("empty active key")
			}
			h := hashKey(key)
			if _, exists := auth.byKeyHash[h]; exists {
				return nil, fmt.Errorf("duplicate active key for service %s", c.Name)
			}
			sc.rawKeys = append(sc.rawKeys, key)
			auth.byKeyHash[h] = sc
		}
		for _, s := range c.Scopes {
			sc.scopes[s] = true
		}
		auth.byName[c.Name] = sc
	}
	return auth, nil
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func extractAPIKey(r *http.Request) (string, bool) {
	const prefix = "ApiKey "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}

func (a *ServiceAuth) authenticate(r *http.Request) (*serviceCredential, bool) {
	rawKey, ok := extractAPIKey(r)
	if !ok {
		return nil, false
	}
	providedHash := hashKey(rawKey)
	cred, ok := a.byKeyHash[providedHash]
	if !ok {
		return nil, false
	}
	if cred.expiry != nil && time.Now().After(*cred.expiry) {
		return nil, false
	}
	return cred, true
}

// Middleware returns an http middleware that enforces the given scopes.
func (a *ServiceAuth) Middleware(requiredScopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			cred, ok := a.authenticate(r)
			if !ok {
				WriteError(ctx, w, http.StatusUnauthorized, "unauthenticated", "missing or invalid API key")
				return
			}
			for _, s := range requiredScopes {
				if !cred.scopes[s] {
					WriteError(ctx, w, http.StatusForbidden, "forbidden", "missing required scope: "+s)
					return
				}
			}
			ctx = WithServiceName(ctx, cred.name)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClientKey returns a raw active key for the named service so it can be used
// as the Authorization header on outbound requests.
func (a *ServiceAuth) ClientKey(serviceName string) (string, bool) {
	cred, ok := a.byName[serviceName]
	if !ok || len(cred.rawKeys) == 0 {
		return "", false
	}
	return cred.rawKeys[0], true
}

// ConstantTimeCompareKeys compares two raw keys in constant time by hashing
// them first. It is exported for tests.
func ConstantTimeCompareKeys(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(hashKey(a)), []byte(hashKey(b))) == 1
}
