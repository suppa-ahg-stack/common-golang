package serverutil

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"suppa-ahg-stack/common-golang/logger"
)

func EnsureSession(next http.Handler, sessionName string, secure bool, logger *logger.FileLogger) http.Handler {
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
