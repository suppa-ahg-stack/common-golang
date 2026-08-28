// Package authsession provides the shared application-side lifecycle for
// auth_app sessions: bounded identity caching, fresh validation for mutations,
// role resolution, token rotation, anonymous sessions and CSRF tokens.
package authsession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"suppa-ahg-stack/common-golang/authapp"
	"suppa-ahg-stack/common-golang/serverutil"
)

type Validator interface {
	ValidateSession(ctx context.Context, sessionToken, refreshToken string) (*authapp.SessionValidateResponse, error)
}

type RoleProvider func(ctx context.Context, userID int64) ([]string, error)

type Logger interface {
	Error(msg string, keysAndValues ...any)
}

type RoleErrorPolicy uint8

const (
	// ReturnRoleError rejects authentication when roles cannot be resolved.
	ReturnRoleError RoleErrorPolicy = iota
	// DenyRolesOnError returns an authenticated identity without roles. Role
	// gates therefore fail closed while public authenticated pages stay usable.
	DenyRolesOnError
)

type Config struct {
	MaxAge           time.Duration
	IdleTimeout      time.Duration
	IdentityCacheTTL time.Duration
	CSRFMaxAge       time.Duration
	RoleCacheTTL     time.Duration
	RoleErrorPolicy  RoleErrorPolicy
}

func (c Config) normalized() Config {
	if c.MaxAge <= 0 {
		c.MaxAge = 24 * time.Hour
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = c.MaxAge
	}
	if c.IdentityCacheTTL <= 0 || c.IdentityCacheTTL > c.MaxAge {
		c.IdentityCacheTTL = c.MaxAge
	}
	if c.CSRFMaxAge <= 0 {
		c.CSRFMaxAge = 20 * time.Minute
	}
	if c.RoleCacheTTL <= 0 {
		c.RoleCacheTTL = 2 * time.Minute
	}
	return c
}

type User struct {
	ID                 int64
	Email              string
	FullName           string
	Status             string
	PasswordMustChange bool
	TotpToken          string
	MfaLastUsedAt      time.Time
	Roles              []string
}

func (u *User) HasRole(role string) bool {
	return u != nil && slices.Contains(u.Roles, role)
}

func (u *User) HasAnyRole(roles ...string) bool {
	if u == nil {
		return false
	}
	for _, role := range roles {
		if u.HasRole(role) {
			return true
		}
	}
	return false
}

type csrfToken struct {
	value     string
	expiresAt time.Time
}

type identityCacheEntry struct {
	identity     *authapp.Identity
	token        string
	validatedAt  time.Time
	lastActivity time.Time
	expiresAt    time.Time
	csrf         *csrfToken
}

type roleCacheEntry struct {
	roles     []string
	expiresAt time.Time
}

type validationResult struct {
	user       *User
	newSession string
	newRefresh string
}

type Manager struct {
	validator    Validator
	roleProvider RoleProvider
	config       Config
	logger       Logger

	mu            sync.RWMutex
	identityCache map[string]*identityCacheEntry
	userSessions  map[int64]map[string]struct{}
	roleCache     map[int64]*roleCacheEntry
	validations   singleflight.Group
	stopCh        chan struct{}
	stopOnce      sync.Once
}

func NewManager(validator Validator, roleProvider RoleProvider, config Config, logger Logger) *Manager {
	if validator == nil {
		panic("authsession: validator is required")
	}
	config = config.normalized()
	m := &Manager{
		validator:     validator,
		roleProvider:  roleProvider,
		config:        config,
		logger:        logger,
		identityCache: make(map[string]*identityCacheEntry),
		userSessions:  make(map[int64]map[string]struct{}),
		roleCache:     make(map[int64]*roleCacheEntry),
		stopCh:        make(chan struct{}),
	}
	go m.cleanupCache()
	return m
}

func (m *Manager) Stop() {
	m.stopOnce.Do(func() { close(m.stopCh) })
}

// CreateAnonymous creates a locally cached anonymous session. Authenticated
// sessions are created and rotated only by auth_app.
func (m *Manager) CreateAnonymous() (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	m.setIdentity(hashToken(token), token, 0, &identityCacheEntry{
		lastActivity: now,
		expiresAt:    now.Add(m.config.MaxAge),
	})
	return token, nil
}

