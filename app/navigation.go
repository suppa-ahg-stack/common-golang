package app

import (
	"context"
	"net/http"
	"strings"

	"suppa-ahg-stack/common-golang/serverutil"
)

type AuthChecker func(w http.ResponseWriter, r *http.Request, route serverutil.PageRoute) (context.Context, bool)

func (a *App[TConfig, TQueries, TSessionService, TSseNames]) NavigationHandler(authChecker AuthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		route, ok := a.Routes[path]
		if !ok && !strings.HasSuffix(path, "/") {
			path = path + "/"
			route, ok = a.Routes[path]
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

		a.PublishFragmentForPath(path, route.TargetSelector, r)
		w.WriteHeader(http.StatusAccepted)
	}
}
