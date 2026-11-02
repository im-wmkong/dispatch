package dispatch_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func (lengthHandler) Handle(_ context.Context, params *lengthParams) (greetResult, error) {
	return greetResult{Message: fmt.Sprint(len(params.Value))}, nil
}

func TestDispatcherSupportsDifferentParams(t *testing.T) {
	dispatcher := dispatch.New[rawInput, greetResult]()
	if err := dispatcher.Register("greet", dispatch.NewAction(greetBinder{}, greetHandler{})); err != nil {
		t.Fatalf("register greet: %v", err)
	}
	if err := dispatcher.Register("length", dispatch.NewAction(lengthBinder{}, lengthHandler{})); err != nil {
		t.Fatalf("register length: %v", err)
	}
	for _, test := range []struct {
		name  string
		input rawInput
		want  string
	}{
		{name: "greet", input: rawInput{Values: map[string]string{"name": "alice"}}, want: "hello, alice"},
		{name: "length", input: rawInput{Values: map[string]string{"value": "hello"}}, want: "5"},
	} {
		result, err := dispatcher.Dispatch(context.Background(), test.name, test.input)
		if err != nil || result.Message != test.want {
			t.Fatalf("dispatch %s: result=%#v err=%v", test.name, result, err)
		}
	}
}

func TestDispatcherRegistrationErrors(t *testing.T) {
	dispatcher := dispatch.New[rawInput, greetResult]()
	action := dispatch.NewAction(greetBinder{}, greetHandler{})
	if err := dispatcher.Register("greet", dispatch.Action[rawInput, greetResult]{}); !errors.Is(err, dispatch.ErrInvalidAction) {
		t.Fatalf("expected invalid action error, got %v", err)
	}
	if err := dispatcher.Register(" ", action); !errors.Is(err, dispatch.ErrInvalidAction) {
		t.Fatalf("expected invalid name error, got %v", err)
	}
	if err := dispatcher.Register("greet", action); err != nil {
		t.Fatalf("invalid action reserved the name: %v", err)
	}
	replacement := dispatch.NewAction(greetBinder{}, dispatch.HandlerFunc[greetParams, greetResult](func(context.Context, greetParams) (greetResult, error) {
		return greetResult{Message: "replacement"}, nil
	}))
	if err := dispatcher.Register("greet", replacement); !errors.Is(err, dispatch.ErrDuplicateAction) {
		t.Fatalf("expected duplicate action error, got %v", err)
	}
	result, err := dispatcher.Dispatch(context.Background(), "greet", rawInput{Values: map[string]string{"name": "alice"}})
	if err != nil || result.Message != "hello, alice" {
		t.Fatalf("duplicate registration replaced the original action: result=%#v err=%v", result, err)
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
				return greetResult{Message: "partial result"}, handlerErr
			}),
		)

		result, err := dispatcher.Dispatch(context.Background(), "test", rawInput{})
		if !errors.Is(err, handlerErr) {
			t.Fatalf("expected handler error, got %v", err)
		}
		if result != (greetResult{}) {
			t.Fatalf("expected zero result on error, got %#v", result)
		}
	})
}

func TestDispatcherValidatesRequiredNestedStruct(t *testing.T) {
	type detail struct{ Value int }
	type params struct {
		Detail detail `validate:"required"`
	}
	var handled bool
	registry := dispatch.NewRegistry[int, bool]()
	if err := registry.Register("nested", dispatch.NewAction(
		dispatch.BinderFunc[int, params](func(_ context.Context, input int) (params, error) {
			return params{Detail: detail{Value: input}}, nil
		}),
		dispatch.HandlerFunc[params, bool](func(context.Context, params) (bool, error) {
			handled = true
			return true, nil
		}),
	)); err != nil {
		t.Fatalf("register nested params: %v", err)
	}
	dispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}
	result, err := dispatcher.Dispatch(context.Background(), "nested", 0)
	var fieldErrors validator.ValidationErrors
	if !errors.Is(err, dispatch.ErrValidate) || !errors.As(err, &fieldErrors) || result || handled {
		t.Fatalf("zero nested struct passed validation: result=%v handled=%v err=%v", result, handled, err)
	}
	if fieldErrors[0].Field() != "Detail" || fieldErrors[0].Tag() != "required" {
		t.Fatalf("unexpected field error: %v", fieldErrors[0])
	}
	result, err = dispatcher.Dispatch(context.Background(), "nested", 1)
	if err != nil || !result || !handled {
		t.Fatalf("nonzero nested struct failed: result=%v handled=%v err=%v", result, handled, err)
	}
}

