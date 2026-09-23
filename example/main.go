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
	registry := dispatch.NewRegistry[string]()

	err := dispatch.Register(
		registry,
		"greet_func",
		dispatch.BinderFunc[string, greetParams](func(_ context.Context, input string) (greetParams, error) {
			var params greetParams
			err := json.Unmarshal([]byte(input), &params)
			return params, err
		}),
		dispatch.HandlerFunc[greetParams, string](func(_ context.Context, params greetParams) (string, error) {
			return fmt.Sprintf("Hello, %s!", params.Name), nil
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	greet := greetAction{}
	if err := dispatch.Register(registry, "greet_struct", greet, greet); err != nil {
		log.Fatal(err)
	}

	dispatcher, err := dispatch.NewDispatcher(registry)
	if err != nil {
		log.Fatal(err)
	}

	input := `{"name":"Alice"}`

	funcOutput, err := dispatcher.Dispatch(context.Background(), "greet_func", input)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(funcOutput)

	structOutput, err := dispatcher.Dispatch(context.Background(), "greet_struct", input)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(structOutput)
}
