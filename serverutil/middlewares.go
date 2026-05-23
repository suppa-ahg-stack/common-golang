package serverutil

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

func ClearSessionCookie(w http.ResponseWriter, sessionName string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func ClearCsrfCookie(w http.ResponseWriter, csrfCookieName string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

type contextKey struct{}

var NonceKey = contextKey{}

func CspMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		nonce := base64.StdEncoding.EncodeToString(b)

		csp := "default-src 'self'; " +
			"script-src 'nonce-" + nonce + "' 'strict-dynamic'; " +
			"style-src 'self'; " +
			"style-src-elem 'self'; " +
			"style-src-attr 'unsafe-inline'; " +
			"connect-src 'self'; " +
			"img-src 'self' data:; " +
			"font-src 'self'; " +
			"base-uri 'self'; " +
			"form-action 'self'; " +
			"frame-ancestors 'none'"

		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		ctx := context.WithValue(r.Context(), NonceKey, nonce)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