func TestRegistryLifecycle(t *testing.T) {
	t.Run("zero value is usable", func(t *testing.T) {
		var registry dispatch.Registry[rawInput, greetResult]
		if err := registry.Register("test", dispatch.NewAction(greetBinder{}, greetHandler{})); err != nil {
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

		if err := (*dispatch.Registry[rawInput, greetResult])(nil).Register("test", dispatch.NewAction(validBinder, validHandler)); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected invalid registry error, got %v", err)
		}
		if err := dispatch.NewRegistry[rawInput, greetResult]().Register(" ", dispatch.NewAction(validBinder, validHandler)); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected invalid name error, got %v", err)
		}
		if err := dispatch.NewRegistry[rawInput, greetResult]().Register(" test", dispatch.NewAction(validBinder, validHandler)); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected whitespace name error, got %v", err)
		}

		var nilBinder dispatch.BinderFunc[rawInput, greetParams]
		if err := dispatch.NewRegistry[rawInput, greetResult]().Register("test", dispatch.NewAction(nilBinder, validHandler)); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected nil binder error, got %v", err)
		}
		var nilHandler dispatch.HandlerFunc[greetParams, greetResult]
		if err := dispatch.NewRegistry[rawInput, greetResult]().Register("test", dispatch.NewAction(validBinder, nilHandler)); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected nil handler error, got %v", err)
		}
		registry := dispatch.NewRegistry[rawInput, greetResult]()
		var zeroAction dispatch.Action[rawInput, greetResult]
		if err := registry.Register("test", zeroAction); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected zero action error, got %v", err)
		}
		invalidParams := dispatch.NewAction(
			dispatch.BinderFunc[rawInput, string](func(context.Context, rawInput) (string, error) { return "", nil }),
			dispatch.HandlerFunc[string, greetResult](func(context.Context, string) (greetResult, error) { return greetResult{}, nil }),
		)
		if err := registry.Register("test", invalidParams); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected invalid parameter type error, got %v", err)
		}
		if err := registry.Register("test", dispatch.NewAction(validBinder, validHandler)); err != nil {
			t.Fatalf("invalid definition reserved the action name: %v", err)
		}
	})

	t.Run("rejects duplicates before and after dispatcher construction", func(t *testing.T) {
		registry := dispatch.NewRegistry[rawInput, greetResult]()
		if err := registry.Register("test", dispatch.NewAction(greetBinder{}, greetHandler{})); err != nil {
			t.Fatalf("register action: %v", err)
		}
		if err := registry.Register("test", dispatch.NewAction(greetBinder{}, greetHandler{})); !errors.Is(err, dispatch.ErrDuplicateAction) {
			t.Fatalf("expected duplicate error, got %v", err)
		}
		if _, err := dispatch.NewDispatcher(registry); err != nil {
			t.Fatalf("new dispatcher: %v", err)
		}
		if err := registry.Register("test", dispatch.NewAction(greetBinder{}, greetHandler{})); !errors.Is(err, dispatch.ErrDuplicateAction) {
			t.Fatalf("expected duplicate error after dispatcher construction, got %v", err)
		}
	})
}

