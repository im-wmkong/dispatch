# dispatch

`dispatch` 是一个与传输协议无关的静态 Action 分发核心。

## 设计模型

```text
Registry<I>
└── action name → Invoker<I>

Dispatcher<I>
└── Dispatch(ctx, action, input I)
        ├── Registry lookup
        └── Invoker
                ├── Binder<I, P>
                ├── validator/v10
                └── Handler<P, R>
```

- 一个 `Registry` 固定一种输入类型 `I`。
- 一个应用可以创建多个不同输入类型的 `Registry`。
- `Binder` 决定如何把输入绑定为业务参数。
- `Handler` 只处理完成绑定和校验后的强类型参数。
- `P` 和 `R` 在 Binder/Handler 内保持编译期类型安全。
- 字符串动态分发决定了 `Dispatch` 的统一出口为 `any, error`。
- 创建 `Dispatcher` 时 Registry 被封存；运行期间只能查询，不能注册或替换 Action。
- 核心包不依赖 HTTP、JSON 或 RPC。

## 公共接口

```go
type Binder[I, P any] interface {
    Bind(ctx context.Context, input I) (P, error)
}

type Handler[P, R any] interface {
    Handle(ctx context.Context, params P) (R, error)
}
```

组装和调用：

```go
registry := dispatch.NewRegistry[Input]()

err := dispatch.Register(
    registry,
    "example",
    exampleBinder,
    exampleHandler,
)

dispatcher, err := dispatch.NewDispatcher(registry)
output, err := dispatcher.Dispatch(ctx, "example", input)
```

泛型参数由 Go 根据 Binder 和 Handler 自动推导，注册处不需要显式填写。简单逻辑可以直接使用 `BinderFunc` 和 `HandlerFunc`。

## 职责边界

框架负责：

- 静态注册和名称冲突检查；
- 创建 Dispatcher 时封存 Registry；
- 按 action 名称查询 Invoker；
- 编排 Bind、Validate、Handle；
- 使用 `validator/v10` 统一校验业务参数；
- 在内部完成不同参数类型和结果类型的类型擦除。

业务负责：

- 定义入口使用的 RawInput；
- 决定 Binder 如何解析或转换 RawInput；
- 定义参数结构及 `validate` 标签；
- 实现 Handler 业务逻辑；
- 处理 Dispatcher 返回的具体业务结果和错误。
