# Dispatch 🚀

[![Go Reference](https://pkg.go.dev/badge/github.com/im-wmkong/dispatch.svg)](https://pkg.go.dev/github.com/im-wmkong/dispatch)
[![Go Report Card](https://goreportcard.com/badge/github.com/im-wmkong/dispatch)](https://goreportcard.com/report/github.com/im-wmkong/dispatch)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

**Dispatch** 是一个基于 Go 泛型的强类型 Action 分发库。

通过名称选择 Action，将**参数绑定 → 参数校验 → 业务执行**串成统一流程。每个 Action 可以使用自己的参数结构体，对外保持统一的输入与输出类型，让业务代码专注于处理已经绑定、校验过的参数。

适合需要按名称调用不同业务动作的场景，例如命令入口、消息处理和工具调用。

## ✨ 核心特性

- 🛡️ **强类型分发** —— `Dispatcher[Input, Output]` 明确输入与返回值类型，`NewAction` 在编译期检查 Binder 与 Handler 的参数类型是否一致。
- 🧩 **可组合的绑定与处理** —— `Binder` 负责转换输入，`Handler` 负责业务逻辑；支持结构体实现，也支持 `BinderFunc` / `HandlerFunc` 函数适配器。
- ✅ **自动参数校验** —— 内置 [go-playground/validator](https://github.com/go-playground/validator)，通过 `validate` 标签声明规则，校验通过后才执行 Handler。
- 🔄 **并发注册与注销** —— 支持并发调用 `Register`、`Unregister` 和 `Dispatch`；重复名称会报错，注销后可以重新注册。
- 📦 **独立或共享注册表** —— `New` 创建独立 Dispatcher，`NewRegistry` + `NewDispatcher` 让多个 Dispatcher 共享同一组 Action。

## 📦 安装

需要 **Go 1.22 或更新版本**。

```bash
go get github.com/im-wmkong/dispatch
```

## 🗺️ 能力总览

| 组件 | 核心职责 | 关键能力 |
| :--- | :--- | :--- |
| **`Dispatcher[Input, Output]`** | 管理并执行 Action | `New` 创建独立实例；`Register / Unregister / Dispatch` 注册、注销和调用。 |
| **`Registry[Input, Output]`** | 存储命名 Action | `NewRegistry` 创建注册表；`Register / Unregister` 管理名称映射；通过 `NewDispatcher` 共享。 |
| **`Action[Input, Output]`** | 组合绑定与处理 | `NewAction(binder, handler)` 自动推导泛型参数；注册时检查定义是否合法。 |
| **`Binder[Input, Params]`** | 输入转换 | `Bind(ctx, input)` 返回 Action 的参数；`BinderFunc` 适配普通函数。 |
| **`Handler[Params, Output]`** | 业务执行 | `Handle(ctx, params)` 返回结果；`HandlerFunc` 适配普通函数。 |

→ 完整 API 参考：[pkg.go.dev](https://pkg.go.dev/github.com/im-wmkong/dispatch)。

## 🚀 快速开始

以 JSON 字符串作为输入，注册一个 `greet` Action：

1. 定义带校验标签的参数结构体。
2. 用 `NewAction` 连接 Binder 与 Handler，并注册到 Dispatcher。
3. 通过 `Dispatch` 按名称调用，直接获得 `string` 结果。

将下面的代码保存为 `main.go`：

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/im-wmkong/dispatch"
)

type GreetParams struct {
	Name string `json:"name" validate:"required,min=2"`
}

func main() {
	dispatcher := dispatch.New[string, string]()

	action := dispatch.NewAction(
		dispatch.BinderFunc[string, GreetParams](func(_ context.Context, input string) (GreetParams, error) {
			var params GreetParams
			err := json.Unmarshal([]byte(input), &params)
			return params, err
		}),
		dispatch.HandlerFunc[GreetParams, string](func(_ context.Context, params GreetParams) (string, error) {
			return fmt.Sprintf("Hello, %s!", params.Name), nil
		}),
	)
	if err := dispatcher.Register("greet", action); err != nil {
		log.Fatal(err)
	}

	result, err := dispatcher.Dispatch(context.Background(), "greet", `{"name":"Alice"}`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)
}
```

```bash
go run main.go
```

输出：

```text
Hello, Alice!
```

> **💡 提示**：JSON 解析由示例中的 Binder 实现。`Input` 也可以是 `[]byte`、请求对象或自定义结构体，只要 Binder 的输入类型与 Dispatcher 一致即可。

### 调用流程

```text
Dispatch(ctx, name, input)
  → 查找 Action
  → Binder.Bind(ctx, input)
  → validator.StructCtx(ctx, params)
  → Handler.Handle(ctx, params)
  → 返回 Output
```

任一步骤返回错误都会停止后续流程。调用失败时返回 `Output` 的零值与错误，即使 Handler 同时返回了部分结果，该结果也会被丢弃。

## 💡 常用模式

以下片段沿用快速开始中的类型和变量；按需补充导入。

### 用结构体组织 Action

同一个结构体可以同时实现 Binder 与 Handler，也可以将两者拆成独立对象。需要注入数据库、客户端等依赖时，直接放在结构体字段中。

```go
type GreetAction struct{}

func (GreetAction) Bind(_ context.Context, input string) (GreetParams, error) {
	var params GreetParams
	err := json.Unmarshal([]byte(input), &params)
	return params, err
}

func (GreetAction) Handle(_ context.Context, params GreetParams) (string, error) {
	return fmt.Sprintf("Hello from struct, %s!", params.Name), nil
}
```

```go
greet := GreetAction{}
if err := dispatcher.Register("greet_struct", dispatch.NewAction(greet, greet)); err != nil {
	log.Fatal(err)
}
```

### 复用 Binder，使用不同的参数类型

同一个 Dispatcher 中的 Action 可以拥有不同的 `Params` 类型。对于 JSON 输入，可以自行实现一个通用 Binder：

```go
type JSONBinder[Params any] struct{}

func (JSONBinder[Params]) Bind(_ context.Context, input string) (Params, error) {
	var params Params
	err := json.Unmarshal([]byte(input), &params)
	return params, err
}

type AddParams struct {
	A int `json:"a" validate:"gte=0"`
	B int `json:"b" validate:"gte=0"`
}
```

```go
if err := dispatcher.Register("add", dispatch.NewAction(
	JSONBinder[AddParams]{},
	dispatch.HandlerFunc[AddParams, string](func(_ context.Context, params AddParams) (string, error) {
		return fmt.Sprint(params.A + params.B), nil
	}),
)); err != nil {
	log.Fatal(err)
}

sum, err := dispatcher.Dispatch(context.Background(), "add", `{"a":2,"b":3}`)
if err != nil {
	log.Fatal(err)
}
fmt.Println(sum) // 5
```

这里 `greet` 使用 `GreetParams`，`add` 使用 `AddParams`，两者都接收 `string`、返回 `string`。需要不同的输出类型时，可以创建另一个 Dispatcher，例如 `dispatch.New[string, int]()`。

### 多个 Dispatcher 共享注册表

```go
registry := dispatch.NewRegistry[string, string]()
if err := registry.Register("greet", action); err != nil {
	log.Fatal(err)
}

first, err := dispatch.NewDispatcher(registry)
if err != nil {
	log.Fatal(err)
}
second, err := dispatch.NewDispatcher(registry)
if err != nil {
	log.Fatal(err)
}

// 两个 Dispatcher 都可以执行同一组 Action。
for _, d := range []*dispatch.Dispatcher[string, string]{first, second} {
	result, err := d.Dispatch(context.Background(), "greet", `{"name":"Alice"}`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)
}
```

通过 Registry 或任一共享它的 Dispatcher 注册、注销 Action，其他 Dispatcher 的后续查找都会看到变化。每次调用 `New` 则创建独立的注册表。

### 区分绑定、校验与业务错误

```go
// 空 name 可以完成 JSON 解析，但无法通过 required 校验。
_, err = dispatcher.Dispatch(context.Background(), "greet", `{"name":""}`)
switch {
case errors.Is(err, dispatch.ErrActionNotFound):
	log.Print("unknown action")
case errors.Is(err, dispatch.ErrBind):
	log.Print("invalid input format")
case errors.Is(err, dispatch.ErrValidate):
	var fields validator.ValidationErrors
	if errors.As(err, &fields) {
		for _, field := range fields {
			log.Printf("%s failed %s", field.Field(), field.Tag())
		}
	}
case err != nil:
	log.Printf("action failed: %v", err)
}
```

该片段需要导入 `errors` 和 `github.com/go-playground/validator/v10`。

| 错误 | 触发条件 |
| :--- | :--- |
| **`ErrInvalidAction`** | 名称为空或带首尾空白、Action 零值、Binder / Handler 为 nil、参数类型不合法，或使用 nil / 未初始化的 Dispatcher、nil Registry。 |
| **`ErrDuplicateAction`** | 注册表中已经存在同名 Action。原有 Action 保持不变。 |
| **`ErrActionNotFound`** | 调用或注销未注册的 Action。 |
| **`ErrBind`** | Binder 返回错误。可以继续用 `errors.Is` / `errors.As` 取得原始原因。 |
| **`ErrValidate`** | 参数校验失败。保留底层 validator 错误。 |
| **Handler 返回的错误** | 业务处理失败。保留原始错误，并由 Dispatcher 补充 Action 名称。 |

### 注销 Action

```go
if err := dispatcher.Unregister("greet"); err != nil {
	log.Fatal(err)
}

_, err = dispatcher.Dispatch(context.Background(), "greet", `{"name":"Alice"}`)
fmt.Println(errors.Is(err, dispatch.ErrActionNotFound)) // true
```

`Unregister` 删除名称映射后立即返回。已经查找到该 Action 的调用会继续执行；资源释放由业务代码负责。注销后可以用同一名称重新注册，重复注销返回 `ErrActionNotFound`。

## 🚧 使用约定

- **参数类型**：`Params` 必须是结构体或结构体指针。Binder 与 Handler 的参数类型一致性由编译器检查，结构体限制在注册时检查；`Action` 应通过 `NewAction` 构造。
- **类型边界**：同一 Registry / Dispatcher 内的 Action 必须具有一致的 `Input` 和 `Output` 类型，`Params` 可以不同。
- **校验规则**：使用 validator 的 `validate` 标签，并启用 `WithRequiredStructEnabled`，`required` 也会检查非指针结构体的零值。当前公开 API 使用默认校验器。
- **并发使用**：注册表操作支持并发，Binder 与 Handler 会在调用方的 goroutine 中执行；其中的共享可变状态需要自行同步。
- **Context 传递**：同一个 `ctx` 会传入绑定、校验和处理流程。Binder / Handler 及其下游调用需要自行响应取消和超时。

## 📚 示例

完整可运行示例见 [`example/main.go`](example/main.go)，包含函数适配器、结构体 Action，以及返回 `int` 的 Dispatcher。

在仓库根目录运行：

```bash
go run ./example
```

## 🤝 参与贡献

欢迎提交 Issue 与 Pull Request。提交前请运行：

```bash
go test -race ./...
```

## 📄 开源协议

[MIT License](LICENSE)。
