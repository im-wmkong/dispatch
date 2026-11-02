package dispatch_test

import (
	"context"
	"errors"
	"testing"

	"github.com/im-wmkong/dispatch"
)

func TestActionDefinitionIsReusable(t *testing.T) {
	action := dispatch.NewAction(greetBinder{}, greetHandler{})
	registry := dispatch.NewRegistry[rawInput, greetResult]()
	for _, name := range []string{"greet", "greet_again"} {
		if err := registry.Register(name, action); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	dispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}
	for _, name := range []string{"greet", "greet_again"} {
		result, err := dispatcher.Dispatch(context.Background(), name, rawInput{Values: map[string]string{"name": "alice"}})
		if err != nil || result.Message != "hello, alice" {
			t.Fatalf("registered action %s is not executable: result=%#v err=%v", name, result, err)
		}
	}
}

func TestRegistryRejectsZeroAction(t *testing.T) {
	registry := dispatch.NewRegistry[rawInput, greetResult]()
	if err := registry.Register("greet", dispatch.Action[rawInput, greetResult]{}); !errors.Is(err, dispatch.ErrInvalidAction) {
		t.Fatalf("expected invalid action error, got %v", err)
	}
	if err := registry.Unregister("greet"); !errors.Is(err, dispatch.ErrActionNotFound) {
		t.Fatalf("zero action was registered: %v", err)
	}
	if err := registry.Register("greet", dispatch.NewAction(greetBinder{}, greetHandler{})); err != nil {
		t.Fatalf("invalid action reserved the name: %v", err)
	}
}
