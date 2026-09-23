package dispatch_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	dispatch "github.com/im-wmkong/dispatch"

	"github.com/go-playground/validator/v10"
)

type rawInput struct {
	Values map[string]string
}

type greetParams struct {
	Name string `validate:"required,min=2,max=20"`
}

type greetResult struct {
	Message string
}

type greetBinder struct{}

func (greetBinder) Bind(_ context.Context, input rawInput) (greetParams, error) {
	return greetParams{Name: input.Values["name"]}, nil
}

type greetHandler struct{}

func (greetHandler) Handle(_ context.Context, params greetParams) (greetResult, error) {
	return greetResult{Message: "hello, " + params.Name}, nil
}

type lengthParams struct {
	Value string `validate:"required"`
}

type lengthBinder struct{}

func (lengthBinder) Bind(_ context.Context, input rawInput) (*lengthParams, error) {
	return &lengthParams{Value: input.Values["value"]}, nil
}

type lengthHandler struct{}

func (lengthHandler) Handle(_ context.Context, params *lengthParams) (int, error) {
	return len(params.Value), nil
}

func TestDispatcherSupportsDifferentParamsAndResults(t *testing.T) {
	registry := dispatch.NewRegistry[rawInput]()
	if err := dispatch.Register(registry, "greet", greetBinder{}, greetHandler{}); err != nil {
		t.Fatalf("register greet: %v", err)
	}
	if err := dispatch.Register(registry, "length", lengthBinder{}, lengthHandler{}); err != nil {
		t.Fatalf("register length: %v", err)
	}
	dispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}

	greetOutput, err := dispatcher.Dispatch(
		context.Background(),
		"greet",
		rawInput{Values: map[string]string{"name": "alice"}},
	)
	if err != nil {
		t.Fatalf("dispatch greet: %v", err)
	}
	if got, ok := greetOutput.(greetResult); !ok || got.Message != "hello, alice" {
		t.Fatalf("unexpected greet output: %#v", greetOutput)
	}

	lengthOutput, err := dispatcher.Dispatch(
		context.Background(),
		"length",
		rawInput{Values: map[string]string{"value": "hello"}},
	)
	if err != nil {
		t.Fatalf("dispatch length: %v", err)
	}
	if got, ok := lengthOutput.(int); !ok || got != 5 {
		t.Fatalf("unexpected length output: %#v", lengthOutput)
	}
}

func TestDispatcherPipeline(t *testing.T) {
	t.Run("binder failure stops pipeline", func(t *testing.T) {
		binderErr := errors.New("decode failed")
		var handled atomic.Bool
		dispatcher := dispatcherFor(
			t,
			dispatch.BinderFunc[rawInput, greetParams](func(context.Context, rawInput) (greetParams, error) {
				return greetParams{}, binderErr
			}),
			dispatch.HandlerFunc[greetParams, greetResult](func(context.Context, greetParams) (greetResult, error) {
				handled.Store(true)
				return greetResult{}, nil
			}),
		)

		_, err := dispatcher.Dispatch(context.Background(), "test", rawInput{})
		if !errors.Is(err, dispatch.ErrBind) || !errors.Is(err, binderErr) {
			t.Fatalf("expected wrapped bind error, got %v", err)
		}
		if handled.Load() {
			t.Fatal("handler ran after binder failure")
		}
	})

	t.Run("validation failure stops handler", func(t *testing.T) {
		var handled atomic.Bool
		dispatcher := dispatcherFor(
			t,
			dispatch.BinderFunc[rawInput, greetParams](func(context.Context, rawInput) (greetParams, error) {
				return greetParams{}, nil
			}),
			dispatch.HandlerFunc[greetParams, greetResult](func(context.Context, greetParams) (greetResult, error) {
				handled.Store(true)
				return greetResult{}, nil
			}),
		)

		_, err := dispatcher.Dispatch(context.Background(), "test", rawInput{})
		if !errors.Is(err, dispatch.ErrValidate) {
			t.Fatalf("expected validation error, got %v", err)
		}
		var fieldErrors validator.ValidationErrors
		if !errors.As(err, &fieldErrors) {
			t.Fatalf("expected validator field errors, got %T", err)
		}
		if fieldErrors[0].Field() != "Name" {
			t.Fatalf("unexpected field: %s", fieldErrors[0].Field())
		}
		if handled.Load() {
			t.Fatal("handler ran after validation failure")
		}
	})

	t.Run("handler error is preserved", func(t *testing.T) {
		handlerErr := errors.New("business failed")
		dispatcher := dispatcherFor(
			t,
			dispatch.BinderFunc[rawInput, greetParams](func(context.Context, rawInput) (greetParams, error) {
				return greetParams{Name: "alice"}, nil
			}),
			dispatch.HandlerFunc[greetParams, greetResult](func(context.Context, greetParams) (greetResult, error) {
				return greetResult{}, handlerErr
			}),
		)

		_, err := dispatcher.Dispatch(context.Background(), "test", rawInput{})
		if !errors.Is(err, handlerErr) {
			t.Fatalf("expected handler error, got %v", err)
		}
	})
}