func TestDispatchersShareRegistry(t *testing.T) {
	registry := dispatch.NewRegistry[rawInput, greetResult]()
	greetDispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new greet dispatcher: %v", err)
	}
	lengthDispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new length dispatcher: %v", err)
	}
	ctx := context.Background()
	if _, err := greetDispatcher.Dispatch(ctx, "greet", rawInput{}); !errors.Is(err, dispatch.ErrActionNotFound) {
		t.Fatalf("expected not found before registration, got %v", err)
	}
	if err := greetDispatcher.Register("greet", dispatch.NewAction(greetBinder{}, greetHandler{})); err != nil {
		t.Fatalf("register greet after dispatcher construction: %v", err)
	}
	for _, dispatcher := range []*dispatch.Dispatcher[rawInput, greetResult]{greetDispatcher, lengthDispatcher} {
		greeting, err := dispatcher.Dispatch(ctx, "greet", rawInput{Values: map[string]string{"name": "alice"}})
		if err != nil || greeting.Message != "hello, alice" {
			t.Fatalf("newly registered greet is unavailable: result=%#v err=%v", greeting, err)
		}
	}
	if err := registry.Register("length", dispatch.NewAction(lengthBinder{}, lengthHandler{})); err != nil {
		t.Fatalf("register length after dispatcher construction: %v", err)
	}
	length, err := lengthDispatcher.Dispatch(ctx, "length", rawInput{Values: map[string]string{"value": "hello"}})
	if err != nil || length.Message != "5" {
		t.Fatalf("newly registered length is unavailable: result=%#v err=%v", length, err)
	}
	if err := lengthDispatcher.Unregister("greet"); err != nil {
		t.Fatalf("unregister shared action: %v", err)
	}
	for _, dispatcher := range []*dispatch.Dispatcher[rawInput, greetResult]{greetDispatcher, lengthDispatcher} {
		if _, err := dispatcher.Dispatch(ctx, "greet", rawInput{}); !errors.Is(err, dispatch.ErrActionNotFound) {
			t.Fatalf("shared dispatcher did not see the removal: %v", err)
		}
	}
	if err := registry.Unregister("greet"); !errors.Is(err, dispatch.ErrActionNotFound) {
		t.Fatalf("registry did not see the removal: %v", err)
	}
}

func TestConcurrentDuplicateRegistration(t *testing.T) {
	registry := dispatch.NewRegistry[int, int]()
	dispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}
	type params struct{ Value int }
	binder := dispatch.BinderFunc[int, params](func(_ context.Context, input int) (params, error) {
		return params{Value: input}, nil
	})
	type attempt struct {
		index       int
		registerErr error
		output      int
		dispatchErr error
	}
	const workers = 32
	start := make(chan struct{})
	results := make(chan attempt, workers)
	for index := 0; index < workers; index++ {
		go func() {
			<-start
			registerErr := registry.Register(
				"winner",
				dispatch.NewAction(
					binder,
					dispatch.HandlerFunc[params, int](func(_ context.Context, input params) (int, error) {
						return input.Value + index, nil
					}),
				),
			)
			output, dispatchErr := dispatcher.Dispatch(context.Background(), "winner", 100)
			results <- attempt{index: index, registerErr: registerErr, output: output, dispatchErr: dispatchErr}
		}()
	}
	close(start)
	winner := -1
	outputs := make([]int, 0, workers)
	for range workers {
		result := <-results
		if result.registerErr == nil {
			if winner != -1 {
				t.Fatal("more than one registration succeeded for the same name")
			}
			winner = result.index
		} else if !errors.Is(result.registerErr, dispatch.ErrDuplicateAction) {
			t.Fatalf("unexpected registration error: %v", result.registerErr)
		}
		if result.dispatchErr != nil {
			t.Fatalf("dispatch during concurrent registration: %v", result.dispatchErr)
		}
		outputs = append(outputs, result.output)
	}
	if winner == -1 {
		t.Fatal("no registration succeeded")
	}
	for _, output := range outputs {
		if output != 100+winner {
			t.Fatalf("registered handler was replaced: winner=%d output=%d", winner, output)
		}
	}
}

func TestConcurrentRegistrationAndDispatch(t *testing.T) {
	dispatcher := dispatch.New[int, int]()
	type params struct{ Value int }
	binder := dispatch.BinderFunc[int, params](func(_ context.Context, input int) (params, error) {
		return params{Value: input}, nil
	})
	handler := dispatch.HandlerFunc[params, int](func(_ context.Context, input params) (int, error) {
		return input.Value * 2, nil
	})
	if err := dispatcher.Register("double", dispatch.NewAction(binder, handler)); err != nil {
		t.Fatalf("register double: %v", err)
	}
	const additions = 32
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		for index := 0; index < additions; index++ {
			if err := dispatcher.Register(fmt.Sprintf("action_%d", index), dispatch.NewAction(binder, handler)); err != nil {
				results <- err
				return
			}
		}
		results <- nil
	}()
	go func() {
		<-start
		for input := 0; input < additions*4; input++ {
			output, err := dispatcher.Dispatch(context.Background(), "double", input)
			if err != nil {
				results <- err
				return
			}
			if output != input*2 {
				results <- fmt.Errorf("input %d: got %d", input, output)
				return
			}
		}
		results <- nil
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent registration and dispatch: %v", err)
		}
	}
	for index := 0; index < additions; index++ {
		output, err := dispatcher.Dispatch(context.Background(), fmt.Sprintf("action_%d", index), index)
		if err != nil || output != index*2 {
			t.Fatalf("new action %d is unavailable: output=%d err=%v", index, output, err)
		}
	}
}