// Create is retained as the application session-service interface. A non-nil
// user ID is rejected because only auth_app may create authenticated sessions.
func (m *Manager) Create(_ context.Context, userID *int64) (string, error) {
	if userID != nil {
		return "", errors.New("authsession: authenticated sessions must be created by auth_app")
	}
	return m.CreateAnonymous()
}

func (m *Manager) Validate(ctx context.Context, sessionToken, refreshToken string) (*User, string, string, error) {
	return m.validate(ctx, sessionToken, refreshToken, false)
}

// ValidateFresh bypasses the identity cache and is intended for mutations.
func (m *Manager) ValidateFresh(ctx context.Context, sessionToken, refreshToken string) (*User, string, string, error) {
	return m.validate(ctx, sessionToken, refreshToken, true)
}

func (m *Manager) validate(ctx context.Context, sessionToken, refreshToken string, fresh bool) (*User, string, string, error) {
	if sessionToken == "" {
		return nil, "", "", errors.New("empty token")
	}
	hash := hashToken(sessionToken)
	now := time.Now()
	if !fresh {
		if user, ok, err := m.cachedUser(ctx, hash, now); ok || err != nil {
			return user, "", "", err
		}
	} else {
		result, err := m.validateAndCache(ctx, sessionToken, refreshToken, hash, now)
		return result.user, result.newSession, result.newRefresh, err
	}

	// Concurrent cache misses for the same token share one auth_app request.
	value, err, _ := m.validations.Do(hash, func() (any, error) {
		if user, ok, err := m.cachedUser(ctx, hash, time.Now()); ok || err != nil {
			return validationResult{user: user}, err
		}
		return m.validateAndCache(ctx, sessionToken, refreshToken, hash, time.Now())
	})
	if err != nil {
		return nil, "", "", err
	}
	result := value.(validationResult)
	return result.user, result.newSession, result.newRefresh, nil
}

func (m *Manager) cachedUser(ctx context.Context, hash string, now time.Time) (*User, bool, error) {
	m.mu.RLock()
	entry, ok := m.identityCache[hash]
	if ok {
		entry = cloneIdentityEntry(entry)
	}
	m.mu.RUnlock()
	if !ok || !now.Before(entry.expiresAt) || now.Sub(entry.lastActivity) >= m.config.IdleTimeout || now.Sub(entry.validatedAt) >= m.config.IdentityCacheTTL {
		return nil, false, nil
	}
	user, err := m.buildUser(ctx, entry.identity)
	if err != nil {
		return nil, true, err
	}
	m.mu.Lock()
	if current, exists := m.identityCache[hash]; exists && now.Before(current.expiresAt) {
		current.lastActivity = now
	}
	m.mu.Unlock()
	return user, true, nil
}

func (m *Manager) validateAndCache(ctx context.Context, sessionToken, refreshToken, hash string, now time.Time) (validationResult, error) {
	resp, err := m.validator.ValidateSession(ctx, sessionToken, refreshToken)
	if err != nil {
		return validationResult{}, err
	}
	if resp.User == nil {
		m.removeIdentity(hash)
		return validationResult{}, nil
	}
	user, err := m.buildUser(ctx, resp.User)
	if err != nil {
		return validationResult{}, err
	}

	m.mu.RLock()
	var existingCSRF *csrfToken
	if old := m.identityCache[hash]; old != nil && old.csrf != nil {
		copy := *old.csrf
		existingCSRF = &copy
	}
	m.mu.RUnlock()

	entry := &identityCacheEntry{
		identity:     cloneIdentity(resp.User),
		validatedAt:  now,
		lastActivity: now,
		expiresAt:    now.Add(m.config.MaxAge),
		csrf:         existingCSRF,
	}
	m.setIdentity(hash, sessionToken, user.ID, entry)
	if resp.SessionToken != "" && resp.SessionToken != sessionToken {
		m.setIdentity(hashToken(resp.SessionToken), resp.SessionToken, user.ID, &identityCacheEntry{
			identity:     cloneIdentity(resp.User),
			validatedAt:  now,
			lastActivity: now,
			expiresAt:    now.Add(m.config.MaxAge),
			csrf:         existingCSRF,
		})
		m.removeIdentity(hash)
	}
	return validationResult{user: user, newSession: resp.SessionToken, newRefresh: resp.RefreshToken}, nil
}

