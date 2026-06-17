package app

import (
	"context"
	"net/http"
	"net/url"
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

// NavigationHandlerWithPlanner is the planner-aware navigation handler.
// It supports dynamic route patterns (e.g. /organisations/{id}/apps/) and refreshes
// both page content and layout fragments according to the FragmentPlanner.
func (a *App[TConfig, TQueries, TSessionService, TSseNames]) NavigationHandlerWithPlanner(authChecker AuthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		_, route, ok := MatchRoute(a.Routes, path)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		ctx := r.Context()
		if authChecker != nil {
			var ok bool
			ctx, ok = authChecker(w, r, route)
			if !ok {
				return
			}
		}

		ctx = WithTargetPath(ctx, path)
		r = r.WithContext(ctx)

		// Use a normalized path for the SSE event URL so static routes always
		// publish with their canonical trailing-slash form (e.g. /login/).
		// The raw path (including query string) is kept in context for renderers.
		publishPath := path
		if u, err := url.Parse(path); err == nil && u.RawQuery == "" && !strings.HasSuffix(u.Path, "/") {
			publishPath = u.Path + "/"
		}

		a.PublishFragmentScope(publishPath, RootsForPolicy(RefreshPageAndNavbar), r)
		w.WriteHeader(http.StatusAccepted)
	}
}
