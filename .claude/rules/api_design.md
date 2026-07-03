---
paths:
  - "api_def/**"
---

# API Design Guide

> 仅当设计或修改 API 接口时读入。从写下第一行 Smithy 到在浏览器里看到可交互的 API 文档，本文覆盖完整流程。
>
> 工作目录：`api_def/`

## 目录

1. [一分钟速览：从模型到文档](#1-一分钟速览从模型到文档)
2. [设计准则](#2-设计准则)
3. [项目布局与文件命名](#3-项目布局与文件命名)
4. [Smithy 编写规范](#4-smithy-编写规范)
5. [公共模型](#5-公共模型)
6. [构建与文档](#6-构建与文档)

---

## 1. 一分钟速览：从模型到文档

整个 API 设计工作流只有三步，工具链已经配好：

**① 写 Smithy 模型** → 在 `api_def/server_api/smithy/` 下创建或修改 `.smithy` 文件。

**② 构建 OpenAPI** → 一行命令把 Smithy 编译成 OpenAPI 规格：

```bash
cd api_def/server_api && smithy build && cd .. && make merge
```

产物落在 `api_def/combined_api/openapi.yaml`。

**③ 预览文档** → 一行命令启动交互式 API 文档：

```bash
npx @scalar/cli document serve api_def/combined_api/openapi.yaml --port 8080
```

浏览器打开 http://localhost:8080，即可浏览和试用 API。

> 如果你只想看文档、不写模型，从第 ③ 步开始即可。如果你还要本地 mock API，`make up` 同时启动 Prism mock（4010）和 Scalar 文档（8010）。

---

## 2. 设计准则

本项目 API 遵循 **RESTful 资源导向**风格，权威参考是 [Google API Design Guide](https://docs.cloud.google.com/apis/design)。以下五条是从中提炼的、每次设计都要落实的核心约定：

**以资源为中心组织 URI。** URL 路径用名词复数表示资源集合，用路径参数定位单个资源：

```
GET    /api/v1/characters          # 列出角色
GET    /api/v1/characters/{id}     # 获取单个角色
POST   /api/v1/characters          # 创建角色
PUT    /api/v1/characters/{id}     # 替换式更新角色
DELETE /api/v1/characters/{id}     # 删除角色
```

不要出现动词式路径（如 `/createCharacter`、`/getCharacterById`）。动作由 HTTP 方法表达。

**列表操作标配分页。** 用 `limit`（每页条数）和 `starting_after`（游标）做基于游标的分页，比基于页码的 offset 分页更适合数据频繁变动的场景。

**字段名 snake_case。** 请求和响应的 JSON key 统一用 `display_name`、`created_at`，不用 `displayName` 或 `createdAt`。

**成功统一返回 200。** 不区分 200/201/204 来编码不同语义，结果在响应体中体现。

**错误体含 code 和 message。** 每次失败响应至少携带机器可读的 `code` 和人类可读的 `message`，由公共模型 `OperationErrors` 统一提供（见 [§5](#5-公共模型)）。

接口定义语言采用 [Smithy 2.0](https://smithy.io/2.0/index.html)，HTTP 协议绑定为 `aws.protocols#restJson1`。Smithy 是 AWS 开源的 API IDL，强类型、可扩展，编译为目标格式（OpenAPI、TypeScript SDK 等）而非手写。

---

## 3. 项目布局与文件命名

```
api_def/
├── server_api/                       # 面向前端的 API（现阶段所有公开 API 在此）
│   ├── smithy/                       # Smithy 源文件
│   │   ├── api_service.smithy        #   服务定义：声明所属 operations
│   │   ├── {resource}_api.smithy     #   一个资源的操作定义
│   │   ├── {resource}_comp.smithy    #   一个资源的结构体、枚举等组件
│   │   └── smithy-build.json        #   构建配置（Maven 依赖 + OpenAPI 插件）
│   └── build/                        # 构建产物（smithy 生成，勿手改）
├── internal_api/                     # 服务间 API，folder-per-service
│   └── {service}/                    #   每个服务独立目录
├── combined_api/                     # 合并后的最终 OpenAPI 规格
│   ├── openapi-merge.json           #   合并配置
│   └── openapi.yaml                 #   最终产物
└── docker-compose.yml               # Prism mock + Scalar docs
```

每个资源刚好两个文件——`api`（操作）加 `comp`（组件），不多不少。资源之间彼此隔离：修改角色 API 不会触碰用户 API 的文件。没有实体资源的"活动"（如身份认证）视为资源处理，同样分配两个文件。

---

## 4. Smithy 编写规范

### 4.1 文件模板

每个 `.smithy` 文件以三段元数据开头——版本、命名约定、命名空间——之后不再重复：

```smithy
$version: "2.0"
$operationInputSuffix: "Input"
$operationOutputSuffix: "Output"

namespace example
```

`$operationInputSuffix` 和 `$operationOutputSuffix` 控制 Smithy 为每个 operation 自动生成的 Input/Output 类型名称后缀，全局统一设置一次即可。

### 4.2 Service 定义

`api_service.smithy` 是操作清单——只声明有哪些操作，不含操作细节：

```smithy
use aws.protocols#restJson1

@restJson1
service APIService {
    version: "1.0.0"
    operations: [
        ListCharacters
        GetCharacter
        CreateCharacter
        UpdateCharacter
        DeleteCharacter
    ]
}
```

新增操作时在这里加一行，Smithy 构建时自动纳入 OpenAPI 输出。

### 4.3 操作定义

每个操作**必须**通过 `with [OperationErrors]` 混入标准错误响应，列表操作**必须**同时混入 `with [PagingParams]`：

```smithy
use common#OperationErrors
use common#PagingParams

// 列表操作：with 两个 mixin，分页参数由 PagingParams 自动注入
@readonly
@http(method: "GET", uri: "/api/v1/characters", code: 200)
operation ListCharacters with [OperationErrors] {
    input := with [PagingParams] {
        @httpQuery("gender")
        gender: String
    }
    output := {
        @httpPayload
        @required
        @contentType("application/json")
        body: ListCharactersResp
    }
}

// 单资源读取：只需 OperationErrors
@readonly
@http(method: "GET", uri: "/api/v1/characters/{character_id}", code: 200)
operation GetCharacter with [OperationErrors] {
    input := {
        @httpLabel
        @required
        character_id: String
    }
    output := {
        @httpPayload
        @required
        @contentType("application/json")
        body: Character
    }
}

// 创建：POST + 自定义请求体
@http(method: "POST", uri: "/api/v1/characters", code: 200)
operation CreateCharacter with [OperationErrors] {
    input := {
        @httpPayload
        @required
        @contentType("application/json")
        body: CreateCharacterReq
    }
    output := {
        @httpPayload
        @required
        @contentType("application/json")
        body: Character
    }
}
```

要点速查：

- `@readonly` 标记在 GET 操作上（语义标注，不影响生成）
- `@httpLabel` 标记路径参数，`@httpQuery` 标记查询参数，`@httpPayload` 标记请求/响应体
- 响应体永远包一层 `output` 结构体，即使直接返回资源对象——这一层是 HTTP 协议的边界，资源对象在 `body` 字段内

### 4.4 模型命名

| 场景 | 规则 | 示例 |
|---|---|---|
| HTTP 请求/响应的包装层 | `{Verb}{Resource}Input` / `Output` | `CreateCharacterInput` |
| 自定义请求体/响应体 | `{Verb}{Resource}Req` / `Resp` | `CreateCharacterReq` |
| 列表响应体 | `List{Resource}Resp`，内含 `{Resource}List` | `ListCharactersResp` |
| 列表类型 | `{Resource}List` | `CharacterList` |
| 列表操作名 | `List` / `Find` / `Search`，不用 `Get` | `ListCharacters` |

当请求体或响应体**就是资源对象本身**时，直接复用，不需要单独的 Req/Resp。例如 Update 操作的 input 在路径中已含 id，请求体就是资源字段的子集，复用 `UpdateCharacterReq` 即可。当响应体就是资源对象时也一样——`output` 的 `body` 直接指向 `Character`。

### 4.5 Create 与 Update 的 id 差异

Create 时资源尚不存在、没有 id；Update 时 id 来自路径。两者共享一份资源结构体会产生矛盾：结构体要不要包含 `id` 字段？

**本项目采用 Google / Stripe 的做法：资源结构体中不包含 id，Update 的 id 完全由路径参数提供。** 这样 Create 和 Update 面对同一份资源结构体，各自所需字段自然自洽——Create 只提供可设置的字段，Update 操作额外带上路径中的 id。

### 4.6 结构体与枚举

```smithy
structure Character {
    id: String
    display_name: String
    @timestampFormat("date-time")
    created_at: Timestamp
}

structure ListCharactersResp with [PageMetadata] {
    @required
    list: CharacterList
}

list CharacterList {
    member: Character
}

enum Gender {
    MALE = "male"
    FEMALE = "female"
    NEUTRAL = "neutral"
}
```

列表响应体必须挂 `with [PageMetadata]`（来自公共模型），为响应注入分页元数据。

### 4.7 审查清单

提交或审查 Smithy 变更时按此清单逐项核对：

- [ ] 文件头：`$version: "2.0"`、namespace、`$operationInputSuffix` / `$operationOutputSuffix`
- [ ] 每个 operation 挂 `with [OperationErrors]`
- [ ] 列表操作挂 `with [PagingParams]`，列表响应体挂 `with [PageMetadata]`
- [ ] HTTP 方法与语义匹配：GET=读、POST=创建、PUT=替换式更新、DELETE=删除
- [ ] URI 路径用名词复数，字段名 `snake_case`
- [ ] 注解正确：`@httpPayload`、`@httpQuery`、`@httpLabel`、`@required`
- [ ] 文档注释使用 `///` 格式

---

## 5. 公共模型

`com.github.ikonglong:smithy-common`（通过 JitPack 拉取，[仓库](https://github.com/ikonglong/smithy-common)）封装了跨资源复用的 Smithy 模型。目前提供三个 mixin：

| Mixin | 挂载位置 | 注入内容 |
|---|---|---|
| `common#OperationErrors` | 每个 operation | 标准 4xx/5xx 错误响应（含 `code` + `message`） |
| `common#PagingParams` | 列表 operation 的 input | 查询参数 `limit`（每页条数）和 `starting_after`（游标） |
| `common#PageMetadata` | 列表响应的结构体 | 分页元数据字段 |

使用三步走——import、声明、挂载：

```smithy
use common#OperationErrors
use common#PagingParams
use common#PageMetadata

operation ListUsers with [OperationErrors] {        // ① 挂错误
    input := with [PagingParams] { ... }            // ② 挂分页参数
    output := { body: ListUsersResp }
}

structure ListUsersResp with [PageMetadata] {       // ③ 挂分页元数据
    @required
    list: UserList
}
```

版本号在 `smithy-build.json` 的 `maven.dependencies` 中声明，当前为 `0.1.0`。

---

## 6. 构建与文档

### 6.1 构建 OpenAPI 规格

**前置条件**：安装 [Smithy CLI](https://smithy.io/2.0/quickstart.html#building-the-model)（需要 Java 运行时）。

构建分两步：Smithy 编译 → 合并为最终文件。

```bash
cd api_def/server_api && smithy build
cd .. && make merge
```

- `smithy build` 读取 `smithy-build.json`，编译 `server_api` 投影，输出 `server_api/build/smithy/server-api/openapi/APIService.openapi.json`
- `make merge` 用 `openapi-merge-cli` 将各服务规格合并为 `combined_api/openapi.yaml`，并将 `openapi` 版本号修正为 `3.1.1`

构建配置的核心在 `server_api/smithy-build.json`——声明了源文件目录、Maven 依赖（Smithy OpenAPI 插件 + smithy-common）和 JitPack 仓库地址。新增 smithy-common 版本时修改此文件中的版本号即可。

### 6.2 启动文档服务

**本地快速启动**（无需 Docker）：

```bash
npx @scalar/cli document serve api_def/combined_api/openapi.yaml --port 8080
```

**Docker 启动**（同时启动 mock）：

```bash
cd api_def && make up
```

Docker 模式下同时启动两个服务：

| 服务 | 地址 | 用途 |
|---|---|---|
| Scalar Docs | http://localhost:8010 | 交互式 API 文档 |
| Prism Mock | http://localhost:4010 | 以 OpenAPI 为规格的 mock server，支持 CORS |
