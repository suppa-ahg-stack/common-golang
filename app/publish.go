package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/invopop/ctxi18n"
	"suppa-ahg-stack/common-golang/serverutil"
	"suppa-ahg-stack/common-golang/sse"
)

type ConfigWithSession interface {
	GetLangCookieName() string
	GetAppDefaultLang() string
	GetSessionName() string
}

type NavUpdateEvent struct {
	Selector string `json:"selector"`
	HTML     string `json:"html"`
	Path     string `json:"path"`
	Swap     string `json:"swap"`
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) getConfig() ConfigWithSession {
	cfg, ok := any(a.Config).(ConfigWithSession)
	if !ok {
		panic(fmt.Sprintf("Config type %T does not implement ConfigWithSession", a.Config))
	}
	return cfg
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) PublishFragmentForPath(path string, selector string, r *http.Request) {
	cfg := a.getConfig()
	route, ok := a.Routes[path]
	if !ok {
		a.Logger.Error(fmt.Sprintf("publishFragmentForPath: unknown path %s", path))
		return
	}

	targetSelector := selector
	if targetSelector == "" {
		targetSelector = route.TargetSelector
	}

	langCookie, err := r.Cookie(cfg.GetLangCookieName())
	if !errors.Is(err, http.ErrNoCookie) && err != nil {
		a.Logger.Error(fmt.Sprintf("Error while retrieving lang cookie with error: %v", err))
		return
	}

	locale := cfg.GetAppDefaultLang()
	if langCookie != nil {
		locale = langCookie.Value
	}

	ctx, err := ctxi18n.WithLocale(r.Context(), locale)
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Error while setting locale: %v", err))
		return
	}
	r = r.WithContext(ctx)
	var buf bytes.Buffer
	if err := route.PageContentFunc().Render(ctx, &buf); err != nil {
		a.Logger.Error(fmt.Sprintf("Failed to render fragment for %s: %v", path, err))
		return
	}

	cookie, err := r.Cookie(cfg.GetSessionName())
	sessionID := ""
	if err == nil {
		sessionID = cookie.Value
	}

	eventData, err := json.Marshal(NavUpdateEvent{
		Selector: targetSelector,
		HTML:     buf.String(),
		Path:     path,
		Swap:     "morph",
	})
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Failed to marshal nav event: %v", err))
		return
	}

	a.DomUpdateBroker.PublishToUser(sessionID, sse.Event{
		Type: "dom-update",
		Data: eventData,
	})

	for sel, component := range a.PageComponent[path] {
		if sel == targetSelector || !component.IsLayoutComponent() {
			continue
		}
		a.PublishFragment(path, sel, r)
	}
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) PublishFragment(path string, selector string, r *http.Request) {
	cfg := a.getConfig()
	component, ok := a.PageComponent[path][selector]
	if !ok {
		a.Logger.Error(fmt.Sprintf("publishFragment: unknown for path %s and selector %s", path, selector))
		return
	}

	targetSelector := selector
	if targetSelector == "" {
		targetSelector = component.GetTargetSelector()
	}

	langCookie, err := r.Cookie(cfg.GetLangCookieName())
	if !errors.Is(err, http.ErrNoCookie) && err != nil {
		a.Logger.Error(fmt.Sprintf("Error while retrieving lang cookie with error: %v", err))
		return
	}

	locale := cfg.GetAppDefaultLang()
	if langCookie != nil {
		locale = langCookie.Value
	}

	ctx, err := ctxi18n.WithLocale(r.Context(), locale)
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Error while setting locale: %v", err))
		return
	}
	r = r.WithContext(ctx)
	var buf bytes.Buffer
	if err := component.Render(r, ctx, &buf); err != nil {
		a.Logger.Error("Failed to render fragment for %s: %v", path, err)
		return
	}

	cookie, err := r.Cookie(cfg.GetSessionName())
	sessionID := ""
	if err == nil {
		sessionID = cookie.Value
	}

	eventData, err := json.Marshal(NavUpdateEvent{
		Selector: targetSelector,
		HTML:     buf.String(),
		Path:     path,
		Swap:     "morph",
	})
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Failed to marshal nav event: %v", err))
		return
	}

	a.DomUpdateBroker.PublishToUser(sessionID, sse.Event{
		Type: "dom-update",
		Data: eventData,
	})
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) PublishToast(message string, toastType string, duration int, r *http.Request) {
	cfg := a.getConfig()
	cookie, err := r.Cookie(cfg.GetSessionName())
	sessionID := ""
	if err == nil {
		sessionID = cookie.Value
	}

	eventData, err := json.Marshal(map[string]any{
		"message":  message,
		"type":     toastType,
		"duration": duration,
	})
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Failed to marshal toast event: %v", err))
		return
	}

	a.DomUpdateBroker.PublishToUser(sessionID, sse.Event{
		Type: "toast",
		Data: eventData,
	})
}

