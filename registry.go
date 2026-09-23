package dispatch

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// Registry stores the static mapping from action name to invoker for one input
// type. It accepts registrations until a Dispatcher is created from it.
type Registry[I any] struct {
	mu       sync.RWMutex
	sealed   bool
	invokers map[string]invoker[I]
}

func NewRegistry[I any]() *Registry[I] {
	return &Registry[I]{invokers: make(map[string]invoker[I])}
}

// Register associates an action name with a strongly typed Binder and Handler.
// I, P, and R are inferred from the arguments at the call site.
func Register[I, P, R any](
	registry *Registry[I],
	name string,
	binder Binder[I, P],
	handler Handler[P, R],
) error {
	if registry == nil {
		return fmt.Errorf("%w: registry is nil", ErrInvalidAction)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is blank", ErrInvalidAction)
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("%w: name has surrounding whitespace", ErrInvalidAction)
	}
	if isNil(binder) {
		return fmt.Errorf("%w: binder for %q is nil", ErrInvalidAction, name)
	}
	if isNil(handler) {
		return fmt.Errorf("%w: handler for %q is nil", ErrInvalidAction, name)
	}
	if !isStructType(reflect.TypeFor[P]()) {
		return fmt.Errorf("%w: params for %q must be a struct or pointer to struct", ErrInvalidAction, name)
	}
	return registry.register(name, typedInvoker[I, P, R]{
		binder:  binder,
		handler: handler,
	})
}

func (r *Registry[I]) register(name string, actionInvoker invoker[I]) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.sealed {
		return fmt.Errorf("%w: %q", ErrRegistrySealed, name)
	}
	if _, exists := r.invokers[name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateAction, name)
	}
	if r.invokers == nil {
		r.invokers = make(map[string]invoker[I])
	}
	r.invokers[name] = actionInvoker
	return nil
}

func (r *Registry[I]) seal() {
	r.mu.Lock()
	r.sealed = true
	r.mu.Unlock()
}

func (r *Registry[I]) lookup(name string) (invoker[I], bool) {
	r.mu.RLock()
	actionInvoker, exists := r.invokers[name]
	r.mu.RUnlock()
	return actionInvoker, exists
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
