// Package dispatch provides concurrent action registration, removal, and
// strongly typed dispatch.
//
// New[Input, Output] creates a Dispatcher with its own Registry. Register and Unregister
// manage its actions. NewRegistry and NewDispatcher allow explicit Registry sharing.
// NewAction connects a Binder and Handler with a compile-time checked parameter
// type; Register accepts actions with matching Input and Output and validates their
// definitions before storing their invokers under unique names.
// A Dispatcher selects the named action, then binds, validates, and handles the input.
package dispatch
