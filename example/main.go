package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/im-wmkong/dispatch"
)

type greetParams struct {
	Name string `json:"name" validate:"required"`
}

type greetAction struct{}

func (greetAction) Bind(_ context.Context, input string) (greetParams, error) {
	var params greetParams
	err := json.Unmarshal([]byte(input), &params)
	return params, err
}

func (greetAction) Handle(_ context.Context, params greetParams) (string, error) {
	return fmt.Sprintf("Hello from struct, %s!", params.Name), nil
}

func main() {
	dispatcher := dispatch.New[string, string]()
	lengthDispatcher := dispatch.New[string, int]()

	err := dispatcher.Register(
		"greet_func",
		dispatch.NewAction(
			dispatch.BinderFunc[string, greetParams](func(_ context.Context, input string) (greetParams, error) {
				var params greetParams
				err := json.Unmarshal([]byte(input), &params)
				return params, err
			}),
			dispatch.HandlerFunc[greetParams, string](func(_ context.Context, params greetParams) (string, error) {
				return fmt.Sprintf("Hello, %s!", params.Name), nil
			}),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	greet := greetAction{}
	if err := dispatcher.Register("greet_struct", dispatch.NewAction(greet, greet)); err != nil {
		log.Fatal(err)
	}
	if err := lengthDispatcher.Register(
		"name_length",
		dispatch.NewAction(
			greet,
			dispatch.HandlerFunc[greetParams, int](func(_ context.Context, params greetParams) (int, error) {
				return len(params.Name), nil
			}),
		),
	); err != nil {
		log.Fatal(err)
	}

	input := `{"name":"Alice"}`
	var output string
	output, err = dispatcher.Dispatch(context.Background(), "greet_func", input)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(output)

	output, err = dispatcher.Dispatch(context.Background(), "greet_struct", input)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(output)

	length, err := lengthDispatcher.Dispatch(context.Background(), "name_length", input)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Name length: %d\n", length)
}
