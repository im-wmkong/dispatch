package dispatch

import "errors"

var (
	// Registration errors.
	ErrRegistrySealed  = errors.New("action registry is sealed")
	ErrDuplicateAction = errors.New("action already registered")
	ErrInvalidAction   = errors.New("invalid action registration")

	// Dispatch errors. Binder and validation errors wrap the original cause.
	ErrActionNotFound = errors.New("action not found")
	ErrBind           = errors.New("action input binding failed")
	ErrValidate       = errors.New("action params validation failed")
)
