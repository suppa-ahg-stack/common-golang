package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/invopop/ctxi18n"
	"suppa-ahg-stack/common-golang/serverutil"
)

type ConfigWithLangs interface {
	ConfigWithSession
	GetAvailableLangs() string
	GetSessionDomain() string
}

type CsrfSessionManager interface {
	GetCsrfToken(sessionToken string) (string, error)
	SetCsrfToken(sessionToken string) (string, error)
}

type SetLangRequest struct {
	Lang string `json:"lang"`
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) getLangConfig() ConfigWithLangs {
	cfg, ok := any(a.Config).(ConfigWithLangs)
	if !ok {
		panic(fmt.Sprintf("Config type %T does not implement ConfigWithLangs", a.Config))
	}
	return cfg
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) HandleGetCsrf(w http.ResponseWriter, r *http.Request, sm CsrfSessionManager) {
	cfg := a.getLangConfig()
	cookie, err := r.Cookie(cfg.GetSessionName())
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Failed to get session token from cookies with error: %v", err))
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	csrf, err := sm.GetCsrfToken(cookie.Value)
	if errors.Is(err, serverutil.CsrfErrors.NotFoundInSessionCache) || errors.Is(err, serverutil.CsrfErrors.Expired) {
		csrf, err = sm.SetCsrfToken(cookie.Value)
		if err != nil {
			a.Logger.Error(fmt.Sprintf("Failed to set csrf token with error: %v", err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		a.Logger.Error(fmt.Sprintf("Failed to get csrf token with error: %v", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(csrf))
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Failed to write bytes for csrf token with error: %v", err))
	}
}

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) HandleSetLang(w http.ResponseWriter, r *http.Request) {
	cfg := a.getLangConfig()
	var req SetLangRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	allowedLangs := strings.Split(cfg.GetAvailableLangs(), ",")
	allowed := make(map[string]bool, len(allowedLangs))
	for _, lang := range allowedLangs {
		allowed[strings.TrimSpace(lang)] = true
	}

	if !allowed[req.Lang] {
		http.Error(w, "invalid language", http.StatusBadRequest)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cfg.GetLangCookieName(),
		Value:    req.Lang,
		Path:     "/",
		Domain:   cfg.GetSessionDomain(),
		MaxAge:   31536000,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	ctx, err := ctxi18n.WithLocale(r.Context(), req.Lang)
	if err != nil {
		a.Logger.Error(fmt.Sprintf("Error while setting locale with error: %v. Had locale from cookie: %s", err, req.Lang))
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r.WithContext(ctx), "/", http.StatusSeeOther)
}
