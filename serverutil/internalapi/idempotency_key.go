package internalapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	// MinIdempotencyKeyLen is the minimum length of a valid idempotency key.
	MinIdempotencyKeyLen = 16
	// MaxIdempotencyKeyLen is the maximum length of a valid idempotency key.
	MaxIdempotencyKeyLen = 255
)

// ErrInvalidIdempotencyKey is returned when an idempotency key does not satisfy
// the shared key contract.
var ErrInvalidIdempotencyKey = errors.New("invalid idempotency key")

// ValidateIdempotencyKey enforces the shared key contract:
//   - length between MinIdempotencyKeyLen and MaxIdempotencyKeyLen bytes,
//   - every byte is a visible ASCII character (0x21–0x7E),
//   - no trimming or normalization is performed.
//
// The raw key is never included in the returned error.
func ValidateIdempotencyKey(key string) error {
	n := len(key)
	if n < MinIdempotencyKeyLen || n > MaxIdempotencyKeyLen {
		return fmt.Errorf("%w: length %d not in [%d,%d]", ErrInvalidIdempotencyKey, n, MinIdempotencyKeyLen, MaxIdempotencyKeyLen)
	}
	for i := 0; i < n; i++ {
		c := key[i]
		if c < 0x21 || c > 0x7E {
			return fmt.Errorf("%w: byte 0x%02X at position %d is outside visible ASCII", ErrInvalidIdempotencyKey, c, i)
		}
	}
	return nil
}

// NewIdempotencyKey generates a new 16-byte random idempotency key encoded as
// 32 lowercase hexadecimal characters.
func NewIdempotencyKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate idempotency key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// IdempotencyKeyFingerprint returns a short hexadecimal diagnostic value
// derived from the SHA-256 hash of the raw key. The raw key must never be
// persisted or logged; use this fingerprint for diagnostics instead.
func IdempotencyKeyFingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:16]
}
