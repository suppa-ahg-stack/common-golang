package internalapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// MaxRequestBodyBytes limits the size of incoming JSON request bodies.
const MaxRequestBodyBytes = 1 << 20 // 1 MiB

// ErrorResponse is the standard error envelope for internal APIs.
type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// ParseJSON reads and decodes a single JSON object from the request body.
// It rejects unknown fields and bodies larger than MaxRequestBodyBytes.
func ParseJSON(r *http.Request, dst any) error {
	body := http.MaxBytesReader(nil, r.Body, MaxRequestBodyBytes)
	defer body.Close()

	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError writes a structured error response and includes the request ID
// from the context when available.
func WriteError(ctx context.Context, w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: RequestIDFromContext(ctx),
	})
}