func (m *Manager) buildUser(ctx context.Context, identity *authapp.Identity) (*User, error) {
	if identity == nil {
		return nil, nil
	}
	roles, err := m.resolveRoles(ctx, identity.ID)
	if err != nil {
		if m.config.RoleErrorPolicy == ReturnRoleError {
			return nil, err
		}
		if m.logger != nil {
			m.logger.Error("failed to resolve local roles", "user_id", identity.ID, "error", err)
		}
		roles = nil
	}
	return &User{
		ID:                 identity.ID,
		Email:              identity.Email,
		FullName:           identity.FullName,
		Status:             identity.Status,
		PasswordMustChange: identity.PasswordMustChange,
		Roles:              cloneStrings(roles),
	}, nil
}

func (m *Manager) resolveRoles(ctx context.Context, userID int64) ([]string, error) {
	now := time.Now()
	m.mu.RLock()
	provider := m.roleProvider
	entry, ok := m.roleCache[userID]
	if ok {
		entry = &roleCacheEntry{roles: cloneStrings(entry.roles), expiresAt: entry.expiresAt}
	}
	m.mu.RUnlock()
	if provider == nil {
		return nil, nil
	}
	if ok && now.Before(entry.expiresAt) {
		return entry.roles, nil
	}
	roles, err := provider(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles = cloneStrings(roles)
	m.mu.Lock()
	m.roleCache[userID] = &roleCacheEntry{roles: roles, expiresAt: now.Add(m.config.RoleCacheTTL)}
	m.mu.Unlock()
	return cloneStrings(roles), nil
}

func (m *Manager) ActiveSessionIDs(userID int64) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tokens := m.userSessions[userID]
	out := make([]string, 0, len(tokens))
	for token := range tokens {
		out = append(out, token)
	}
	slices.Sort(out)
	return out
}

func (m *Manager) HasSession(sessionToken string) bool {
	m.mu.RLock()
	_, ok := m.identityCache[hashToken(sessionToken)]
	m.mu.RUnlock()
	return ok
}

// EnsureKnown makes an existing cookie token usable by local CSRF/session
// middleware after a process restart. A valid remote identity is cached; an
// unknown remote token is replaced with a fresh anonymous session. Dependency
// failures are returned and never overwrite a potentially valid user cookie.
func (m *Manager) EnsureKnown(ctx context.Context, sessionToken, refreshToken string) (session, refresh string, err error) {
	if m.HasSession(sessionToken) {
		return "", "", nil
	}
	user, rotatedSession, rotatedRefresh, err := m.Validate(ctx, sessionToken, refreshToken)
	if err != nil {
		return "", "", err
	}
	if user != nil {
		return rotatedSession, rotatedRefresh, nil
	}
	anonymous, err := m.CreateAnonymous()
	return anonymous, "", err
}

func (m *Manager) InvalidateRoles(userID int64) {
	m.mu.Lock()
	delete(m.roleCache, userID)
	m.mu.Unlock()
}

func (m *Manager) InvalidateAllRoles() {
	m.mu.Lock()
	m.roleCache = make(map[int64]*roleCacheEntry)
	m.mu.Unlock()
}

func (m *Manager) RefreshRoles(ctx context.Context, userID int64) ([]string, error) {
	m.mu.RLock()
	provider := m.roleProvider
	m.mu.RUnlock()
	if provider == nil {
		return nil, errors.New("no role provider configured")
	}
	m.InvalidateRoles(userID)
	return m.resolveRoles(ctx, userID)
}

// SetRoleProvider replaces the provider and clears role state. It supports
// dependency reconfiguration and deterministic failure testing.
func (m *Manager) SetRoleProvider(provider RoleProvider) {
	m.mu.Lock()
	m.roleProvider = provider
	m.roleCache = make(map[int64]*roleCacheEntry)
	m.mu.Unlock()
}

