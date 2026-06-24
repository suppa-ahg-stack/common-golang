package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"suppa-ahg-stack/common-golang/serverutil/internalapi"
)

// HandlerDeps contains the dependencies required by the notification HTTP handlers.
type HandlerDeps struct {
	Service       *Service
	CurrentUserID func(r *http.Request) (int64, bool)
	SessionID     func(r *http.Request) (string, bool)
	CsrfChecker   func(r *http.Request, csrf, sessionID string) (bool, error)
}

// RegisterHandlers registers the notification REST endpoints on the provided mux.
func RegisterHandlers(mux *http.ServeMux, deps HandlerDeps) {
	mux.Handle("GET /notifications/summary", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleSummary(w, r, deps)
	}))
	mux.Handle("GET /notifications", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleList(w, r, deps)
	}))
	mux.Handle("POST /notifications/{id}/read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleMarkRead(w, r, deps)
	}))
	mux.Handle("POST /notifications/read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleMarkReadMany(w, r, deps)
	}))
}

// HandleSummary handles GET /notifications/summary.
func HandleSummary(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	userID, ok := deps.CurrentUserID(r)
	if !ok {
		internalapi.WriteError(r.Context(), w, http.StatusUnauthorized, "unauthorized", "user not authenticated")
		return
	}

	summary, err := deps.Service.UnreadCount(r.Context(), userID)
	if err != nil {
		internalapi.WriteError(r.Context(), w, http.StatusInternalServerError, "internal_error", "failed to load summary")
		return
	}

	internalapi.WriteJSON(w, http.StatusOK, summary)
}

// HandleList handles GET /notifications.
func HandleList(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	userID, ok := deps.CurrentUserID(r)
	if !ok {
		internalapi.WriteError(r.Context(), w, http.StatusUnauthorized, "unauthorized", "user not authenticated")
		return
	}

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	opts := ListOptions{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	page, err := deps.Service.List(r.Context(), userID, opts)
	if err != nil {
		if errors.Is(err, ErrInvalidCursor) {
			internalapi.WriteError(r.Context(), w, http.StatusBadRequest, "bad_request", "invalid cursor")
			return
		}
		internalapi.WriteError(r.Context(), w, http.StatusInternalServerError, "internal_error", "failed to load notifications")
		return
	}

	internalapi.WriteJSON(w, http.StatusOK, page)
}

// HandleMarkRead handles POST /notifications/{id}/read.
func HandleMarkRead(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	userID, ok := deps.CurrentUserID(r)
	if !ok {
		internalapi.WriteError(r.Context(), w, http.StatusUnauthorized, "unauthorized", "user not authenticated")
		return
	}

	sessionID, ok := deps.SessionID(r)
	if !ok {
		internalapi.WriteError(r.Context(), w, http.StatusUnauthorized, "unauthorized", "session not found")
		return
	}

	var body struct {
		Csrf string `json:"csrf"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		internalapi.WriteError(r.Context(), w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	csrf := body.Csrf
	if csrf == "" {
		internalapi.WriteError(r.Context(), w, http.StatusBadRequest, "bad_request", "missing csrf token")
		return
	}

	valid, err := deps.CsrfChecker(r, csrf, sessionID)
	if err != nil || !valid {
		internalapi.WriteError(r.Context(), w, http.StatusForbidden, "forbidden", "invalid csrf token")
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		internalapi.WriteError(r.Context(), w, http.StatusBadRequest, "bad_request", "invalid notification id")
		return
	}

	if err := deps.Service.MarkRead(r.Context(), id, userID); err != nil {
		internalapi.WriteError(r.Context(), w, http.StatusInternalServerError, "internal_error", "failed to mark as read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleMarkReadMany handles POST /notifications/read (bulk).
func HandleMarkReadMany(w http.ResponseWriter, r *http.Request, deps HandlerDeps) {
	userID, ok := deps.CurrentUserID(r)
	if !ok {
		internalapi.WriteError(r.Context(), w, http.StatusUnauthorized, "unauthorized", "user not authenticated")
		return
	}

	sessionID, ok := deps.SessionID(r)
	if !ok {
		internalapi.WriteError(r.Context(), w, http.StatusUnauthorized, "unauthorized", "session not found")
		return
	}

	var body struct {
		Csrf string  `json:"csrf"`
		IDs  []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		internalapi.WriteError(r.Context(), w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	if body.Csrf == "" {
		internalapi.WriteError(r.Context(), w, http.StatusBadRequest, "bad_request", "missing csrf token")
		return
	}
	if len(body.IDs) == 0 {
		internalapi.WriteError(r.Context(), w, http.StatusBadRequest, "bad_request", "missing notification ids")
		return
	}

	valid, err := deps.CsrfChecker(r, body.Csrf, sessionID)
	if err != nil || !valid {
		internalapi.WriteError(r.Context(), w, http.StatusForbidden, "forbidden", "invalid csrf token")
		return
	}

	if err := deps.Service.MarkReadMany(r.Context(), body.IDs, userID); err != nil {
		internalapi.WriteError(r.Context(), w, http.StatusInternalServerError, "internal_error", "failed to mark as read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// NoopPublisher is a Publisher that does nothing. Useful for tests.
type NoopPublisher struct{}

func (NoopPublisher) PublishNotificationCreated(_ context.Context, _ string, _ NotificationCreatedEvent) error {
	return nil
}

func (NoopPublisher) PublishNotificationRead(_ context.Context, _ string, _ NotificationReadEvent) error {
	return nil
}
