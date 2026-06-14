package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/invopop/ctxi18n"
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
