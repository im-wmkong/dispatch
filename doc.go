// Package dispatch provides static action registration and runtime dispatch.
//
// Each Registry fixes one raw input type. An application may create multiple
// registries for different input types. Every registered action connects a
// Binder and Handler with a compile-time checked parameter type.
package dispatch
