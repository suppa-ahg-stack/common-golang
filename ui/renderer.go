package ui

import (
	"context"
	"io"
)

// ComponentRenderer adapts a render function to templ.Component. It is useful
// when an application router builds a component dynamically.
type ComponentRenderer struct {
	RenderFunc func(ctx context.Context, w io.Writer) error
}

func (c *ComponentRenderer) Render(ctx context.Context, w io.Writer) error {
	if c == nil || c.RenderFunc == nil {
		return nil
	}
	return c.RenderFunc(ctx, w)
}
