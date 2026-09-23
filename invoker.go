package dispatch

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
)

// invoker is the internal type-erasure boundary. Registration constructs a
// typed invoker, while Registry and Dispatcher only retain the input type I.
type invoker[I any] interface {
	invoke(ctx context.Context, input I, validate *validator.Validate) (any, error)
}

type typedInvoker[I, P, R any] struct {
	binder  Binder[I, P]
	handler Handler[P, R]
}

func (i typedInvoker[I, P, R]) invoke(
	ctx context.Context,
	input I,
	validate *validator.Validate,
) (any, error) {
	params, err := i.binder.Bind(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBind, err)
	}

	if err := validate.StructCtx(ctx, params); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrValidate, err)
	}

	result, err := i.handler.Handle(ctx, params)
	if err != nil {
		return nil, err
	}
	return result, nil
}
