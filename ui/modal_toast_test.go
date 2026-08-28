package ui

import (
	"context"
	"strings"
	"testing"
)

func TestModalToastContainer(t *testing.T) {
	html := renderToString(t, context.Background(), ModalToastContainer("application-roles"))
	for _, expected := range []string{
		`id="toast-application-roles"`,
		`data-modal-id="application-roles"`,
		`x-data="modalToastContainer"`,
		`x-for="toast in toasts"`,
		`store.removeToast(toast.id)`,
		`animation: toast-shrink`,
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("modal toast is missing %q: %s", expected, html)
		}
	}
}