// PublishDomUpdate sends a single dom-update SSE event to the current user's stream.
func (a *App[TConfig, TQueries, TSessionService, TSseNames]) PublishDomUpdate(rawPath, selector, html string, r *http.Request) bool {
	cfg := a.getConfig()
	cookie, err := r.Cookie(cfg.GetSessionName())
	sessionID := ""
	if err == nil {
		sessionID = cookie.Value
	}

	eventData, err := json.Marshal(NavUpdateEvent{
		Selector: selector,
		HTML:     html,
		Path:     rawPath,
		Swap:     "morph",
	})
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Failed to marshal nav event: %v", err))
		return false
	}

	a.DomUpdateBroker.PublishToUser(sessionID, sse.Event{
		Type: "dom-update",
		Data: eventData,
	})
	return true
}

// PublishModalToast sends a toast event scoped to a modal container.
func (a *App[TConfig, TQueries, TSessionService, TSseNames]) PublishModalToast(message, toastType, duration, modalID string, r *http.Request) {
	cfg := a.getConfig()
	cookie, err := r.Cookie(cfg.GetSessionName())
	sessionID := ""
	if err == nil {
		sessionID = cookie.Value
	}

	eventData, err := json.Marshal(map[string]any{
		"message":  message,
		"type":     toastType,
		"duration": duration,
		"modal_id": modalID,
	})
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Failed to marshal modal toast event: %v", err))
		return
	}

	a.DomUpdateBroker.PublishToUser(sessionID, sse.Event{
		Type: "modal-toast",
		Data: eventData,
	})
}

// PublishFragmentScope renders and publishes a planned set of fragments for rawPath.
// It uses the configured FragmentPlanner; if none is configured, it falls back to
// publishing the route content only.
func (a *App[TConfig, TQueries, TSessionService, TSseNames]) PublishFragmentScope(rawPath string, roots []FragmentID, r *http.Request) bool {
	if a.FragmentPlanner == nil {
		a.PublishFragmentForPath(rawPath, "", r)
		return true
	}

	routeKey, route, ok := MatchRoute(a.Routes, rawPath)
	if !ok {
		a.Logger.Error(fmt.Sprintf("PublishFragmentScope: unknown path %s", rawPath))
		return false
	}

	r, ctx, err := a.buildFragmentRequest(rawPath, r)
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Error while setting locale: %v", err))
		return false
	}

	for _, fragment := range a.FragmentPlanner.Plan(routeKey, roots) {
		switch fragment.Kind {
		case FragmentRouteContent:
			if !a.publishRouteContent(rawPath, route, fragment.Selector, r, ctx) {
				return false
			}
		case FragmentPageComponent:
			if !a.publishPageComponent(rawPath, routeKey, fragment.Selector, r, ctx) {
				return false
			}
		case FragmentCustom:
			if !a.publishCustomFragment(rawPath, fragment, r, ctx) {
				return false
			}
		default:
			a.Logger.Error(fmt.Sprintf("PublishFragmentScope: unknown fragment kind for %s", fragment.ID))
			return false
		}
	}

	return true
}

