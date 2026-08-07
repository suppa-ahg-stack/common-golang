package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"suppa-ahg-stack/common-golang/logger"
	"suppa-ahg-stack/common-golang/serverutil"

	"github.com/a-h/templ"
)

type testConfig struct {
	LangCookieName string
	DefaultLang    string
	SessionName    string
}

func (c testConfig) GetLangCookieName() string { return c.LangCookieName }
func (c testConfig) GetAppDefaultLang() string { return c.DefaultLang }
func (c testConfig) GetSessionName() string    { return c.SessionName }

type testUser struct {
	roles []string
}

func (u testUser) HasAnyRole(roles ...string) bool {
	for _, r := range roles {
		for _, has := range u.roles {
			if r == has {
				return true
			}
		}
	}
	return false
}

func (u testUser) HasRole(role string) bool {
	return u.HasAnyRole(role)
}

type testCsrfChecker struct{}

func (testCsrfChecker) CheckCsrf(csrf, sessionToken string) (bool, error) {
	return csrf == "valid-csrf", nil
}

func newTestInteractionApp(t *testing.T) *App[testConfig, any, any, any] {
	t.Helper()
	l, err := logger.NewFileLogger(logger.LogConfig{
		Filename: filepath.Join(t.TempDir(), "test.log"),
		Level:    0,
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	return &App[testConfig, any, any, any]{
		Config: &testConfig{
			LangCookieName: "lang",
			DefaultLang:    "en",
			SessionName:    "session",
		},
		Logger: l,
		Routes: map[string]serverutil.PageRoute{
			"/organisations/": {
				PageContentFunc: func() templ.Component {
					return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
						_, err := w.Write([]byte("organisations content"))
						return err
					})
				},
				TargetSelector: "#page-content",
				RequiresAuth:   true,
				RequiresRoles:  []string{"organisation_manager"},
			},
			"/applications/": {
				PageContentFunc: func() templ.Component {
					return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
						_, err := w.Write([]byte("applications content"))
						return err
					})
				},
				TargetSelector: "#page-content",
				RequiresAuth:   true,
				RequiresRoles:  []string{"application_manager"},
			},
		},
	}
}

func currentTestUser(r *http.Request) any {
	return r.Context().Value(userContextKey{})
}

type userContextKey struct{}

func withTestUser(ctx context.Context, user any) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func pathAccessForTest(user any, path string) bool {
	u, ok := user.(testUser)
	if !ok {
		return false
	}
	switch {
	case strings.HasPrefix(path, "/organisations/"):
		return u.HasAnyRole("organisation_manager")
	case strings.HasPrefix(path, "/applications/"):
		return u.HasAnyRole("application_manager")
	}
	return false
}

func actionRolesForTest(action string) ([]string, bool) {
	switch action {
	case "create-organisation":
		return []string{"organisation_manager"}, true
	case "create-application":
		return []string{"application_manager"}, true
	}
	return nil, false
}

func TestInteractionHandlerWithPlanner_AllowsActionOnAllowedPath(t *testing.T) {
	a := newTestInteractionApp(t)
	invoked := false
	handler := func(w http.ResponseWriter, r *http.Request, data json.RawMessage, selector, path string) {
		invoked = true
		w.WriteHeader(http.StatusNoContent)
	}

	h := a.InteractionHandlerWithPlanner(
		nil,
		testCsrfChecker{},
		map[string]ActionHandlerWithPath{"create-organisation": handler},
		actionRolesForTest,
		nil,
		pathAccessForTest,
		currentTestUser,
		nil,
		nil,
		nil,
		nil,
	)

	body, _ := json.Marshal(UihPayload{
		Path:     "/organisations/",
		Selector: "#page-content",
		Data: mustJSON(map[string]any{
			"action": "create-organisation",
			"csrf":   "valid-csrf",
		}),
	})
	req := httptest.NewRequest(http.MethodPost, "/uih", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "session-token"})
	ctx := withTestUser(req.Context(), testUser{roles: []string{"organisation_manager"}})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if !invoked {
		t.Fatal("expected action handler to be invoked")
	}
}

