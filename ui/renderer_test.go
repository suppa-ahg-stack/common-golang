package ui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestComponentRenderer(t *testing.T) {
	var out strings.Builder
	renderer := &ComponentRenderer{RenderFunc: func(_ context.Context, w io.Writer) error {
		_, err := w.Write([]byte("rendered"))
		return err
	}}
	if err := renderer.Render(context.Background(), &out); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if out.String() != "rendered" {
		t.Fatalf("Render() output = %q", out.String())
	}

	want := errors.New("boom")
	renderer.RenderFunc = func(context.Context, io.Writer) error { return want }
	if err := renderer.Render(context.Background(), &out); !errors.Is(err, want) {
		t.Fatalf("Render() error = %v, want %v", err, want)
	}

	if err := (*ComponentRenderer)(nil).Render(context.Background(), &out); err != nil {
		t.Fatalf("nil renderer error = %v", err)
	}
}
