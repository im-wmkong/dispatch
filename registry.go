package dispatch

import (
	"fmt"
	"strings"
	"sync"
)

// Registry stores actions with matching Input and Output types. Actions can have
// different Params types, while their input and output types must match at compile time.
// It supports concurrent registrations, removals, and lookups.
type Registry[Input, Output any] struct {
	mu      sync.RWMutex
	actions map[string]invoker[Input, Output]
}

// NewRegistry creates an empty Registry for use with NewDispatcher.
// Use New to create a Dispatcher with its own Registry.
func NewRegistry[Input, Output any]() *Registry[Input, Output] {
	return &Registry[Input, Output]{actions: make(map[string]invoker[Input, Output])}
}

// Register validates an Action definition and stores its invoker under a unique name.
// Create definitions with NewAction to retain compile-time parameter checks.
func (r *Registry[Input, Output]) Register(name string, action Action[Input, Output]) error {
	if r == nil {
		return fmt.Errorf("%w: registry is nil", ErrInvalidAction)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is blank", ErrInvalidAction)
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("%w: name has surrounding whitespace", ErrInvalidAction)
	}
	if action.definition == nil || action.invoker == nil {
		return fmt.Errorf("%w: action for %q is uninitialized", ErrInvalidAction, name)
	}
	if err := action.definition.validate(); err != nil {
		return fmt.Errorf("register action %q: %w", name, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.actions[name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateAction, name)
	}
	if r.actions == nil {
		r.actions = make(map[string]invoker[Input, Output])
	}
	r.actions[name] = action.invoker
	return nil
}

// Unregister removes an action's name mapping. Subsequent lookups will not find
// it until it is registered again. Calls that already obtained the invoker
// continue independently; Unregister neither waits for them nor closes resources.
func (r *Registry[Input, Output]) Unregister(name string) error {
	if r == nil {
		return fmt.Errorf("%w: registry is nil", ErrInvalidAction)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.actions[name]; !exists {
		return fmt.Errorf("%w: %q", ErrActionNotFound, name)
	}
	delete(r.actions, name)
	return nil
}

func (r *Registry[Input, Output]) lookup(name string) (invoker[Input, Output], bool) {
	r.mu.RLock()
	action, exists := r.actions[name]
	r.mu.RUnlock()
	return action, exists
}
