package dispatch

import "context"

// Binder converts a registry input into one action's parameters.
type Binder[I, P any] interface {
	Bind(ctx context.Context, input I) (P, error)
}

// BinderFunc adapts a function to Binder.
type BinderFunc[I, P any] func(ctx context.Context, input I) (P, error)

func (f BinderFunc[I, P]) Bind(ctx context.Context, input I) (P, error) {
	return f(ctx, input)
}
