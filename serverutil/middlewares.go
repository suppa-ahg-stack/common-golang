package serverutil

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"suppa-ahg-stack/common-golang/logger"
)

func EnsureSessionMiddleWare(next http.Handler, sessionName string, secure bool, logger *logger.FileLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := r.Cookie(sessionName)

		if errors.Is(err, http.ErrNoCookie) {
			sessionID, err := GenerateSessionID()
			if err != nil {
				logger.Error(fmt.Sprintf("EnsureSession: couldn't generate session id, %v", err))
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			http.SetCookie(w, &http.Cookie{
				Name:     sessionName,
				Value:    sessionID,
				Path:     "/",
				HttpOnly: true,
				Secure:   secure,
				SameSite: http.SameSiteLaxMode,
			})
		} else if err != nil {
			// malformed cookie or parsing issue
			logger.Error(fmt.Sprintf("EnsureSession: bad cookie, %v", err))
			http.Error(w, "bad cookie", http.StatusBadRequest)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func GenerateSessionID() (string, error) {
	// 32 bytes = 256 bits entropy
	b := make([]byte, 32)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// URL-safe, no padding
	return base64.RawURLEncoding.EncodeToString(b), nil
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
			"script-src 'nonce-" + nonce + "'; " +
			"style-src 'self'; " +
			"style-src-elem 'self'; " +
			"style-src-attr 'unsafe-inline'; " +
			"connect-src 'self'; " +
			"img-src 'self'; " +
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
