package authsession

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"suppa-ahg-stack/common-golang/authapp"
)

type validatorFunc func(context.Context, string, string) (*authapp.SessionValidateResponse, error)

func (f validatorFunc) ValidateSession(ctx context.Context, session, refresh string) (*authapp.SessionValidateResponse, error) {
	return f(ctx, session, refresh)
}

func testManager(t *testing.T, validate validatorFunc, roles RoleProvider, policy RoleErrorPolicy) *Manager {
	t.Helper()
	m := NewManager(validate, roles, Config{
		MaxAge:           time.Hour,
		IdleTimeout:      time.Hour,
		IdentityCacheTTL: time.Hour,
		CSRFMaxAge:       time.Hour,
		RoleCacheTTL:     time.Hour,
		RoleErrorPolicy:  policy,
	}, nil)
	t.Cleanup(m.Stop)
	return m
}

func TestValidateCachesIdentityAndRoles(t *testing.T) {
	var identityCalls, roleCalls atomic.Int32
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		identityCalls.Add(1)
		return &authapp.SessionValidateResponse{User: &authapp.Identity{ID: 1, Email: "a@example.com"}}, nil
	}, func(context.Context, int64) ([]string, error) {
		roleCalls.Add(1)
		return []string{"admin"}, nil
	}, ReturnRoleError)

	for range 2 {
		user, _, _, err := m.Validate(context.Background(), "token", "")
		if err != nil || user == nil || !user.HasRole("admin") {
			t.Fatalf("Validate() = %+v, %v", user, err)
		}
	}
	if identityCalls.Load() != 1 || roleCalls.Load() != 1 {
		t.Fatalf("calls identity=%d roles=%d, want 1/1", identityCalls.Load(), roleCalls.Load())
	}
}

func TestValidateFreshDetectsRevocation(t *testing.T) {
	var active atomic.Bool
	active.Store(true)
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		if !active.Load() {
			return &authapp.SessionValidateResponse{}, nil
		}
		return &authapp.SessionValidateResponse{User: &authapp.Identity{ID: 1}}, nil
	}, nil, ReturnRoleError)

	if user, _, _, err := m.Validate(context.Background(), "token", ""); err != nil || user == nil {
		t.Fatalf("initial Validate() = %+v, %v", user, err)
	}
	active.Store(false)
	if user, _, _, err := m.ValidateFresh(context.Background(), "token", ""); err != nil || user != nil {
		t.Fatalf("ValidateFresh() = %+v, %v", user, err)
	}
	if m.HasSession("token") {
		t.Fatal("revoked session remained cached")
	}
}

func TestRotationTransfersCSRFAndEvictsOldToken(t *testing.T) {
	var calls atomic.Int32
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		if calls.Add(1) == 1 {
			return &authapp.SessionValidateResponse{User: &authapp.Identity{ID: 7}}, nil
		}
		return &authapp.SessionValidateResponse{User: &authapp.Identity{ID: 7}, SessionToken: "rotated", RefreshToken: "refresh"}, nil
	}, nil, ReturnRoleError)
	if _, _, _, err := m.Validate(context.Background(), "old", ""); err != nil {
		t.Fatal(err)
	}
	csrf, err := m.SetCsrfToken("old")
	if err != nil {
		t.Fatal(err)
	}
	_, session, refresh, err := m.ValidateFresh(context.Background(), "old", "")
	if err != nil || session != "rotated" || refresh != "refresh" {
		t.Fatalf("rotation = %q/%q, %v", session, refresh, err)
	}
	if m.HasSession("old") || !m.HasSession("rotated") {
		t.Fatal("rotation cache indexes are inconsistent")
	}
	if ok, err := m.CheckCsrf(csrf, "rotated"); err != nil || !ok {
		t.Fatalf("rotated CSRF = %v, %v", ok, err)
	}
}

func TestConcurrentCacheMissUsesSingleValidation(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return &authapp.SessionValidateResponse{User: &authapp.Identity{ID: 1}}, nil
	}, nil, ReturnRoleError)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, _ = m.Validate(context.Background(), "token", "")
		}()
	}
	<-started
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("validation calls = %d, want 1", calls.Load())
	}
}

func TestRoleErrorPoliciesFailClosed(t *testing.T) {
	want := errors.New("roles unavailable")
	provider := func(context.Context, int64) ([]string, error) { return nil, want }
	validator := func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		return &authapp.SessionValidateResponse{User: &authapp.Identity{ID: 1}}, nil
	}
	strict := testManager(t, validator, provider, ReturnRoleError)
	if user, _, _, err := strict.Validate(context.Background(), "strict", ""); user != nil || !errors.Is(err, want) {
		t.Fatalf("strict result = %+v, %v", user, err)
	}
	deny := testManager(t, validator, provider, DenyRolesOnError)
	if user, _, _, err := deny.Validate(context.Background(), "deny", ""); err != nil || user == nil || len(user.Roles) != 0 {
		t.Fatalf("deny result = %+v, %v", user, err)
	}
}

