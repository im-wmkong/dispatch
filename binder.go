package dispatch

import "context"

// Binder converts a registry input into one action's parameters.
type Binder[Input, Params any] interface {
	Bind(ctx context.Context, input Input) (Params, error)
}

// BinderFunc adapts a function to Binder.
type BinderFunc[Input, Params any] func(ctx context.Context, input Input) (Params, error)

func (f BinderFunc[Input, Params]) Bind(ctx context.Context, input Input) (Params, error) {
	return f(ctx, input)
}
