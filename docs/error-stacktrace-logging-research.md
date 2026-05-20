# 错误日志记录 stacktrace —— 业界方案调研与选型

> 默认的 Go 应用写 error 日志时不会带上 stacktrace。本文调研业界如何在记日志时
> 把**整条 error chain（每一层错误，含 stack）**落进日志，对比三大流派的取舍，
> 最后给出适配本项目（六边形架构 + `go-apperror` 错误模型）的落地建议。
>
> 既是结论，也保留推导过程，便于日后回看为什么这么定。
>
> 适用范围：`github.com/ikonglong/go-apperror`、`internal/common/log`（包别名
> `applog`）、`internal/adapter/rest` 边界层。配套阅读：
> `docs/logging-design.md`（logger 装配）、
> `docs/error-logging-context-discussion.md`（msg/上下文，§3 旧结论本文修订）、
> `.claude/error_handling_guide.md`（错误模型）。

## 目录

1. [缘起：默认不记 stack，我们到底想要什么](#1-缘起默认不记-stack我们到底想要什么)
2. [现状核对：库与本项目日志出口](#2-现状核对库与本项目日志出口)
3. [业界三大流派](#3-业界三大流派)
4. [两个绕不开的设计决策](#4-两个绕不开的设计决策)
5. [选型建议（针对本项目）](#5-选型建议针对本项目)
6. [与既有结论的关系](#6-与既有结论的关系)
7. [参考资料](#7-参考资料)

---

## 1. 缘起：默认不记 stack，我们到底想要什么

Go 标准库的 `errors` 不采集 stacktrace，`log/slog` 也不原生处理 stack。所以一个
「裸用 stdlib」的 Go 应用，写 error 日志时只有一行 `err.Error()` 文本，没有任何
「错误从哪冒出来」的定位信息。

诉求是：**记 error 时，把整条 error chain 中每一层错误（包括其 stack）都写进
日志**。要达成它，先得分清两件常被混为一谈的事：

- **链上消息**：每层 error 自己的描述，拼起来回答「发生了什么」。
- **stack**：错误产生点的调用栈，回答「在代码的哪个位置、经由什么调用路径产生」。

本项目通过 `apperror` 的 `cause` 链（`WithCause`）已经能表达「链」，且
`apperror.FlatMessage` 已能把链上消息拼成一行（见
`docs/error-logging-context-discussion.md`）。**真正缺的是 stack**——这正是本文
要解决的。

---

## 2. 现状核对：库与本项目日志出口

**库不带 stack。** 核对过 `github.com/ikonglong/go-apperror` 源码：`AppError`
只有 `code` / `event` / `caseVal` / `message` / `details` / `cause` 六个字段，
全库没有任何 `runtime.Callers`；`RemoteError` 同理。所以「直接取 error 的
stacktrace」无从取起——库根本没采。

**日志出口已经收口。** 本项目错误日志有单一边界：

- `internal/adapter/rest/error_resp.go` 的 `renderError`：所有 REST 失败的唯一
  出口，把 error 翻译成 JSON + HTTP status，并调 `applog.ErrorAttrs(...)` 记日志。
- `internal/common/log/err_attrs.go` 的 `ErrAttrs(err)`：按 `error_handling_guide.md`
  §4.5 从 error 抽出结构化字段（`code` / `case` / `message`，外加 `RemoteError`
  的 `service` / `status` / `body_code` 等）。

这意味着：**stack 一旦在库里采到，只需在 `ErrAttrs` 这一处统一渲染成结构化
字段，全项目即可生效。** 改动面很小。

---

## 3. 业界三大流派

### 3.1 `pkg/errors`：源头采一次（事实标准接口）

Dave Cheney 的核心论点：**90% 场景下你要的是错误产生点（root cause）的那一条
stack**，不必每层 wrap 都采。

- API：`New` / `Errorf` 创建时采栈；`Wrap` / `Wrapf` 跨包边界补消息；用 `%+v`
  打印整条链含 stack。
- 它定义了一个被整个生态（Sentry、zap、各类 APM）认可的**事实标准接口**：

  ```go
  type stackTracer interface {
      StackTrace() errors.StackTrace // []errors.Frame
  }
  ```

- 早期争议（issue #75）：每次 `Wrap` 都重复采栈很浪费。后来优化为——**若被包裹
  的 error 已带 stack，则跳过本层采集**。
- ⚠️ **`pkg/errors` 已于 2021-12-01 归档（只读），不再维护。** 新项目不应直接
  依赖它；但**应兼容它的 `StackTrace()` 接口**，因为整个生态都按这个接口消费 stack。

### 3.2 `cockroachdb/errors`：每层都采 + PII 安全 + Sentry

功能最全的重量级方案：

- `New` / `Newf` 创建采栈；`Wrap` / `Wrapf` / `WithStack` **每次 wrap 都采一条新栈，
  不去重**（可用 `WrapWithDepth` 调整记录深度）。
- `%+v` → `FormatError` 递归打印整条链 + 每层 stack + hints/details。
- `GetOneLineSource(err)` 取最内层栈顶 `file:line fn`，适合写成一个单字段。
- **PII 安全**：stack（只含文件 / 行 / 函数名，不含参数）默认判为 PII-free，可直
  接进 Sentry；消息里的动态值默认不安全，需 `Safe()` 标记。内置 `BuildSentryReport`。
- 代价：依赖重、心智负担大、每层采栈有分配开销。适合大型分布式系统、错误需跨网络
  传输并上报 Sentry 的场景。**对本项目当前需求偏重**——它最值钱的脱敏与错误跨网络
  保真现在用不上，何时才值得上见 §5.1。

### 3.3 现代轻量替代 + 原生 `slog`

`pkg/errors` 归档后涌现的维护中替代，理念基本一致：「pkg/errors 的 drop-in +
修掉重复采栈 + 原生 slog 集成」。

| 库 | 特点 |
|---|---|
| `go-faster/errors` | pkg/errors 兼容 drop-in，活跃维护 |
| `tozd/go-errors` | drop-in，带 stack + 结构化 details，对 sentinel / joined errors 支持更好 |
| `stackprune/errors` | **创建时采一条栈，wrap 时保留不重复**；`*Error` 实现 `slog.LogValuer`，直接结构化输出 kind / message / stack |
| `go-errors/errors` | pkg/errors 老牌替代，持续维护 |

其中 `slog.LogValuer` 是值得借鉴的现代手法：让 error 类型自己决定「被 slog 记录时
展开成什么结构化字段」，无需在每个日志调用点手写。

### 3.4 标准库与官方态度

- **stdlib `errors` 至今不采 stack，`slog` 也不原生处理 stack。**
- 提案 [golang/go#60873](https://github.com/golang/go/issues/60873)（在 `fmt.Errorf`
  里用 `@trace` 标记按需采点）2023-06 提出，**至今仍是 open，未被接受**。

> 结论：短中期别指望标准库内置，要么自己采，要么用三方库。

---

## 4. 两个绕不开的设计决策

### 4.1 在哪里采 stack

| 策略 | 含义 | 取舍 |
|---|---|---|
| **leaf-only**（推荐） | 只在错误产生点采一次 | 最便宜；那条栈恰好指向根因 |
| every-wrap | 每层 wrap 都采 | 能还原「经过哪些边界」，但有重复和开销，必须配「下游已有则跳过」 |
| 折中（主流） | 创建时采，wrap 时若已有则跳过 | pkg/errors 后期、各轻量库的默认行为 |

对本项目尤其关键：`AppError` 几乎总是在 domain / application / driven-adapter
**分类失败的那一刻**被构造，这本身就是「源头」；而 `AddNote` 是原地改 message、
**不新增链层**。所以**采集**用 leaf-only 足矣，不需要 every-wrap 那种每层重复采。

但别把这条推过头：本项目将以此模板承载复杂业务，链上一定会出现**外部库 error**
和 `fmt.Errorf("%w")` 嵌套，它们各自可能自带 stack。所以正确组合是——**采集
leaf-only，但写日志时 walk 整条链、把每一层带 stack 的错误都输出**（`AppError`
自己的，加上任何实现了 `StackTrace()` 接口的外部 error）。详见 §4.2、§5。

### 4.2 怎么把整条 chain 写进日志（含禁止 `\n` 约束）

三种形态：

1. **一行 flat message**（即现有 `FlatMessage`）：可读，但无 stack、无逐层定位。
2. **`%+v` 整块 dump**：信息全，但**是多行字符串**。
3. **逐层结构化**：walk `errors.Unwrap`，每层输出 `{event, code, message, frames:[…]}`。

⚠️ **硬约束**：`error_handling_guide.md` §6 反模式 #3 明令「不要在 error message
里用 `\n`」——日志聚合器按换行切记录，一条错误会被拆成多条。因此：

> **stack 绝不能塞进 `msg` 或 `message` 字段，也不要用 `%+v` 多行 dump 当字符串
> 字段。** 正确做法：stack 作为**独立结构化字段**——一个**字符串数组**（每帧
> `"file:line fn"` 一个元素），JSON 日志里天然无内嵌换行，聚合器友好。这与本项目
> 既有规范完全自洽。

---

## 5. 选型建议（针对本项目）

> 前提更正：本项目**不会一直停留在简单逻辑的模板**——基本完成后将以它为原型创建
> 承载复杂业务的生产应用（约 2026-05-27 投产）。所以下面每个取舍都按**它即将成为的
> 那个复杂生产应用**的标准来判断，而不是按当前这层薄模板从简。当前的简单是暂时的，
> 「现在逻辑简单 / 赶时间」都不是合法的选型理由——要的是长期正确解。

**结论：仍然给 `go-apperror` 加 leaf-only stack 捕获 + `StackTrace()` 兼容接口，
日志侧 walk chain 输出结构化 stack 数组字段。** 但理由不再是「模板项目从简」，而是：

1. **`go-apperror` 是自有库**（`github.com/ikonglong/...`）。给自己的库加 stack 是
   在自己路线图上加 ~50 行，不存在「fork 第三方并长期维护 fork」的负担——与「引入
   重型第三方依赖」完全不是一个风险量级。
2. **已在自有错误模型上重度投资**：`Code/Case/Event` taxonomy、`RemoteError`、
   HTTP 映射、整本 `error_handling_guide.md`，是一套团队认知已对齐的自洽模型。
   `cockroachdb/errors` 自带另一套哲学（无 Code/Case/Event、无 RemoteError 概念）。
   引入它只有两条路，**长期看都不对**：整体替换你的模型（推翻一套自洽且已对齐的
   设计），或把它塞到 apperror 底下只用 5%（拉重依赖 + 两套错误哲学长期阻抗失配）。
   给自有库补一个本就缺的 stack 能力，才是顺着既有设计的**长期正确解**。
3. **stack 捕获本身不 gnarly**：`runtime.Callers` + `CallersFrames` + 暴露
   `StackTrace()` 是标准操作。真正需要「身经百战的库」的是**脱敏（PII redaction）**
   与**错误跨网络编解码**——这两个本项目当前都不需要。
4. **「上生产 + 复杂业务」反而让方案里两点更值钱**：暴露 `StackTrace()` 事实标准
   接口让将来接 Sentry/APM 近乎零成本；stack 作为结构化数组字段、不带 `\n`，规模化
   后做检索/聚合的价值只增不减。

具体落点：

1. **单点采集**：18 个工厂函数全部汇聚到 `newAppError`，在这里 `runtime.Callers`
   采一次即可，`skip` 是常量（`Callers` + `newAppError` + `NewXxx` = 3）。一处改动
   覆盖全部工厂。

   ```go
   type AppError struct {
       // ...existing fields...
       stack []uintptr // 创建点的 PC，延迟解析
   }

   func newAppError(code Code, event string, opts ...Option) *AppError {
       // ...existing...
       e.stack = callers(3) // runtime.Callers(3, pcs[:])
       return e
   }
   ```

2. **延迟解析 + 深度封顶**：只存 `[]uintptr`，等真要写日志时才用
   `runtime.CallersFrames` 解析成 `file:line fn`——多数 error 根本不被打印，别提前
   付字符串化成本。`pcs` 用定长缓冲（如 32 / 64 帧）封顶，避免复杂业务里的病态深栈
   拖累热路径。

3. **兼容生态**：暴露 `func (e *AppError) StackTrace()`，对齐 `pkg/errors` 事实
   标准接口，将来接 Sentry / zap 零成本。

4. **`RemoteError` 注意**：它是结构体字面量直接构造（不走工厂），要么给它加构造
   函数采栈，要么在 driven adapter 翻译处采——否则它不会有 stack。

5. **统一渲染、逐层输出**：walk 整条 chain，把**每一层带 stack 的 AppError** 作为
   独立结构化 attr 输出（数组,不带 `\n`），连 `fmt.Errorf("%w")` 嵌套的栈也不漏。
   *（实现落点）* 落在 `internal/common/log/err_stack.go` 的 `StackAttrs(err)`，由
   `ErrGroup` 折入 `err` 组（渲染为 `err.stack`）；`renderError` 调的是 `ErrGroup`，
   故 `error_resp.go` 无需改动。仅对非预期 code 渲染（见落点 6）。

6. **可选开关——是否每个 error 都采**：`NotFound` / `AlreadyExists` 这类被当控制
   流用的「预期错误」，采栈是噪音。两种常见做法：
   - (a) **一律采，但只对非预期 code 渲染进日志**（`InternalError` / `UnknownError`
     / `IllegalState`，即走 `rest.unhandled` → 500 那条路）；
   - (b) 按 code 条件采集。

   建议先用 (a)：实现简单，且不丢信息。

### 5.1 什么时候反而值得上 `cockroachdb/errors`

上面的结论有一个明确的反转判据，写在这里以免日后误读为「永远别用」：

> **当这个复杂应用演化为「你自己的多个 Go 服务之间互传带类型的错误」**——把 error
> 连同类型 + stack 编码过网络、对端解码后还能还原原始类型——`cockroachdb/errors`
> 的「错误可移植性」才真正物有所值，值得上。

注意它和现有 `RemoteError` 不是一回事：`RemoteError` 是**消费**别人的远程错误；
`cockroachdb/errors` 解决的是**传输你自己的错误且保真**。若系统是单体，或服务间走
标准 HTTP/JSON 错误体（本项目当前形态），则自有库 + `StackTrace()` 接口已经够用。

### 5.2 折中：不想手写就内嵌一个轻量库

若不愿长期自己维护栈帧采集 / 格式化代码，可让 `AppError` **内嵌一个维护中的轻量库**
（`go-faster/errors` 或 `tozd/go-errors`）作为 stack 载体——拿到经过实战的栈采集
与 `slog.LogValuer`，又不像 `cockroachdb/errors` 那么重。代价是多一个依赖、且要协调
两层的 `Error()` 格式。本文略偏向手写（模型更单一、依赖更少）；把「栈帧维护外包给
轻量库」当作可接受的长期取舍即可，而非赶工捷径。

---

## 6. 与既有结论的关系

`docs/error-logging-context-discussion.md` §3「关于 stacktrace」此前的结论是
**「本项目暂不引入 stacktrace」**——理由是对当时的 AppError 体系属过度设计，且
本设计靠 `event` + `code` + `cause` 定位、不靠 stack。

本文是对那个结论的**重新审视**：当目标明确变为「记 error 时输出整条 chain 含
stack」后，方向从「不引入」转为「引入，但用最轻的方式」。两者并不矛盾——§3 的两条
核心约束在本文依然成立并被吸收:

- slog 的 `AddSource` 不顶用（指向日志调用点，不是错误产生点）；要的是**错误
  产生点**的栈，所以必须在 `apperror` 构造处采，而非靠 slog。
- stack 只作为**独立结构化 attr**，绝不进 `msg` / `message`（禁止 `\n`）。

> 已回填：`docs/error-logging-context-discussion.md` §3 末尾的旧结论「暂不引入
> stacktrace」已改为「决定引入，但用最轻的方式」并指向本文，两处不再打架。

---

## 7. 参考资料

- [Dave Cheney —— Stack traces and the errors package](https://dave.cheney.net/2016/06/12/stack-traces-and-the-errors-package)
- [pkg/errors issue #75 —— Why call callers() on every wrap?](https://github.com/pkg/errors/issues/75)
- [cockroachdb/errors（pkg.go.dev）](https://pkg.go.dev/github.com/cockroachdb/errors) · [GitHub](https://github.com/cockroachdb/errors)
- [tozd/go-errors](https://github.com/tozd/go-errors) · [stackprune/errors](https://pkg.go.dev/github.com/stackprune/errors)
- [DoltHub —— Getting stack traces for errors in Go](https://www.dolthub.com/blog/2023-11-10-stack-traces-in-go/)
- [golang/go#60873 —— proposal: errors: add (stack)trace at error annotation](https://github.com/golang/go/issues/60873)
- [oneuptime —— How to create custom error types with stack traces in Go](https://oneuptime.com/blog/post/2026-01-30-how-to-create-custom-error-types-with-stack-traces-in-go/view)
