package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
