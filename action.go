package dispatch

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-playground/validator/v10"
)

// Action is an opaque registration definition created by NewAction.
// It binds definition validation and execution to the same implementation.
// The zero value is invalid.
type Action[Input, Output any] struct {
	definition validatable
	invoker    invoker[Input, Output]
}

// validatable validates its own action definition before registration.
type validatable interface {
	validate() error
}

// invoker is the strongly typed execution contract. It hides an action's
// parameter type while preserving its input and output types.
type invoker[Input, Output any] interface {
	invoke(ctx context.Context, input Input, validate *validator.Validate) (Output, error)
}

type typedAction[Input, Params, Output any] struct {
	binder  Binder[Input, Params]
	handler Handler[Params, Output]
}

// NewAction connects a Binder and Handler with the same parameter type.
// Input, Params, and Output are inferred from the arguments. Registry.Register
// validates the definition before storing its invoker; constructing an Action
// executes no business logic.
func NewAction[Input, Params, Output any](
	binder Binder[Input, Params],
	handler Handler[Params, Output],
) Action[Input, Output] {
	action := &typedAction[Input, Params, Output]{binder: binder, handler: handler}
	return Action[Input, Output]{definition: action, invoker: action}
}

func (a typedAction[Input, Params, Output]) validate() error {
	if isNil(a.binder) {
		return fmt.Errorf("%w: binder is nil", ErrInvalidAction)
	}
	if isNil(a.handler) {
		return fmt.Errorf("%w: handler is nil", ErrInvalidAction)
	}
	if !isStructType(reflect.TypeFor[Params]()) {
		return fmt.Errorf("%w: params must be a struct or pointer to struct", ErrInvalidAction)
	}
	return nil
}

func (a typedAction[Input, Params, Output]) invoke(
	ctx context.Context,
	input Input,
	validate *validator.Validate,
) (Output, error) {
	var zero Output
	params, err := a.binder.Bind(ctx, input)
	if err != nil {
		return zero, fmt.Errorf("%w: %w", ErrBind, err)
	}
	if err := validate.StructCtx(ctx, params); err != nil {
		return zero, fmt.Errorf("%w: %w", ErrValidate, err)
	}
	result, err := a.handler.Handle(ctx, params)
	if err != nil {
		return zero, err
	}
	return result, nil
}

func isStructType(valueType reflect.Type) bool {
	if valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	return valueType.Kind() == reflect.Struct
}

func isNil(value any) bool {
	if value == nil {
		return true
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