func TestRegistryUnregister(t *testing.T) {
	registry := dispatch.NewRegistry[rawInput, greetResult]()
	if err := registry.Register("greet", dispatch.NewAction(greetBinder{}, greetHandler{})); err != nil {
		t.Fatalf("register greet: %v", err)
	}
	if err := registry.Register("length", dispatch.NewAction(lengthBinder{}, lengthHandler{})); err != nil {
		t.Fatalf("register length: %v", err)
	}
	dispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}
	if err := registry.Unregister("greet"); err != nil {
		t.Fatalf("unregister greet: %v", err)
	}
	ctx := context.Background()
	input := rawInput{Values: map[string]string{"name": "alice"}}
	result, err := dispatcher.Dispatch(ctx, "greet", input)
	if !errors.Is(err, dispatch.ErrActionNotFound) || result != (greetResult{}) {
		t.Fatalf("deleted action remained callable: result=%#v err=%v", result, err)
	}
	if err := registry.Unregister("greet"); !errors.Is(err, dispatch.ErrActionNotFound) {
		t.Fatalf("expected missing action error on repeated removal, got %v", err)
	}
	lengthDispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new length dispatcher: %v", err)
	}
	length, err := lengthDispatcher.Dispatch(ctx, "length", rawInput{Values: map[string]string{"value": "hello"}})
	if err != nil || length.Message != "5" {
		t.Fatalf("unrelated action was affected: result=%#v err=%v", length, err)
	}
	if err := registry.Register(
		"greet",
		dispatch.NewAction(
			greetBinder{},
			dispatch.HandlerFunc[greetParams, greetResult](func(_ context.Context, params greetParams) (greetResult, error) {
				return greetResult{Message: "updated, " + params.Name}, nil
			}),
		),
	); err != nil {
		t.Fatalf("register greet again: %v", err)
	}
	result, err = dispatcher.Dispatch(ctx, "greet", input)
	if err != nil || result.Message != "updated, alice" {
		t.Fatalf("dispatcher did not see the new registration: result=%#v err=%v", result, err)
	}
}

func TestRegistryUnregisterMissingAction(t *testing.T) {
	for _, test := range []struct {
		name     string
		registry *dispatch.Registry[rawInput, greetResult]
		err      error
	}{
		{name: "nil registry", err: dispatch.ErrInvalidAction},
		{name: "zero registry", registry: &dispatch.Registry[rawInput, greetResult]{}, err: dispatch.ErrActionNotFound},
		{name: "empty registry", registry: dispatch.NewRegistry[rawInput, greetResult](), err: dispatch.ErrActionNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.registry.Unregister("missing"); !errors.Is(err, test.err) {
				t.Fatalf("expected %v, got %v", test.err, err)
			}
		})
	}
}

