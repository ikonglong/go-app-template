# 错误日志：msg 写什么、请求上下文记到哪里 —— 讨论记录与共识

> 围绕「写 error 日志时 slog 的 `msg` 参数怎么取」与「REST 边界要不要把 HTTP
> 请求一起记进日志」两个具体问题展开。既是结论，也保留推导过程，便于日后回看
> 为什么这么定。
>
> 适用范围：`internal/common/log`（包别名 `applog`）的 `*Attrs` 调用方，尤其是
> `internal/adapter/rest` 边界层。配套阅读：`docs/logging-design.md`（logger 装配）、
> `.claude/error_handling.md`（错误模型）。

## 目录

1. [前提：「错误描述」有两个承载位置](#1-前提错误描述有两个承载位置)
2. [问题一：error 日志的 `msg` 该重写还是取 error message](#2-问题一error-日志的-msg-该重写还是取-error-message)
3. [关于 stacktrace](#3-关于-stacktrace)
4. [问题二：REST 边界记录 HTTP 请求](#4-问题二rest-边界记录-http-请求)
5. [共识速查](#5-共识速查)

---

## 1. 前提：「错误描述」有两个承载位置

两个问题之所以容易纠结，根因是「错误的文字描述」在本项目里其实落在**两个不同
的位置**，分工不同，先分清它们后面就顺了。

| 位置 | 是什么 | 谁写的、回答什么 |
|---|---|---|
| slog 的 `msg` | `applog.ErrorAttrs(ctx, event, msg, ...)` 的第三个参数，即 slog record 的 `msg` 字段 | 日志**调用点**写的，回答「我此刻在记录哪个动作的失败」 |
| 结构化 `message` attr | 由 `ErrAttrs(err)` 从 `AppError.Message()` 提取（见 `internal/common/log/err_attrs.go`） | 错误**诞生处**写的，回答「什么坏了」 |

`renderError` 现状（`internal/adapter/rest/error_resp.go:38`）：

```go
applog.ErrorAttrs(ctx, appErr.Event(), "request failed", applog.ErrAttrs(err)...)
```

`ErrAttrs` 已经把 `code` / `case` / `message`（以及 `RemoteError` 的
`service` / `operation` / `status` / `body_code` / `body_message` /
`retry_after`）拆成结构化字段。也就是说，**错误自带的描述已经进日志了**，走的是
结构化 `message`，不经过 `msg`。

---

## 2. 问题一：error 日志的 `msg` 该重写还是取 error message

**共识：`msg` 按调用场景写一个稳定、低基数的短语（如 `"request failed"`），或者
直接留空；不要复制 `error.Message()`。**

三条理由：

1. **会重复。** `ErrAttrs` 已经输出 `message`，`msg` 再取一遍 `error.Message()`，
   同一条记录里就有两份相同文本。`internal/common/log/log.go` 的 wrapper 注释已经
   明说这点：`msg` 「MAY be empty — common for failure logs where ErrAttrs already
   supplies the `message` field… and `msg` would just duplicate it」。

2. **角色不同。** `message`（结构化）是错误视角——「什么坏了」，诞生于错误产生处；
   `msg` 是日志点视角——「我在记录哪个动作的失败」。在 REST 边界，这个动作就是
   「一个请求失败了」，所以固定写 `"request failed"` 即可，它本就不该随错误内容
   变化。

3. **可聚合维度另有其物。** 真正用来做仪表盘 pivot 的是 `event`（主键）+
   `code` / `case`（聚合标签），全是结构化、低基数字段。`msg` 是给人扫读的，不该
   承担聚合职责，保持低基数甚至为空都没有损失。

> 一句话：error 的 message/description **必须**进日志，但走结构化 `message`
> attr（项目已做），不走 slog 的 `msg`。

---

## 3. 关于 stacktrace

讨论中提到「想顺便输出 stacktrace」，这里先纠正一个前提，再给取向。

**事实：`apperror.AppError` 已携带 stacktrace。** `go-apperror` v0.1.0 的
`AppError` 在构造时通过 `runtime.Callers` 采集调用栈，暴露 `StackTrace()` 方法。
`RemoteError` 同理。所以「直接取 error 的 stacktrace」已经是可行的——需要做的
只是在日志侧把它渲染出来。

由此推导出的取向：

- **slog 的 `AddSource` 不顶用。** 它给的是日志**调用点**（`renderError` 那一行）
  的 `source`，所有错误都指向同一处，诊断价值近乎零。它适合定位「哪行代码在打
  日志」，不适合定位「错误从哪冒出来」。
- **本设计本来就不靠 stack 定位。** 用 `event`（操作名）做主键定位「哪个操作」，
  `code` / `case` 定位「什么失败」，`cause` 链（`WithCause`）保留根因。对「已分类
  的预期错误」，这套维度通常比一长串 stack 更精准。
- **真要 stack，只对未分类错误有意义**——panic、或从外部 wrap 进来的陌生 error，
  也就是走 `rest.unhandled` → `InternalError` 那条路的情况。即便要采，也作为**独立
  attr**（如 `slog.String("stack", …)`），**绝不塞进 `msg` 或 `message`**：它含
  换行，会触发 `error_handling.md`《Anti-Patterns》#4（日志
  聚合器按换行切记录，一条错误会被拆成多条）。

**结论（已修订）：决定引入 stacktrace，但用最轻的方式。** 本节早先的结论是「暂不
引入」（理由：对当时的 AppError 体系属过度设计）。在目标明确为「记 error 时输出
整条 chain 含 stack」、且本项目将承载复杂业务并临近投产后，方向改为**引入**——给
自有库 `go-apperror` 加 leaf-only stack 捕获 + `StackTrace()` 兼容接口，写日志时
walk 整条链、逐层把 stack 作为独立结构化 attr 输出。完整调研与论据见
`docs/error-stacktrace-logging-research.md`。

本节的两条核心约束在新方案里依然成立并被吸收：① slog 的 `AddSource` 不顶用（指向
日志调用点而非错误产生点），所以必须在 `apperror` 构造处采栈；② stack 只作为独立
结构化 attr，绝不进 `msg` / `message`（`\n` 会被聚合器切记录）。

---

## 4. 问题二：REST 边界记录 HTTP 请求

### 4.0 先看现状：method/url 其实已经在记了

`RequestLogger` 中间件（`internal/adapter/rest/request_logger.go`）对每个请求
（`/health/*` 除外）发一条 INFO access log，含 `method` / `path` / `status` /
`latency`，并且与 error 行**共享同一个 `request_id`**。所以 error 行和 access log
行本就能靠 `request_id` 关联（join）起来。

这点改变了下面每一项的「必要性」判断——很多信息不是「没有」，而是「在另一条
关联记录里」。

### 4.1 method + url —— 必需，但不是「必须在 error 行重复」

- 必需，没有异议。
- 严格讲，靠 `request_id` 已能从 access log 关联到 method/path，error 行不重复也
  不会丢信息。
- 但在 error 行**补一份**成本极低（`method` 低基数、`path` 可聚合），省去 join、
  单行即可诊断，**值得做**——作为结构化 attr 加，不进 `msg`。

### 4.2 完整 req body —— 没必要，而且在本项目里是安全事故

- **必要性低。** 诊断靠 `event + code + case + message + request_id` 基本够用；
  完整 body 只在排查「输入相关 bug」时偶尔有价值。
- **问题（严重）：**
  1. **敏感数据泄露。** `signUpReq` 里就有明文 `Password`（`internal/adapter/rest/account.go:37`）。
     无条件记完整 body＝**明文密码进日志**。`error_handling.md` §3.5 / §2.4
     明确禁止无条件记录 `Request.Body`。
  2. **体积 / 成本。** body 可能很大，放大日志量与存储成本。
  3. **高基数。** 自由数据，无法用于聚合。
- **结论：** 不要无条件记完整 req body。若确有需要，必须**脱敏**（至少 redact 掉
  password 等字段），并限定在 DEBUG 或排查开关下；常规只记 method + path，顶多附
  少量已知安全的标识（如 account id）。

### 4.3 完整响应 —— 看「响应」指哪个，inbound 这里都不该记

先分清两种「响应」：

- **我们返回给 client 的响应**（`errorResp`）：**不必记**。它就是 `code` /
  `message` / `case` 拼出来的，这些已在 error 行里；`status` 已在 access log。
  记它纯属重复。
- **远程服务返回给我们的响应**（`RemoteError.Response`）：那是 **driven adapter 层**
  的职责，不是 inbound REST 层的事。其 `status` / `body_code` / `body_message`
  已由 `ErrAttrs` 输出；完整 `Response.Body` 仅 forensics 用，同样有敏感 / 体积
  问题，guide 也说它「often nil to avoid logging sensitive bodies」。

你问的 inbound 场景里，「响应」＝给 client 的那个 → **不记录**。

### 4.4 这些数据放 `AddNote()` 还是 `msg`？—— 两个都不放

**正确答案：作为独立的结构化 `slog.Attr` 记录。** 逐一说明为什么排除另两个：

1. **不放 `msg`。** `msg` 是自由文本、给人读的字段，塞 method/url/body 进去会变成
   不可聚合的长串；body 一旦含换行还会破坏日志切分（§6.3）。

2. **不放 `AddNote`。** `AddNote` 改的是 **`AppError.Message()`**（`" -> "`
   前置拼接，见库源码 `apperror.go` 的 `AddNote`），它属于**错误语义**，用途是
   「同一错误向上传递时补一层语境」，如 `"loading user during checkout"`
   （§4.1 / §4.3）。把请求 metadata 塞进去有三重坏处：
   - **污染 `Message()`**——而 `Message()` 经 `ErrAttrs` 进 `message` 字段，且
     sanitized response 也用它（`error_resp.go:47`），可能把请求细节甚至敏感数据
     漏给 client；
   - **高基数进 message**——破坏按 `code` / `case` 的聚合，也破坏按 message 的去重；
   - **层次不对**——`AddNote` 在底层调用点用，而完整 req 只有边界才有，把边界
     metadata 倒灌进底层错误是耦合。

3. **应放结构化 attr，且和错误字段各自装进分组（`req` / `err`）。** slog 原生支持
   嵌套对象——`slog.Group(key, attrs...)` 返回一个 `slog.Attr`，JSON handler 下渲染成
   `"req":{"method":"POST","url":"/accounts"}`，text handler 下渲染成点号扁平形式
   `req.method=POST req.url=/accounts`。分组而非平铺有两个好处：同类字段命名空间内聚、
   一眼看出归属；不同来源（错误 vs 请求）也不会在顶层混成一片。

   对应地，错误字段也从平铺改为 `err` 分组：`applog.ErrGroup(err)` 把 `ErrAttrs`
   产出的 `code` / `case` / `message`（以及 RemoteError 的远程字段）包进一个 `err`
   组。`ErrAttrs` 仍保留为产「平铺字段列表」的底层构件，`ErrGroup` 是它的分组封装
   （内部 `slog.GroupValue(ErrAttrs(err)...)`，直接展开 `[]slog.Attr`，无需转 `[]any`）。

   `renderError` 落地（`internal/adapter/rest/error_resp.go`）：

   ```go
   applog.ErrorAttrs(ctx, appErr.Event(), "request failed",
       applog.ErrGroup(err),
       slog.Group("req",
           slog.String("method", string(c.Method())),
           slog.String("url", string(c.URI().RequestURI())),
       ),
   )
   ```

   产出的 JSON 形如：

   ```json
   {
     "level": "ERROR",
     "msg": "request failed",
     "event": "account.create",
     "err": { "code": "ALREADY_EXISTS", "message": "...", "case": "..." },
     "req": { "method": "POST", "url": "/accounts" },
     "request_id": "..."
   }
   ```

   注意 `event` 仍留在**顶层**作为主聚合键（不进 `err` 组）；`err` / `req` 是并列的
   两个嵌套对象。`url` 取 `URI().RequestURI()`（path + query），丢掉恒定的 scheme/host
   噪音；若担心 query 携带敏感参数，降级到 `c.Path()`（仅 path，与 access log 一致）。

### 4.5 分组的边界：access log 不嵌套

承上，自然会问：access log 那条 INFO 行（`request_logger.go` 的 "request handled"）
要不要也把 method/path 收进 `req` 组，跟 error 行保持一致？**不要。**

判断标准是：**分组是为隔离「多个来源」的字段，单一来源不分组。**

- error 行同时有错误字段（`err`）和请求字段（`req`）两类来源，顶层混在一起既难分辨
  归属、又可能撞 key（如 RemoteError 的远程 `status` 撞请求语义的 status），所以分组
  隔离。
- access log 只有**单一来源**——就是这次访问的 `method` / `path` / `status` /
  `latency`，没有第二类字段跟它抢顶层。套一个 `req` 组只是徒增一层：没有隔离价值，
  字段还更难查（`req.method` vs `method`）；何况 `status` / `latency` 属于响应 / 时延，
  硬塞进 `req` 反而语义错位。

所以 access log **保持平铺**，不为「跟 error 行形式统一」而嵌套——一致性应服务可读性，
不是形式对齐。两条日志结构不同，恰好如实反映它们内容来源数不同（多源 vs 单源），这是
合理差异。唯一代价是跨日志聚合时字段名不一致（access 是 `method`、error 是
`req.method`），可接受——二者本就靠 `request_id` join，access log 才是请求的权威记录，
error 行的 `req` 只是单行自足诊断的便利冗余。

### 4.6 健康检查失败也走统一的 errorResp

readiness 探针（`health_check.go` 的 `Ready`）失败原先自己渲染 `HealthCheckResponse`
并平铺记 `slog.String("error", err.Error())`，是一条独立于 `renderError` 的错误路径。
为「错误记录保持一致」，把它统一收进 `renderError`：

```go
if err := h.dbPing(pingCtx); err != nil {
    renderError(ctx, c, apperror.NewUnavailable(healthReadyEvent,
        apperror.WithMessage("database not ready"),
        apperror.WithCause(err)))
    return
}
```

**为什么 kubelet 不受影响**：HTTP readiness probe **只看状态码**（2xx/3xx = ready，
其余 = not ready），**不解析响应体**。`CodeUnavailable` 映射到 503，所以失败仍是 503、
成功仍是 200，判定与改前完全一致。响应体从 `HealthCheckResponse` 变成 `errorResp`
对探针无感。

**代价与归宿**：失败响应不再带 per-dependency 的 `Checks` 明细——但这本就该看日志，
不是看探针响应体（kubelet 不展示它给人）。诊断细节经 `renderError` 进结构化日志。

**顺带暴露并修掉的根因**：`ErrAttrs` 当初不输出 `cause`，直接套 `renderError` 会丢掉
db ping 的根因（以及任何 `WithCause` 包装的根因，account / session 路径同样受影响）。
按 guide §3.4「log them server-side」，给 `ErrAttrs` 补上 `cause` 字段——**仅进日志**，
`errorResp` 仍只暴露 `code` / `case` / `message`（`error_resp.go` 不读 cause，sanitize
不变）。这样 readiness 失败的 db 错误以 `err.cause` 留在日志里，诊断不丢；这条修复
惠及所有走 `renderError` 的路径，不止健康检查。

> 这也顺带解决了「响应体把 `err.Error()` 拼进 `Checks` 暴露给调用方」的泄露问题——
> 失败响应改由 `renderError` sanitize，不再回传原始 cause。

---

## 5. 共识速查

| 数据 | 性质 | 进日志方式 |
|---|---|---|
| 谁 / 什么坏了 | 错误语义 | `code` / `case` / `message` / `cause`（+ `AddNote` 补语义上下文）→ 经 `ErrGroup` 进 `err` 嵌套组；`cause` 仅进日志、不进响应；`event` 例外，留顶层做主键 |
| method / url（+ request_id / latency） | 可观测维度 | `req` 嵌套组（`slog.Group`），**不进 error**；request_id/latency 在 access log |
| slog `msg` | 日志点描述 | 固定低基数短语（`"request failed"`）或留空 |
| 完整 req / resp body | 敏感 / 高基数 | 默认不记；要记必须脱敏 + 限场景（DEBUG / 开关） |
| stacktrace | apperror 现不带，决定引入 | 在 `go-apperror` 构造处 leaf-only 采集；写日志时 walk 链逐层作为独立结构化 attr，绝不进 `msg` / `message`（详见 `docs/error-stacktrace-logging-research.md`） |

**两条总纲：**

1. **错误语义走错误对象、可观测维度走 slog.Attr**——两者不互相倒灌。
2. **凡是要进 error message 的，先问会不会被 sanitized response 漏给 client**
   ——会，就别放进去。
