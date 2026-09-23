package dispatch

import "context"

// Handler executes business logic with bound and validated parameters.
type Handler[P, R any] interface {
	Handle(ctx context.Context, params P) (R, error)
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc[P, R any] func(ctx context.Context, params P) (R, error)

func (f HandlerFunc[P, R]) Handle(ctx context.Context, params P) (R, error) {
	return f(ctx, params)
}
