# README

## 参考资料

[Install Smith Cli & Build API Models](https://smithy.io/2.0/quickstart.html#building-the-model)

[Smithy 2.0](https://smithy.io/2.0/index.html)

[Smithy examples](https://github.com/smithy-lang/smithy-examples)

## 目录结构说明

```plain text
api_def/                                     # API 定义
├── README.md                                # 说明文档
├── server_api/                              # Medeo V2 服务端 API
│   ├── api_service.smithy                   # Service 定义
│   ├── {resource}_api.smithy                # 一个资源的 API 定义
│   ├── {resource}_comp.smithy               # 与一个资源相关的组件定义
│   └── smithy-build.json
└── internal_api/                            # 内部服务 API
    └── consumption/                         # 消费服务
        ├── consumption_service.smithy       # 服务定义
        ├── consumption_api.smithy           # 操作定义
        └── consumption_comp.smithy          # 组件定义
```

api_def/server_api/smithy-build.json

**为方便前端简单从 IDL 生成直接可用的代码，现阶段将所有暴露给前端的 API 定义都放入 api_def/server_api/smithy 目录下。** 以后有精力的时候再探索更好的组织形式。

**内部服务采用 folder-per-service 方式组织，便于未来的微服务化演进。** 各服务独立定义，为后续引入服务网格、服务发现、熔断器以及 gRPC/Thrift 等高效协议预留扩展空间。

根据资源名称命名相关文件：
- 名称为 {resource name}_api 的文件包含指定资源的 api operation 定义
- 名称为 {resource name}_comp 的文件包含与指定资源相关的组件定义
- 每个资源的 API 定义由两个文件组成：api、comp。不同资源间彼此隔离

有些场景不存在资源，可能是一个活动包含一系列操作，此时可将活动看作资源。例如，identity 下的 Authentication 就不是一个资源，而一个活动，包含多个操作。

## 模型的命名

HTTP 请求、响应的命名分别以 Input、Output 为后缀。如果 HTTP 请求体、响应体无法直接复用资源对象，就得自定义它们，它们的命名分别以 Req、Resp 为后缀。例如：

```plain text
$version: "2.0"
// 自定义从 smithy 生成的 Input、Output 类型的名称后缀
$operationInputSuffix: "Input"
$operationOutputSuffix: "Output"

namespace examples

operation VerifyJwt {
    // 如果 http req 类型不会被重用，可直接在这里定义匿名的内联（inline）类型。
    // 生成代码时，内联类型的名称可通过 55、56 行所示的控制语句进行设置
    input := {
        @httpPayload
        @required
        @contentType("application/json")
        // 注意，即使 VerifyJwtReq 不会被重用，smithy 目前也不支持内联定义它。
        // 只能内联定义 operation 的 Input 和 Output
        body: VerifyJwtReq
    }

    // 如果 http req 类型会被重用，则应该提供独立的 http req 定义，不应该使用内联类型。
    // input: VerifyJwtInput

    // output 也支持内联定义
    output: VerifyJwtOutput
    errors: [
        HTTP4xxResp
        HTTP5xxResp
    ]
}

structure VerifyJwtInput {
    @httpPayload
    @required
    @contentType("application/json")
    body: VerifyJwtReq
}

structure VerifyJwtOutput {
    @httpPayload
    @required
    @contentType("application/json")
    body: VerifyJwtResp
}
```

如果 HTTP 请求体和响应体就是资源对象，那么其命名就是资源名称。例如：

```plain text
operation UpdateUser {
    input: UpdateUserInput
    output: UpdateUserOutput
    errors: [
        HTTP4xxResp
        HTTP5xxResp
    ]
}

structure UpdateUserInput {
    /// 用户 ID (带前缀的ULID: usr_{ULID})
    @httpLabel
    @required
    @pattern("^usr_[0-9A-HJKMNP-TV-Z]{26}$")
    id: String

    @httpPayload
    @required
    @contentType("application/json")
    body: User
}

structure UpdateUserOutput {
    @httpPayload
    @required
    @contentType("application/json")
    body: User
}

/// User Resource
structure User {
    ...
}
```

对于列表的操作，相关模型的命名不应以 Get 开始，应以 List、Find 或 Search 开始，根据具体场景选择恰当的前缀：

```plain text
operation ListUsers {
    input: ListUsersInput
    output: ListUsersOutput
    ...
}

structure ListUsersInput {
    ...
}

structure ListUsersOutput {
    @httpPayload
    @required
    @contentType("application/json")
    body: ListUsersResp
}

structure ListUsersResp {
    items: UserList
}
```

对于列表模型，命名使用 List 后缀：

```plain text
list UserList {
    member: Track
}
```

## Create 和 Update 共用资源，但对 id 字段的校验不同，如何处理？

试验了下，smithy 不支持有条件的校验声明（不同操作，校验不同）。
**为了让生成的 API SDK 切实可用**，有两个解决办法：
1. 将资源中可共用的部分定义为一个 Mixin，再使用混入语法 `with <Mixin>` 为 Update 定义必须包含 id 的资源
2. 对于 Update 操作，路径中已包含资源 id，因此资源中就可不包含资源 id 了，这样两个操作对资源的需求就统一了。

Google，Stripe 的 API 设计采用的是方案二。因此**采用方案二**。

## 怎么跑 mock-api (currently serving combined_api/openapi.yaml)

### Prerequisition

需要本地有 docker runtime.
(私货推荐 **colima**, 比 docker desktop 的 HyperKit/Hyper-V 占用资源低一半, 开源, 基于 Lima VM. 启动快快快! 
是我在 macOS 上最喜欢的 docker/containerd runtime 提供工具.)

### 启动
更多功能详见Makefile.
```
make up
```
### 服务端口
- Prism Mock: `http://localhost:4010`
- Scalar Docs: `http://localhost:8010`

## API 设计规范

编写和审查 Smithy 文档时，需关注以下维度。

### Smithy 语法与结构

- 命名约定：服务、操作、结构体、字段名遵循驼峰命名法
- 版本声明：确认有正确的 `$version: "2.0"` 声明
- 命名空间：namespace 定义合理且一致
- 导入语句：use 语句正确且必要
- 注释规范：文档注释使用 `///` 格式

### HTTP 映射

- HTTP 方法正确使用 GET/POST/PATCH/PUT/DELETE
- URI 路径符合 RESTful 设计原则
- `@httpQuery`、`@httpPayload` 使用合理
- HTTP 状态码定义合适

### Google API 设计指南合规性

- 资源导向设计：API 以资源为中心
- 标准方法：正确使用标准 CRUD 操作
- 集合名使用复数形式
- 方法名使用动词+名词格式
- 字段名使用 snake_case（如果适用）
- 列表操作包含分页支持
- 错误码和错误消息符合 Google 标准

### 数据类型和验证

- `@required` 注解使用合理
- `@pattern`、`@range` 等验证注解正确
- 数据类型定义恰当
- 枚举定义清晰完整

### 一致性

- 跨文件命名和设计一致
- API 版本定义一致
- 错误处理方式统一