func TestRegistryLifecycle(t *testing.T) {
	t.Run("zero value is usable", func(t *testing.T) {
		var registry dispatch.Registry[rawInput]
		if err := dispatch.Register(&registry, "test", greetBinder{}, greetHandler{}); err != nil {
			t.Fatalf("register on zero value registry: %v", err)
		}
		if _, err := dispatch.NewDispatcher(&registry); err != nil {
			t.Fatalf("new dispatcher: %v", err)
		}
	})

	t.Run("rejects invalid registrations", func(t *testing.T) {
		validBinder := dispatch.BinderFunc[rawInput, greetParams](func(context.Context, rawInput) (greetParams, error) {
			return greetParams{}, nil
		})
		validHandler := dispatch.HandlerFunc[greetParams, greetResult](func(context.Context, greetParams) (greetResult, error) {
			return greetResult{}, nil
		})

		if err := dispatch.Register[rawInput, greetParams, greetResult](nil, "test", validBinder, validHandler); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected invalid registry error, got %v", err)
		}
		if err := dispatch.Register(dispatch.NewRegistry[rawInput](), " ", validBinder, validHandler); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected invalid name error, got %v", err)
		}
		if err := dispatch.Register(dispatch.NewRegistry[rawInput](), " test", validBinder, validHandler); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected whitespace name error, got %v", err)
		}

		var nilBinder dispatch.BinderFunc[rawInput, greetParams]
		if err := dispatch.Register(dispatch.NewRegistry[rawInput](), "test", nilBinder, validHandler); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected nil binder error, got %v", err)
		}
		var nilHandler dispatch.HandlerFunc[greetParams, greetResult]
		if err := dispatch.Register(dispatch.NewRegistry[rawInput](), "test", validBinder, nilHandler); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected nil handler error, got %v", err)
		}
	})

	t.Run("rejects duplicate and registration after dispatcher", func(t *testing.T) {
		registry := dispatch.NewRegistry[rawInput]()
		if err := dispatch.Register(registry, "test", greetBinder{}, greetHandler{}); err != nil {
			t.Fatalf("register action: %v", err)
		}
		if err := dispatch.Register(registry, "test", greetBinder{}, greetHandler{}); !errors.Is(err, dispatch.ErrDuplicateAction) {
			t.Fatalf("expected duplicate error, got %v", err)
		}
		if _, err := dispatch.NewDispatcher(registry); err != nil {
			t.Fatalf("new dispatcher: %v", err)
		}
		if err := dispatch.Register(registry, "later", greetBinder{}, greetHandler{}); !errors.Is(err, dispatch.ErrRegistrySealed) {
			t.Fatalf("expected sealed error, got %v", err)
		}
	})
}

func TestUnknownAction(t *testing.T) {
	registry := dispatch.NewRegistry[rawInput]()
	dispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}

	_, err = dispatcher.Dispatch(context.Background(), "missing", rawInput{})
	if !errors.Is(err, dispatch.ErrActionNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestMultipleRegistryInputTypesCoexist(t *testing.T) {
	stringRegistry := dispatch.NewRegistry[string]()
	if err := dispatch.Register(
		stringRegistry,
		"length",
		dispatch.BinderFunc[string, struct{ Value string }](func(_ context.Context, input string) (struct{ Value string }, error) {
			return struct{ Value string }{Value: input}, nil
		}),
		dispatch.HandlerFunc[struct{ Value string }, int](func(_ context.Context, params struct{ Value string }) (int, error) {
			return len(params.Value), nil
		}),
	); err != nil {
		t.Fatalf("register string action: %v", err)
	}

	type jobInput struct{ ID int }
	jobRegistry := dispatch.NewRegistry[jobInput]()
	if err := dispatch.Register(
		jobRegistry,
		"describe",
		dispatch.BinderFunc[jobInput, struct{ ID int }](func(_ context.Context, input jobInput) (struct{ ID int }, error) {
			return struct{ ID int }{ID: input.ID}, nil
		}),
		dispatch.HandlerFunc[struct{ ID int }, string](func(_ context.Context, params struct{ ID int }) (string, error) {
			return fmt.Sprintf("job-%d", params.ID), nil
		}),
	); err != nil {
		t.Fatalf("register job action: %v", err)
	}

	stringDispatcher, _ := dispatch.NewDispatcher(stringRegistry)
	jobDispatcher, _ := dispatch.NewDispatcher(jobRegistry)

	stringOutput, err := stringDispatcher.Dispatch(context.Background(), "length", "hello")
	if err != nil || stringOutput != 5 {
		t.Fatalf("unexpected string dispatch: output=%#v err=%v", stringOutput, err)
	}
	jobOutput, err := jobDispatcher.Dispatch(context.Background(), "describe", jobInput{ID: 7})
	if err != nil || jobOutput != "job-7" {
		t.Fatalf("unexpected job dispatch: output=%#v err=%v", jobOutput, err)
	}
}

func TestDispatcherConcurrentUse(t *testing.T) {
	registry := dispatch.NewRegistry[int]()
	if err := dispatch.Register(
		registry,
		"double",
		dispatch.BinderFunc[int, struct{ Value int }](func(_ context.Context, input int) (struct{ Value int }, error) {
			return struct{ Value int }{Value: input}, nil
		}),
		dispatch.HandlerFunc[struct{ Value int }, int](func(_ context.Context, params struct{ Value int }) (int, error) {
			return params.Value * 2, nil
		}),
	); err != nil {
		t.Fatalf("register action: %v", err)
	}
	dispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}

	const workers = 100
	var wait sync.WaitGroup
	errorsCh := make(chan error, workers)
	for value := 0; value < workers; value++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			output, err := dispatcher.Dispatch(context.Background(), "double", value)
			if err != nil {
				errorsCh <- err
				return
			}
			if output != value*2 {
				errorsCh <- fmt.Errorf("input %d: got %#v", value, output)
			}
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Error(err)
	}
}

func dispatcherFor[P, R any](
	t *testing.T,
	binder dispatch.Binder[rawInput, P],
	handler dispatch.Handler[P, R],
) *dispatch.Dispatcher[rawInput] {
	t.Helper()
	registry := dispatch.NewRegistry[rawInput]()
	if err := dispatch.Register(registry, "test", binder, handler); err != nil {
		t.Fatalf("register action: %v", err)
	}
	dispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}
	return dispatcher
}
