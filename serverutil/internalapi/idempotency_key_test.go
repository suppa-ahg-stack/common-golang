package internalapi

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateIdempotencyKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"minimum length", "abcdefghijklmnop", false},
		{"maximum length", strings.Repeat("a", MaxIdempotencyKeyLen), false},
		{"all visible ascii", visibleASCII(), false},
		{"empty", "", true},
		{"too short 15", "abcdefghijklmno", true},
		{"too long 256", strings.Repeat("a", MaxIdempotencyKeyLen+1), true},
		{"space", "abcdefghijklmnop q", true},
		{"tab", "abcdefghijklmno\tp", true},
		{"carriage return", "abcdefghijklmno\rp", true},
		{"line feed", "abcdefghijklmno\np", true},
		{"nul", "abcdefghijklmno\x00p", true},
		{"del", "abcdefghijklmno\x7Fp", true},
		{"unicode", "abcdefghijklmnoé", true},
		{"low bound", "abcdefghijklmno\x20p", true},
		{"high bound", "abcdefghijklmno\x7Fp", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdempotencyKey(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for key %q", tt.key)
				}
				if !errors.Is(err, ErrInvalidIdempotencyKey) {
					t.Errorf("expected ErrInvalidIdempotencyKey, got %v", err)
				}
				if tt.key != "" && strings.Contains(err.Error(), tt.key) {
					t.Errorf("error message must not contain the raw key: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for key %q: %v", tt.key, err)
			}
		})
	}
}

func TestValidateIdempotencyKeyDoesNotTrim(t *testing.T) {
	key := "  abcdefghijklmnop  "
	if err := ValidateIdempotencyKey(key); err == nil {
		t.Fatal("expected validation to reject a key with leading/trailing spaces without trimming")
	}
}

func TestNewIdempotencyKey(t *testing.T) {
	key, err := NewIdempotencyKey()
	if err != nil {
		t.Fatalf("NewIdempotencyKey failed: %v", err)
	}
	if err := ValidateIdempotencyKey(key); err != nil {
		t.Fatalf("generated key failed validation: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("expected 32 hex characters, got %d", len(key))
	}
}

func TestIdempotencyKeyFingerprint(t *testing.T) {
	fp1 := IdempotencyKeyFingerprint("key-aaaaaaaaaaaaaaaa")
	fp2 := IdempotencyKeyFingerprint("key-aaaaaaaaaaaaaaaa")
	fp3 := IdempotencyKeyFingerprint("key-bbbbbbbbbbbbbbbb")
	if fp1 != fp2 {
		t.Errorf("same key produced different fingerprints: %q vs %q", fp1, fp2)
	}
	if fp1 == fp3 {
		t.Errorf("different keys produced the same fingerprint: %q", fp1)
	}
	if len(fp1) != 16 {
		t.Errorf("expected 16 hex characters, got %d", len(fp1))
	}
}

func visibleASCII() string {
	var b strings.Builder
	b.Grow(MaxIdempotencyKeyLen)
	for i := 0x21; i <= 0x7E && b.Len() < MaxIdempotencyKeyLen; i++ {
		b.WriteByte(byte(i))
	}
	return b.String()
}
