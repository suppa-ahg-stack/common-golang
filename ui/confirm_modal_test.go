package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func renderToString(t *testing.T, ctx context.Context, cmp templ.Component) string {
	t.Helper()
	var buf strings.Builder
	if err := cmp.Render(ctx, &buf); err != nil {
		t.Fatalf("failed to render component: %v", err)
	}
	return buf.String()
}

func TestConfirmModalRendersDialogStructure(t *testing.T) {
	ctx := context.Background()
	props := ConfirmModalProps{
		ID:           "common-confirm-dialog",
		CancelLabel:  "Cancel",
		ConfirmLabel: "Confirm",
	}

	html := renderToString(t, ctx, ConfirmModal(props))

	if !strings.Contains(html, `<dialog id="common-confirm-dialog"`) {
		t.Errorf("expected dialog with id common-confirm-dialog, got: %s", html)
	}
	if !strings.Contains(html, `aria-labelledby="common-confirm-dialog-title"`) {
		t.Errorf("expected aria-labelledby attribute, got: %s", html)
	}
	if !strings.Contains(html, `aria-describedby="common-confirm-dialog-message"`) {
		t.Errorf("expected aria-describedby attribute, got: %s", html)
	}
	if !strings.Contains(html, `<h3 id="common-confirm-dialog-title"`) {
		t.Errorf("expected titled element, got: %s", html)
	}
	if !strings.Contains(html, `<p id="common-confirm-dialog-message"`) {
		t.Errorf("expected described element, got: %s", html)
	}
}

func TestConfirmModalEscapesLabels(t *testing.T) {
	ctx := context.Background()
	props := ConfirmModalProps{
		ID:           "common-confirm-dialog",
		CancelLabel:  "Annuler <script>",
		ConfirmLabel: "Confirmer & Co",
	}

	html := renderToString(t, ctx, ConfirmModal(props))

	if strings.Contains(html, "<script>") {
		t.Errorf("rendered HTML must not contain a raw script tag, got: %s", html)
	}
	if !strings.Contains(html, "Confirmer &amp; Co") {
		t.Errorf("expected HTML-escaped ampersand in confirm label, got: %s", html)
	}
}

func TestConfirmModalUsesSimpleAlpineExpressions(t *testing.T) {
	ctx := context.Background()
	props := ConfirmModalProps{
		ID:           "common-confirm-dialog",
		CancelLabel:  "Cancel",
		ConfirmLabel: "Confirm",
	}

	html := renderToString(t, ctx, ConfirmModal(props))

	// The CSP build disallows inline object construction or the `new` keyword.
	forbidden := []string{"new CustomEvent", "new ", "{ title:", "{title:", "{ message:", "{message:", "{ variant:", "{variant:"}
	for _, f := range forbidden {
		if strings.Contains(html, f) {
			t.Errorf("rendered HTML contains forbidden inline expression %q: %s", f, html)
		}
	}

	// Confirm expected CSP-safe directives are present.
	required := []string{
		`x-data="confirmDialog"`,
		`x-text="title"`,
		`x-text="message"`,
		`x-text="confirmLabel"`,
		`data-default-confirm-label="Confirm"`,
		`@close="onClose()"`,
		`@click="cancel()"`,
		`@click="confirm()"`,
		`:class="{ 'btn-error': variant === 'danger', 'btn-primary': variant !== 'danger' }"`,
		`:disabled="pending"`,
	}
	for _, r := range required {
		if !strings.Contains(html, r) {
			t.Errorf("rendered HTML missing required directive %q: %s", r, html)
		}
	}
}

func TestConfirmModalUsesBackdropForm(t *testing.T) {
	ctx := context.Background()
	props := ConfirmModalProps{
		ID:           "common-confirm-dialog",
		CancelLabel:  "Cancel",
		ConfirmLabel: "Confirm",
	}

	html := renderToString(t, ctx, ConfirmModal(props))

	if !strings.Contains(html, `<form method="dialog" class="modal-backdrop">`) {
		t.Errorf("expected native dialog backdrop form, got: %s", html)
	}
}
