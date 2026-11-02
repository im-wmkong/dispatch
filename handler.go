package dispatch

import "context"

// Handler executes business logic with bound and validated parameters.
type Handler[Params, Output any] interface {
	Handle(ctx context.Context, params Params) (Output, error)
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc[Params, Output any] func(ctx context.Context, params Params) (Output, error)

func (f HandlerFunc[Params, Output]) Handle(ctx context.Context, params Params) (Output, error) {
	return f(ctx, params)
}