// PublishFragmentForPathWithPlanner publishes the fragment for rawPath and selector
// using the FragmentPlanner. If no planner is configured, it falls back to the
// legacy PublishFragmentForPath behavior.
func (a *App[TConfig, TQueries, TSessionService, TSseNames]) PublishFragmentForPathWithPlanner(rawPath, selector string, r *http.Request) {
	if a.FragmentPlanner == nil {
		a.PublishFragmentForPath(rawPath, selector, r)
		return
	}

	routeKey, route, ok := MatchRoute(a.Routes, rawPath)
	if !ok {
		a.Logger.Error(fmt.Sprintf("PublishFragmentForPathWithPlanner: unknown path %s", rawPath))
		return
	}

	targetSelector := selector
	if targetSelector == "" {
		targetSelector = route.TargetSelector
	}

	if targetSelector == route.TargetSelector {
		a.PublishFragmentScope(rawPath, RootsForPolicy(RefreshPageOnly), r)
		return
	}

	fragment, ok := a.FragmentPlanner.FindBySelector(routeKey, targetSelector)
	if !ok {
		a.Logger.Error(fmt.Sprintf("PublishFragmentForPathWithPlanner: unknown selector %s for path %s", targetSelector, rawPath))
		return
	}

	a.PublishFragmentScope(rawPath, []FragmentID{fragment.ID}, r)
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) publishRouteContent(rawPath string, route serverutil.PageRoute, selector string, r *http.Request, ctx context.Context) bool {
	var buf bytes.Buffer
	if err := route.PageContentFunc().Render(ctx, &buf); err != nil {
		a.Logger.Error(fmt.Sprintf("Failed to render fragment for %s: %v", rawPath, err))
		return false
	}

	return a.PublishDomUpdate(rawPath, selector, buf.String(), r)
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) publishPageComponent(rawPath, routeKey, selector string, r *http.Request, ctx context.Context) bool {
	components, ok := a.PageComponent[routeKey]
	if !ok {
		return false
	}
	component, ok := components[selector]
	if !ok {
		a.Logger.Error(fmt.Sprintf("publishPageComponent: unknown for path %s and selector %s", routeKey, selector))
		return false
	}

	var buf bytes.Buffer
	if err := component.Render(r, ctx, &buf); err != nil {
		a.Logger.Error(fmt.Sprintf("publishPageComponent: failed to render %s: %v", selector, err))
		return false
	}

	return a.PublishDomUpdate(rawPath, selector, buf.String(), r)
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) publishCustomFragment(rawPath string, fragment FragmentSpec, r *http.Request, ctx context.Context) bool {
	if fragment.RenderFunc == nil {
		a.Logger.Error(fmt.Sprintf("publishCustomFragment: missing render func for %s", fragment.ID))
		return false
	}

	var buf bytes.Buffer
	if err := fragment.RenderFunc(r, ctx, &buf); err != nil {
		a.Logger.Error(fmt.Sprintf("publishCustomFragment: failed to render %s: %v", fragment.ID, err))
		return false
	}

	return a.PublishDomUpdate(rawPath, fragment.Selector, buf.String(), r)
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) buildFragmentRequest(rawPath string, r *http.Request) (*http.Request, context.Context, error) {
	cfg := a.getConfig()
	locale := cfg.GetAppDefaultLang()
	if langCookie, err := r.Cookie(cfg.GetLangCookieName()); err == nil && langCookie != nil {
		locale = langCookie.Value
	}

	ctx, err := ctxi18n.WithLocale(r.Context(), locale)
	if err != nil {
		return nil, nil, err
	}

	if TargetPathFromContext(ctx) == "" {
		ctx = WithTargetPath(ctx, rawPath)
	}
	return r.WithContext(ctx), ctx, nil
}