func TestUnregisterPreservesCallsThatAlreadyLookedUpTheAction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dispatcher := dispatch.New[rawInput, greetResult]()
	started := make(chan struct{})
	release := make(chan struct{})
	if err := dispatcher.Register(
		"greet",
		dispatch.NewAction(
			dispatch.BinderFunc[rawInput, greetParams](func(ctx context.Context, input rawInput) (greetParams, error) {
				close(started)
				select {
				case <-release:
					return greetParams{Name: input.Values["name"]}, nil
				case <-ctx.Done():
					return greetParams{}, ctx.Err()
				}
			}),
			greetHandler{},
		),
	); err != nil {
		t.Fatalf("register greet: %v", err)
	}
	type completion struct {
		result greetResult
		err    error
	}
	done := make(chan completion, 1)
	go func() {
		result, err := dispatcher.Dispatch(ctx, "greet", rawInput{Values: map[string]string{"name": "alice"}})
		done <- completion{result: result, err: err}
	}()
	select {
	case <-started:
	case result := <-done:
		t.Fatalf("call returned before reaching binder: %#v", result)
	case <-ctx.Done():
		t.Fatal("call did not reach binder")
	}
	removed := make(chan error, 1)
	go func() { removed <- dispatcher.Unregister("greet") }()
	select {
	case err := <-removed:
		if err != nil {
			t.Fatalf("unregister while binder is blocked: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("unregister waited for the active call")
	}
	if _, err := dispatcher.Dispatch(ctx, "greet", rawInput{}); !errors.Is(err, dispatch.ErrActionNotFound) {
		t.Fatalf("expected deleted action to be unavailable, got %v", err)
	}
	close(release)
	select {
	case result := <-done:
		if result.err != nil || result.result.Message != "hello, alice" {
			t.Fatalf("original call did not complete: result=%#v err=%v", result.result, result.err)
		}
	case <-ctx.Done():
		t.Fatal("original call did not complete")
	}
}

func TestConcurrentUnregisterAndDispatch(t *testing.T) {
	dispatcher := dispatch.New[int, int]()
	type params struct{ Value int }
	binder := dispatch.BinderFunc[int, params](func(_ context.Context, input int) (params, error) {
		return params{Value: input}, nil
	})
	handler := dispatch.HandlerFunc[params, int](func(_ context.Context, input params) (int, error) {
		return input.Value * 2, nil
	})
	if err := dispatcher.Register("double", dispatch.NewAction(binder, handler)); err != nil {
		t.Fatalf("register double: %v", err)
	}
	const updates = 32
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		for range updates {
			if err := dispatcher.Unregister("double"); err != nil {
				results <- err
				return
			}
			if err := dispatcher.Register("double", dispatch.NewAction(binder, handler)); err != nil {
				results <- err
				return
			}
		}
		results <- nil
	}()
	go func() {
		<-start
		for input := 0; input < updates*4; input++ {
			output, err := dispatcher.Dispatch(context.Background(), "double", input)
			if errors.Is(err, dispatch.ErrActionNotFound) {
				continue
			}
			if err != nil {
				results <- err
				return
			}
			if output != input*2 {
				results <- fmt.Errorf("input %d: got %d", input, output)
				return
			}
		}
		results <- nil
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent unregister and dispatch: %v", err)
		}
	}
	output, err := dispatcher.Dispatch(context.Background(), "double", 10)
	if err != nil || output != 20 {
		t.Fatalf("final registration is unavailable: output=%d err=%v", output, err)
	}
}

