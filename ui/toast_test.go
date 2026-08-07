package ui

import (
	"context"
	"strings"
	"testing"
)

func TestToast(t *testing.T) {
	html := renderToString(t, context.Background(), Toast())
	for _, expected := range []string{
		`id="toast-container"`,
		`@mouseenter="store.pauseTimer(toast.id)"`,
		`@mouseleave="store.resumeTimer(toast.id)"`,
		`animation: toast-shrink`,
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("toast is missing %q", expected)
		}
	}
}
