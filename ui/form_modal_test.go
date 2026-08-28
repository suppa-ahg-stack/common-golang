package ui

import (
	"context"
	"strings"
	"testing"
)

func TestFormModalTriggerRendersAccessibleCSPDialog(t *testing.T) {
	html := renderToString(t, context.Background(), FormModalTrigger(FormModalTriggerProps{
		ID: "agent-modal-new", Label: "Add agent", Title: "Add agent", Variant: "primary", SizeClass: "max-w-3xl",
	}))
	for _, expected := range []string{
		`<button type="button" class="btn btn-primary"`,
		`<dialog id="agent-modal-new" class="modal" data-agent-card-modal`,
		`aria-labelledby="agent-modal-new-title"`,
		`$refs.formModal.showModal()`,
		`<h2 id="agent-modal-new-title"`,
		`max-w-3xl max-h-[90vh] overflow-y-auto`,
		`<form method="dialog" class="modal-backdrop">`,
	} {
		if !strings.Contains(html, expected) { t.Errorf("expected %q in %s", expected, html) }
	}
	for _, forbidden := range []string{"new CustomEvent", "new ", "{ title:"} {
		if strings.Contains(html, forbidden) { t.Errorf("forbidden CSP expression %q in %s", forbidden, html) }
	}
}