func TestUnknownAction(t *testing.T) {
	registry := dispatch.NewRegistry[rawInput, greetResult]()
	dispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}

	_, err = dispatcher.Dispatch(context.Background(), "missing", rawInput{})
	if !errors.Is(err, dispatch.ErrActionNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestNewSupportsDifferentInputAndResultTypes(t *testing.T) {
	stringDispatcher := dispatch.New[string, int]()
	if err := stringDispatcher.Register(
		"length",
		dispatch.NewAction(
			dispatch.BinderFunc[string, struct{ Value string }](func(_ context.Context, input string) (struct{ Value string }, error) {
				return struct{ Value string }{Value: input}, nil
			}),
			dispatch.HandlerFunc[struct{ Value string }, int](func(_ context.Context, params struct{ Value string }) (int, error) {
				return len(params.Value), nil
			}),
		),
	); err != nil {
		t.Fatalf("register string action: %v", err)
	}

	type jobInput struct{ ID int }
	jobDispatcher := dispatch.New[jobInput, string]()
	if err := jobDispatcher.Register(
		"describe",
		dispatch.NewAction(
			dispatch.BinderFunc[jobInput, struct{ ID int }](func(_ context.Context, input jobInput) (struct{ ID int }, error) {
				return struct{ ID int }{ID: input.ID}, nil
			}),
			dispatch.HandlerFunc[struct{ ID int }, string](func(_ context.Context, params struct{ ID int }) (string, error) {
				return fmt.Sprintf("job-%d", params.ID), nil
			}),
		),
	); err != nil {
		t.Fatalf("register job action: %v", err)
	}

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
	registry := dispatch.NewRegistry[int, int]()
	if err := registry.Register(
		"double",
		dispatch.NewAction(
			dispatch.BinderFunc[int, struct{ Value int }](func(_ context.Context, input int) (struct{ Value int }, error) {
				return struct{ Value int }{Value: input}, nil
			}),
			dispatch.HandlerFunc[struct{ Value int }, int](func(_ context.Context, params struct{ Value int }) (int, error) {
				return params.Value * 2, nil
			}),
		),
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

func TestNewDispatchersAreIndependent(t *testing.T) {
	firstDispatcher := dispatch.New[rawInput, greetResult]()
	secondDispatcher := dispatch.New[rawInput, greetResult]()
	if err := firstDispatcher.Register("greet", dispatch.NewAction(greetBinder{}, greetHandler{})); err != nil {
		t.Fatalf("register first action: %v", err)
	}
	if err := secondDispatcher.Register("greet", dispatch.NewAction(
		greetBinder{},
		dispatch.HandlerFunc[greetParams, greetResult](func(_ context.Context, params greetParams) (greetResult, error) {
			return greetResult{Message: "goodbye, " + params.Name}, nil
		}),
	)); err != nil {
		t.Fatalf("register second action: %v", err)
	}
	ctx := context.Background()
	input := rawInput{Values: map[string]string{"name": "alice"}}
	first, err := firstDispatcher.Dispatch(ctx, "greet", input)
	if err != nil || first.Message != "hello, alice" {
		t.Fatalf("first dispatcher used the wrong registry: result=%#v err=%v", first, err)
	}
	second, err := secondDispatcher.Dispatch(ctx, "greet", input)
	if err != nil || second.Message != "goodbye, alice" {
		t.Fatalf("second dispatcher used the wrong registry: result=%#v err=%v", second, err)
	}
	if err := firstDispatcher.Unregister("greet"); err != nil {
		t.Fatalf("unregister first action: %v", err)
	}
	if _, err := firstDispatcher.Dispatch(ctx, "greet", input); !errors.Is(err, dispatch.ErrActionNotFound) {
		t.Fatalf("first dispatcher did not see the removal: %v", err)
	}
	if err := firstDispatcher.Unregister("greet"); !errors.Is(err, dispatch.ErrActionNotFound) {
		t.Fatalf("expected missing action error on repeated removal, got %v", err)
	}
	second, err = secondDispatcher.Dispatch(ctx, "greet", input)
	if err != nil || second.Message != "goodbye, alice" {
		t.Fatalf("removal affected the second registry: result=%#v err=%v", second, err)
	}
}

func TestInvalidDispatcher(t *testing.T) {
	if dispatcher, err := dispatch.NewDispatcher[rawInput, greetResult](nil); dispatcher != nil || !errors.Is(err, dispatch.ErrInvalidAction) {
		t.Fatalf("expected nil registry error, got dispatcher=%v err=%v", dispatcher, err)
	}
	for _, dispatcher := range []*dispatch.Dispatcher[rawInput, greetResult]{nil, {}} {
		if err := dispatcher.Register("greet", dispatch.NewAction(greetBinder{}, greetHandler{})); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected invalid dispatcher error on registration, got %v", err)
		}
		if err := dispatcher.Unregister("greet"); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected invalid dispatcher error on removal, got %v", err)
		}
		if _, err := dispatcher.Dispatch(context.Background(), "greet", rawInput{}); !errors.Is(err, dispatch.ErrInvalidAction) {
			t.Fatalf("expected invalid dispatcher error, got %v", err)
		}
	}
}

func TestDispatchSupportsNilResults(t *testing.T) {
	t.Run("nil interface", func(t *testing.T) {
		dispatcher := dispatcherFor(
			t,
			greetBinder{},
			dispatch.HandlerFunc[greetParams, fmt.Stringer](func(context.Context, greetParams) (fmt.Stringer, error) {
				return nil, nil
			}),
		)
		result, err := dispatcher.Dispatch(context.Background(), "test", rawInput{Values: map[string]string{"name": "alice"}})
		if err != nil || result != nil {
			t.Fatalf("expected nil interface result, got result=%#v err=%v", result, err)
		}
	})
	t.Run("nil pointer", func(t *testing.T) {
		dispatcher := dispatcherFor(
			t,
			greetBinder{},
			dispatch.HandlerFunc[greetParams, *greetResult](func(context.Context, greetParams) (*greetResult, error) {
				return nil, nil
			}),
		)
		result, err := dispatcher.Dispatch(context.Background(), "test", rawInput{Values: map[string]string{"name": "alice"}})
		if err != nil || result != nil {
			t.Fatalf("expected nil pointer result, got result=%#v err=%v", result, err)
		}
	})
}

func dispatcherFor[Params, Output any](
	t *testing.T,
	binder dispatch.Binder[rawInput, Params],
	handler dispatch.Handler[Params, Output],
) *dispatch.Dispatcher[rawInput, Output] {
	t.Helper()
	dispatcher := dispatch.New[rawInput, Output]()
	if err := dispatcher.Register("test", dispatch.NewAction(binder, handler)); err != nil {
		t.Fatalf("register action: %v", err)
	}
	return dispatcher
}
