package app

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"suppa-ahg-stack/common-golang/serverutil"
)

type targetPathKey struct{}

// WithTargetPath stores the original request path (including query string) in the context.
// Content renderers use this to read pagination, search, or edit selection state.
func WithTargetPath(ctx context.Context, rawPath string) context.Context {
	return context.WithValue(ctx, targetPathKey{}, rawPath)
}

// TargetPathFromContext returns the original request path stored by WithTargetPath.
func TargetPathFromContext(ctx context.Context) string {
	raw, _ := ctx.Value(targetPathKey{}).(string)
	return raw
}

// TargetPathQuery extracts a query parameter from the original request path in context.
func TargetPathQuery(ctx context.Context, key string) string {
	raw := TargetPathFromContext(ctx)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Query().Get(key)
}

// TargetPathRawQuery returns the raw query string of the original request path in context.
func TargetPathRawQuery(ctx context.Context) string {
	raw := TargetPathFromContext(ctx)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.RawQuery
}

// MatchRoute looks up a request path in the SSE route table. It supports static routes
// and simple {param} placeholders for numeric segments (e.g. /organisations/{id}/apps/).
// It returns the route table key (pattern), the matched route, and whether a match was found.
func MatchRoute(routes map[string]serverutil.PageRoute, path string) (string, serverutil.PageRoute, bool) {
	if u, err := url.Parse(path); err == nil {
		path = u.Path
	}


	if route, ok := routes[path]; ok {
		return path, route, true
	}

	// Try with a trailing slash, then without.
	candidates := []string{path}
	if !strings.HasSuffix(path, "/") {
		candidates = append(candidates, path+"/")
	}
	if strings.HasSuffix(path, "/") {
		candidates = append(candidates, strings.TrimSuffix(path, "/"))
	}

	for _, candidate := range candidates {
		if route, ok := routes[candidate]; ok {
			return candidate, route, true
		}
	}

	// Dynamic route matching using {param} placeholders for numeric segments.
	for _, candidate := range candidates {
		reqParts := strings.Split(strings.Trim(candidate, "/"), "/")

		for key, route := range routes {
			trimmedKey := strings.TrimSuffix(key, "/")
			keyParts := strings.Split(strings.Trim(trimmedKey, "/"), "/")
			if len(keyParts) != len(reqParts) {
				continue
			}

			matched := true
			for i, kp := range keyParts {
				if kp == reqParts[i] {
					continue
				}
				if strings.HasPrefix(kp, "{") && strings.HasSuffix(kp, "}") {
					if _, err := strconv.ParseInt(reqParts[i], 10, 64); err != nil {
						matched = false
						break
					}
					continue
				}
				matched = false
				break
			}
			if matched {
				return key, route, true
			}
		}
	}

	return "", serverutil.PageRoute{}, false
}