func TestInteractionHandlerWithPlanner_ForbidsActionOnForbiddenPath(t *testing.T) {
	a := newTestInteractionApp(t)
	invoked := false
	handler := func(w http.ResponseWriter, r *http.Request, data json.RawMessage, selector, path string) {
		invoked = true
		w.WriteHeader(http.StatusNoContent)
	}

	h := a.InteractionHandlerWithPlanner(
		nil,
		testCsrfChecker{},
		map[string]ActionHandlerWithPath{"create-organisation": handler},
		actionRolesForTest,
		nil,
		pathAccessForTest,
		currentTestUser,
		nil,
		nil,
		nil,
		nil,
	)

	body, _ := json.Marshal(UihPayload{
		Path:     "/applications/",
		Selector: "#page-content",
		Data: mustJSON(map[string]any{
			"action": "create-organisation",
			"csrf":   "valid-csrf",
		}),
	})
	req := httptest.NewRequest(http.MethodPost, "/uih", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "session-token"})
	ctx := withTestUser(req.Context(), testUser{roles: []string{"organisation_manager"}})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	if invoked {
		t.Fatal("expected action handler not to be invoked")
	}
}

func TestInteractionHandlerWithPlanner_ForbidsNonActionReRenderOnForbiddenPath(t *testing.T) {
	a := newTestInteractionApp(t)

	h := a.InteractionHandlerWithPlanner(
		nil,
		testCsrfChecker{},
		nil,
		actionRolesForTest,
		nil,
		pathAccessForTest,
		currentTestUser,
		nil,
		nil,
		nil,
		nil,
	)

	body, _ := json.Marshal(UihPayload{
		Path:     "/applications/",
		Selector: "#page-content",
		Data:     json.RawMessage(`{}`),
	})
	req := httptest.NewRequest(http.MethodPost, "/uih", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "session-token"})
	ctx := withTestUser(req.Context(), testUser{roles: []string{"organisation_manager"}})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestInteractionHandlerWithPlanner_MfaCheckerBlocksAction(t *testing.T) {
	a := newTestInteractionApp(t)
	invoked := false
	handler := func(w http.ResponseWriter, r *http.Request, data json.RawMessage, selector, path string) {
		invoked = true
		w.WriteHeader(http.StatusNoContent)
	}

	mfaChecker := func(ctx context.Context, user any, action, path string) bool {
		return false
	}

	h := a.InteractionHandlerWithPlanner(
		nil,
		testCsrfChecker{},
		map[string]ActionHandlerWithPath{"create-organisation": handler},
		actionRolesForTest,
		nil,
		pathAccessForTest,
		currentTestUser,
		nil,
		nil,
		mfaChecker,
		nil,
	)

	body, _ := json.Marshal(UihPayload{
		Path:     "/organisations/",
		Selector: "#page-content",
		Data: mustJSON(map[string]any{
			"action": "create-organisation",
			"csrf":   "valid-csrf",
		}),
	})
	req := httptest.NewRequest(http.MethodPost, "/uih", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "session-token"})
	ctx := withTestUser(req.Context(), testUser{roles: []string{"organisation_manager"}})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if invoked {
		t.Fatal("expected action handler not to be invoked")
	}
}

func TestInteractionHandlerWithPlanner_MfaCheckerBlocksNonActionReRender(t *testing.T) {
	a := newTestInteractionApp(t)

	mfaChecker := func(ctx context.Context, user any, action, path string) bool {
		return false
	}

	h := a.InteractionHandlerWithPlanner(
		nil,
		testCsrfChecker{},
		nil,
		actionRolesForTest,
		nil,
		pathAccessForTest,
		currentTestUser,
		nil,
		nil,
		mfaChecker,
		nil,
	)

	body, _ := json.Marshal(UihPayload{
		Path:     "/organisations/",
		Selector: "#page-content",
		Data:     json.RawMessage(`{}`),
	})
	req := httptest.NewRequest(http.MethodPost, "/uih", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "session-token"})
	ctx := withTestUser(req.Context(), testUser{roles: []string{"organisation_manager"}})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
