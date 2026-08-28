package serverutil

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/invopop/ctxi18n"
)

type LocaleLogger interface {
	Error(msg string, keysAndValues ...any)
}

// SetLang installs the locale selected by a cookie, creating the configured
// default cookie when it is absent.
func SetLang(next http.Handler, langCookie *http.Cookie, logger LocaleLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale := langCookie.Value
		cookie, err := r.Cookie(langCookie.Name)
		if !errors.Is(err, http.ErrNoCookie) && err != nil {
			if logger != nil {
				logger.Error("failed to read locale cookie", "error", err)
			}
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if cookie == nil {
			http.SetCookie(w, langCookie)
		} else {
			locale = cookie.Value
		}
		ctx, err := ctxi18n.WithLocale(r.Context(), locale)
		if err != nil {
			if logger != nil {
				logger.Error(fmt.Sprintf("failed to set locale %q", locale), "error", err)
			}
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