func (m *Manager) Destroy(sessionToken string) {
	if sessionToken != "" {
		m.removeIdentity(hashToken(sessionToken))
	}
}

func (m *Manager) SetCsrfToken(sessionToken string) (string, error) {
	if sessionToken == "" {
		return "", errors.New("empty token")
	}
	value, err := generateToken()
	if err != nil {
		return "", err
	}
	hash := hashToken(sessionToken)
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.identityCache[hash]
	if entry == nil {
		return "", errors.New("session must exist to SET csrf token")
	}
	entry.csrf = &csrfToken{value: value, expiresAt: time.Now().Add(m.config.CSRFMaxAge)}
	return value, nil
}

func (m *Manager) GetCsrfToken(sessionToken string) (string, error) {
	m.mu.RLock()
	entry := m.identityCache[hashToken(sessionToken)]
	if entry != nil && entry.csrf != nil {
		entry = cloneIdentityEntry(entry)
	}
	m.mu.RUnlock()
	if entry == nil {
		return "", errors.New("session must exist to GET csrf token")
	}
	if entry.csrf == nil {
		return "", serverutil.CsrfErrors.NotFoundInSessionCache
	}
	if time.Now().After(entry.csrf.expiresAt) {
		return "", serverutil.CsrfErrors.Expired
	}
	return entry.csrf.value, nil
}

func (m *Manager) CheckCsrf(value, sessionToken string) (bool, error) {
	m.mu.RLock()
	entry := m.identityCache[hashToken(sessionToken)]
	if entry != nil && entry.csrf != nil {
		entry = cloneIdentityEntry(entry)
	}
	m.mu.RUnlock()
	if entry == nil {
		return false, serverutil.CsrfErrors.DoesntExistInCache
	}
	if entry.csrf == nil {
		return false, serverutil.CsrfErrors.NotFoundInSessionCache
	}
	if time.Now().After(entry.csrf.expiresAt) {
		return false, serverutil.CsrfErrors.Expired
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(entry.csrf.value)) == 1, nil
}

func (m *Manager) cleanupCache() {
	interval := min(m.config.IdleTimeout, time.Minute)
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.evictExpired(time.Now())
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) evictExpired(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for hash, entry := range m.identityCache {
		if !now.Before(entry.expiresAt) || now.Sub(entry.lastActivity) > m.config.IdleTimeout {
			m.removeIdentityLocked(hash)
		}
	}
	for userID, entry := range m.roleCache {
		if !now.Before(entry.expiresAt) {
			delete(m.roleCache, userID)
		}
	}
}

func (m *Manager) setIdentity(hash, token string, userID int64, entry *identityCacheEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeIdentityLocked(hash)
	entry.token = token
	m.identityCache[hash] = entry
	if userID != 0 {
		if m.userSessions[userID] == nil {
			m.userSessions[userID] = make(map[string]struct{})
		}
		m.userSessions[userID][token] = struct{}{}
	}
}

func (m *Manager) removeIdentity(hash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeIdentityLocked(hash)
}

func (m *Manager) removeIdentityLocked(hash string) {
	entry := m.identityCache[hash]
	if entry == nil {
		return
	}
	delete(m.identityCache, hash)
	if entry.identity == nil {
		return
	}
	delete(m.userSessions[entry.identity.ID], entry.token)
	if len(m.userSessions[entry.identity.ID]) == 0 {
		delete(m.userSessions, entry.identity.ID)
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}

func cloneIdentity(identity *authapp.Identity) *authapp.Identity {
	if identity == nil {
		return nil
	}
	copy := *identity
	return &copy
}

func cloneIdentityEntry(entry *identityCacheEntry) *identityCacheEntry {
	if entry == nil {
		return nil
	}
	copy := *entry
	copy.identity = cloneIdentity(entry.identity)
	if entry.csrf != nil {
		csrfCopy := *entry.csrf
		copy.csrf = &csrfCopy
	}
	return &copy
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}
