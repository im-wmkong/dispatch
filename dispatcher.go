package dispatch

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
)

// Dispatcher manages and executes Actions with one input and output type.
// It uses a Registry to store actions and is safe for concurrent use.
type Dispatcher[Input, Output any] struct {
	registry *Registry[Input, Output]
	validate *validator.Validate
}

// New creates a Dispatcher with its own Registry. Use Register and Unregister
// to manage actions and Dispatch to execute them.
func New[Input, Output any]() *Dispatcher[Input, Output] {
	registry := NewRegistry[Input, Output]()
	// NewDispatcher only rejects nil registries; NewRegistry always returns a non-nil one.
	dispatcher, _ := NewDispatcher(registry)
	return dispatcher
}

// NewDispatcher creates a Dispatcher using the given Registry.
// Input and Output are inferred from registry. A nil registry returns ErrInvalidAction.
func NewDispatcher[Input, Output any](registry *Registry[Input, Output]) (*Dispatcher[Input, Output], error) {
	if registry == nil {
		return nil, fmt.Errorf("%w: registry is nil", ErrInvalidAction)
	}

	return &Dispatcher[Input, Output]{
		registry: registry,
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}, nil
}

// Register validates an Action definition and stores it under a unique name.
// It delegates to Registry.Register, preserving its validation and errors.
func (d *Dispatcher[Input, Output]) Register(name string, action Action[Input, Output]) error {
	if d == nil || d.registry == nil {
		return fmt.Errorf("%w: dispatcher is nil or uninitialized", ErrInvalidAction)
	}
	return d.registry.Register(name, action)
}

// Unregister removes an action's name mapping using Registry.Unregister.
// Calls that already obtained the action continue independently.
func (d *Dispatcher[Input, Output]) Unregister(name string) error {
	if d == nil || d.registry == nil {
		return fmt.Errorf("%w: dispatcher is nil or uninitialized", ErrInvalidAction)
	}
	return d.registry.Unregister(name)
}

// Dispatch executes the named Action and returns its strongly typed output.
// On failure it returns the zero value of Output and the error.
func (d *Dispatcher[Input, Output]) Dispatch(ctx context.Context, name string, input Input) (Output, error) {
	var zero Output
	if d == nil || d.registry == nil {
		return zero, fmt.Errorf("%w: dispatcher is nil or uninitialized", ErrInvalidAction)
	}
	registered, exists := d.registry.lookup(name)
	if !exists {
		return zero, fmt.Errorf("%w: %q", ErrActionNotFound, name)
	}
	result, err := registered.invoke(ctx, input, d.validate)
	if err != nil {
		return zero, fmt.Errorf("dispatch action %q: %w", name, err)
	}
	return result, nil
}
