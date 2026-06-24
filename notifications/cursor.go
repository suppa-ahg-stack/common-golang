package notifications

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidCursor is returned when a cursor cannot be decoded.
var ErrInvalidCursor = errors.New("invalid cursor")

// cursorPayload is the decoded cursor content.
type cursorPayload struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

// EncodeCursor encodes a cursor into an opaque base64 string.
func EncodeCursor(createdAt time.Time, id int64) string {
	p := cursorPayload{CreatedAt: createdAt, ID: id}
	b, _ := json.Marshal(p)
	return base64.URLEncoding.EncodeToString(b)
}

// DecodeCursor decodes a cursor string. It returns ErrInvalidCursor if the cursor is invalid.
func DecodeCursor(cursor string) (time.Time, int64, error) {
	if cursor == "" {
		return time.Time{}, 0, fmt.Errorf("%w: empty", ErrInvalidCursor)
	}
	b, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("%w: invalid encoding", ErrInvalidCursor)
	}
	var p cursorPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return time.Time{}, 0, fmt.Errorf("%w: invalid payload", ErrInvalidCursor)
	}
	return p.CreatedAt, p.ID, nil
}