func TestAnonymousSessionAndCSRF(t *testing.T) {
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		return &authapp.SessionValidateResponse{}, nil
	}, nil, ReturnRoleError)
	token, err := m.CreateAnonymous()
	if err != nil {
		t.Fatal(err)
	}
	csrf, err := m.SetCsrfToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := m.CheckCsrf(csrf, token); err != nil || !ok {
		t.Fatalf("CheckCsrf() = %v, %v", ok, err)
	}
	if ok, err := m.CheckCsrf("wrong", token); err != nil || ok {
		t.Fatalf("wrong CheckCsrf() = %v, %v", ok, err)
	}
}

func TestValidateKeepsTokenWithoutRotation(t *testing.T) {
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		return &authapp.SessionValidateResponse{User: &authapp.Identity{ID: 5}}, nil
	}, nil, ReturnRoleError)
	user, session, refresh, err := m.Validate(context.Background(), "stable", "")
	if err != nil || user == nil || session != "" || refresh != "" {
		t.Fatalf("Validate() = %+v, %q, %q, %v", user, session, refresh, err)
	}
	if !m.HasSession("stable") || len(m.ActiveSessionIDs(5)) != 1 || m.ActiveSessionIDs(5)[0] != "stable" {
		t.Fatalf("stable session was not indexed: %v", m.ActiveSessionIDs(5))
	}
}

func TestIdentityCacheTTLAndIdleExpiryRevalidate(t *testing.T) {
	var calls atomic.Int32
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		calls.Add(1)
		return &authapp.SessionValidateResponse{User: &authapp.Identity{ID: 1}}, nil
	}, nil, ReturnRoleError)

	if _, _, _, err := m.Validate(context.Background(), "token", ""); err != nil {
		t.Fatal(err)
	}
	hash := hashToken("token")
	m.mu.Lock()
	m.identityCache[hash].validatedAt = time.Now().Add(-2 * m.config.IdentityCacheTTL)
	m.mu.Unlock()
	if _, _, _, err := m.Validate(context.Background(), "token", ""); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("validation calls after TTL expiry = %d", calls.Load())
	}

	m.mu.Lock()
	m.identityCache[hash].lastActivity = time.Now().Add(-2 * m.config.IdleTimeout)
	m.mu.Unlock()
	if _, _, _, err := m.Validate(context.Background(), "token", ""); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("validation calls after idle expiry = %d", calls.Load())
	}
}

func TestEvictExpiredRemovesIdentityAndUserIndex(t *testing.T) {
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		return &authapp.SessionValidateResponse{User: &authapp.Identity{ID: 11}}, nil
	}, nil, ReturnRoleError)
	if _, _, _, err := m.Validate(context.Background(), "expired", ""); err != nil {
		t.Fatal(err)
	}
	m.evictExpired(time.Now().Add(2 * m.config.MaxAge))
	if m.HasSession("expired") || len(m.ActiveSessionIDs(11)) != 0 {
		t.Fatalf("expired session remained indexed: %v", m.ActiveSessionIDs(11))
	}
}

func TestRoleCacheInvalidationAndProviderRecovery(t *testing.T) {
	var calls atomic.Int32
	failing := atomic.Bool{}
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		return &authapp.SessionValidateResponse{User: &authapp.Identity{ID: 9}}, nil
	}, func(context.Context, int64) ([]string, error) {
		calls.Add(1)
		if failing.Load() {
			return nil, errors.New("roles unavailable")
		}
		return []string{"admin"}, nil
	}, ReturnRoleError)

	if user, _, _, err := m.Validate(context.Background(), "token", ""); err != nil || !user.HasRole("admin") {
		t.Fatalf("initial Validate() = %+v, %v", user, err)
	}
	failing.Store(true)
	if user, _, _, err := m.Validate(context.Background(), "token", ""); err != nil || !user.HasRole("admin") {
		t.Fatalf("cached roles did not survive provider failure: %+v, %v", user, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cached role calls = %d", calls.Load())
	}

	failing.Store(false)
	m.InvalidateRoles(9)
	roles, err := m.RefreshRoles(context.Background(), 9)
	if err != nil || len(roles) != 1 || roles[0] != "admin" || calls.Load() != 2 {
		t.Fatalf("RefreshRoles() = %v, %v, calls=%d", roles, err, calls.Load())
	}
}

func TestEnsureKnownCreatesAnonymousOnlyForUnknownRemoteToken(t *testing.T) {
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		return &authapp.SessionValidateResponse{}, nil
	}, nil, ReturnRoleError)
	session, refresh, err := m.EnsureKnown(context.Background(), "unknown", "")
	if err != nil || session == "" || refresh != "" || !m.HasSession(session) {
		t.Fatalf("EnsureKnown() = %q, %q, %v", session, refresh, err)
	}
	if m.HasSession("unknown") {
		t.Fatal("unknown remote token was cached")
	}
}

func TestLifecycleGuards(t *testing.T) {
	m := testManager(t, func(context.Context, string, string) (*authapp.SessionValidateResponse, error) {
		return &authapp.SessionValidateResponse{}, nil
	}, nil, ReturnRoleError)
	userID := int64(1)
	if _, err := m.Create(context.Background(), &userID); err == nil {
		t.Fatal("authenticated local session creation should be rejected")
	}
	if _, err := m.SetCsrfToken("missing"); err == nil {
		t.Fatal("setting CSRF on an unknown session should fail")
	}
	m.Stop()
	m.Stop()
}
