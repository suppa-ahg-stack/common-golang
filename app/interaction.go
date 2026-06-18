package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/invopop/ctxi18n"
)

type ActionHandler func(w http.ResponseWriter, r *http.Request, data json.RawMessage, selector string)

type SessionValidator interface {
	Validate(ctx context.Context, sessionToken string) (any, error)
}

type CsrfChecker interface {
	CheckCsrf(csrf, sessionToken string) (bool, error)
}

type UihPayload struct {
	Path     string          `json:"path"`
	Selector string          `json:"selector"`
	Data     json.RawMessage `json:"data"`
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) InteractionHandler(
	sessionValidator SessionValidator,
	csrfChecker CsrfChecker,
	actionRegistry map[string]ActionHandler,
	authChecker AuthChecker,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload UihPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		if payload.Path == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Extract action from data
		var actionData struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal(payload.Data, &actionData); err != nil {
			actionData.Action = ""
		}

		cfg := a.getLangConfig()
		cookie, err := r.Cookie(cfg.GetSessionName())
		if err != nil {
			http.Error(w, "session required", http.StatusUnauthorized)
			return
		}

		// Validate session
		_, err = sessionValidator.Validate(r.Context(), cookie.Value)
		if err != nil {
			a.Logger.Error(fmt.Sprintf("InteractionHandler: session validation failed: %v", err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Route-based access control for non-action requests
		if actionData.Action == "" {
			route, ok := a.Routes[payload.Path]
			if !ok && !strings.HasSuffix(payload.Path, "/") {
				payload.Path = payload.Path + "/"
				route, ok = a.Routes[payload.Path]
			}
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if authChecker != nil {
				ctx, ok := authChecker(w, r, route)
				if !ok {
					return
				}
				r = r.WithContext(ctx)
			}
		}

		// Validate CSRF for actions
		if actionData.Action != "" && csrfChecker != nil {
			var csrfData struct {
				Csrf string `json:"csrf"`
			}
			if err := json.Unmarshal(payload.Data, &csrfData); err != nil || csrfData.Csrf == "" {
				http.Error(w, "csrf required", http.StatusBadRequest)
				return
			}

			ok, err := csrfChecker.CheckCsrf(csrfData.Csrf, cookie.Value)
			if err != nil {
				a.Logger.Error(fmt.Sprintf("InteractionHandler: csrf check failed: %v", err))
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if !ok {
				a.Logger.Warn("InteractionHandler: invalid csrf")
				http.Error(w, "", http.StatusUnauthorized)
				return
			}
		}

		// Route to action handler
		if handler, ok := actionRegistry[actionData.Action]; ok {
			handler(w, r, payload.Data, payload.Selector)
			return
		}

		// Default: re-render fragment
		if payload.Selector == "#page-content" {
			a.PublishFragmentForPath(payload.Path, payload.Selector, r)
		} else {
			a.PublishFragment(payload.Path, payload.Selector, r)
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

// ActionHandlerWithPath is an action handler that also receives the target path.
type ActionHandlerWithPath func(w http.ResponseWriter, r *http.Request, data json.RawMessage, selector, path string)

// ActionRoleMapper returns the roles required for an action, if any.
// The second return value is false when the action is unknown.
type ActionRoleMapper func(action string) ([]string, bool)

// PathAccessChecker returns true when the given user is allowed to access path.
type PathAccessChecker func(user any, path string) bool

// CurrentUserFunc extracts the authenticated user from the request context.
// It should return nil for unauthenticated requests.
type CurrentUserFunc func(r *http.Request) any

// PreActionHook runs after CSRF validation but before an action handler is invoked.
// It can rewrite the request context or reject the request.
type PreActionHook func(w http.ResponseWriter, r *http.Request, action, path string) (context.Context, bool)

// MfaChecker runs after context enrichment and before path/action access checks.
// It returns true when the request is allowed to proceed with the current MFA
// state. A nil MfaChecker means no MFA gate is enforced.
type MfaChecker func(ctx context.Context, user any, action, path string) bool

// ContextEnricher validates the session token and returns an enriched context.
// It is used when the request middleware does not already populate the context
// with the authenticated user.
type ContextEnricher func(ctx context.Context, sessionToken string) (context.Context, error)

// InteractionHooks are optional callbacks invoked on authorization or validation failures.
type InteractionHooks struct {
	OnAuthzDenied      func(ctx context.Context, user any, resource, reason string, details map[string]any, r *http.Request)
	OnValidationFailed func(ctx context.Context, user any, resource, reason string, details map[string]any, r *http.Request)
}

// InteractionHandlerWithPlanner is the planner-aware POST /uih dispatcher.
// It supports action role mapping, path-based access control for re-renders,
// and audit hooks. The configured FragmentPlanner is used for fragment refreshes;
// if none is configured, it falls back to legacy PublishFragmentForPath/PublishFragment.
func (a *App[TConfig, TQueries, TSessionService, TSseNames]) InteractionHandlerWithPlanner(
	sessionValidator SessionValidator,
	csrfChecker CsrfChecker,
	actionRegistry map[string]ActionHandlerWithPath,
	actionRoles ActionRoleMapper,
	authChecker AuthChecker,
	pathAccessChecker PathAccessChecker,
	currentUser CurrentUserFunc,
	contextEnricher ContextEnricher,
	preActionHook PreActionHook,
	mfaChecker MfaChecker,
	hooks *InteractionHooks,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		var payload UihPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(rec, "invalid body", http.StatusBadRequest)
			return
		}
		if payload.Path == "" {
			http.Error(rec, "bad request", http.StatusBadRequest)
			return
		}

		var actionData struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal(payload.Data, &actionData); err != nil {
			actionData.Action = ""
		}

		cfg := a.getConfig()
		locale := cfg.GetAppDefaultLang()
		if langCookie, err := r.Cookie(cfg.GetLangCookieName()); err == nil && langCookie != nil {
			locale = langCookie.Value
		}
		if ctx, err := ctxi18n.WithLocale(r.Context(), locale); err == nil {
			r = r.WithContext(ctx)
		}

		cookie, err := r.Cookie(cfg.GetSessionName())
		if err != nil {
			http.Error(rec, "session required", http.StatusUnauthorized)
			return
		}

		if sessionValidator != nil {
			_, err = sessionValidator.Validate(r.Context(), cookie.Value)
			if err != nil {
				a.Logger.Error(fmt.Sprintf("InteractionHandlerWithPlanner: session validation failed: %v", err))
				http.Error(rec, "internal error", http.StatusInternalServerError)
				return
			}
		}

		if contextEnricher != nil {
			ctx, err := contextEnricher(r.Context(), cookie.Value)
			if err != nil {
				a.Logger.Error(fmt.Sprintf("InteractionHandlerWithPlanner: context enrichment failed: %v", err))
				http.Error(rec, "internal error", http.StatusInternalServerError)
				return
			}
			r = r.WithContext(ctx)
		}

		user := currentUser(r)
		if user != nil && mfaChecker != nil && !mfaChecker(r.Context(), user, actionData.Action, payload.Path) {
			a.Logger.Warn(fmt.Sprintf("InteractionHandlerWithPlanner: MFA required for user on %s action=%s", payload.Path, actionData.Action))
			http.Error(rec, "MFA required", http.StatusUnauthorized)
			return
		}

		defer func() {
			if hooks == nil || user == nil || rec.status < 400 || rec.status >= 500 {
				return
			}
			resource := actionData.Action
			if resource == "" {
				resource = payload.Path
			}
			details := map[string]any{"path": payload.Path}
			switch rec.status {
			case http.StatusUnauthorized:
				if hooks.OnAuthzDenied != nil {
					hooks.OnAuthzDenied(r.Context(), user, resource, "unauthenticated or invalid csrf", details, r)
				}
			case http.StatusForbidden:
				if hooks.OnAuthzDenied != nil {
					hooks.OnAuthzDenied(r.Context(), user, resource, "missing required role", details, r)
				}
			case http.StatusBadRequest:
				if hooks.OnValidationFailed != nil {
					hooks.OnValidationFailed(r.Context(), user, resource, "invalid request", details, r)
				}
			}
		}()

		// Validate CSRF for actions
		if actionData.Action != "" && csrfChecker != nil {
			var csrfData struct {
				Csrf string `json:"csrf"`
			}
			if err := json.Unmarshal(payload.Data, &csrfData); err != nil || csrfData.Csrf == "" {
				http.Error(rec, "csrf required", http.StatusBadRequest)
				return
			}

			ok, err := csrfChecker.CheckCsrf(csrfData.Csrf, cookie.Value)
			if err != nil {
				a.Logger.Error(fmt.Sprintf("InteractionHandlerWithPlanner: csrf check failed: %v", err))
				http.Error(rec, "internal error", http.StatusInternalServerError)
				return
			}
			if !ok {
				a.Logger.Warn("InteractionHandlerWithPlanner: invalid csrf")
				http.Error(rec, "", http.StatusUnauthorized)
				return
			}
		}

		// Default re-render for the current path; require role for that path.
		if actionData.Action == "" {
			if pathAccessChecker != nil && !pathAccessChecker(user, payload.Path) {
				a.Logger.Warn(fmt.Sprintf("InteractionHandlerWithPlanner: forbidden re-render on %s", payload.Path))
				http.Error(rec, "forbidden", http.StatusForbidden)
				return
			}
			r = r.WithContext(WithTargetPath(r.Context(), payload.Path))
			a.PublishFragmentScope(payload.Path, RootsForPolicy(RefreshPageOnly), r)
			rec.WriteHeader(http.StatusAccepted)
			return
		}

		requiredRoles, ok := actionRoles(actionData.Action)
		if !ok {
			http.Error(rec, "unknown action", http.StatusBadRequest)
			return
		}

		if len(requiredRoles) > 0 {
			userAccess, ok := user.(interface{ HasAnyRole(...string) bool })
			if !ok || !userAccess.HasAnyRole(requiredRoles...) {
				a.Logger.Warn(fmt.Sprintf("InteractionHandlerWithPlanner: forbidden action %s", actionData.Action))
				http.Error(rec, "forbidden", http.StatusForbidden)
				return
			}
		}

		// Enforce path-based access control for action requests too. The action
		// handler will refresh/re-render payload.Path, so the caller must be
		// allowed to access that path.
		if pathAccessChecker != nil && !pathAccessChecker(user, payload.Path) {
			a.Logger.Warn(fmt.Sprintf("InteractionHandlerWithPlanner: forbidden action path %s", payload.Path))
			http.Error(rec, "forbidden", http.StatusForbidden)
			return
		}

		if preActionHook != nil {
			ctx, ok := preActionHook(rec, r, actionData.Action, payload.Path)
			if !ok {
				return
			}
			r = r.WithContext(ctx)
		}

		handler, ok := actionRegistry[actionData.Action]
		if !ok {
			http.Error(rec, "unknown action", http.StatusBadRequest)
			return
		}
		handler(rec, r, payload.Data, payload.Selector, payload.Path)
	}
}

// statusRecorder wraps an http.ResponseWriter to capture the written status code.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.written = true
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Write(b []byte) (int, error) {
	if !rec.written {
		rec.written = true
		rec.status = http.StatusOK
	}
	return rec.ResponseWriter.Write(b)
}

func (rec *statusRecorder) Flush() {
	if f, ok := rec.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
