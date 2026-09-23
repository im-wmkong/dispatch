package dispatch

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
)

// Dispatcher performs runtime action selection for one Registry input type.
// It is safe for concurrent use.
type Dispatcher[I any] struct {
	registry *Registry[I]
	validate *validator.Validate
}

func NewDispatcher[I any](registry *Registry[I]) (*Dispatcher[I], error) {
	if registry == nil {
		return nil, fmt.Errorf("%w: registry is nil", ErrInvalidAction)
	}

	registry.seal()
	return &Dispatcher[I]{registry: registry, validate: validator.New()}, nil
}

func (d *Dispatcher[I]) Dispatch(ctx context.Context, name string, input I) (any, error) {
	actionInvoker, exists := d.registry.lookup(name)
	if !exists {
		return nil, fmt.Errorf("%w: %q", ErrActionNotFound, name)
	}

	result, err := actionInvoker.invoke(ctx, input, d.validate)
	if err != nil {
		return nil, fmt.Errorf("dispatch action %q: %w", name, err)
	}
	return result, nil
}
