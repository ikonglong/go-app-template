# Go 与 Java 日志使用模式 —— 讨论记录与设计决策

> 从对比 Java/Log4j 出发，厘清 Go/slog 惯用法，为本项目（六边形架构加 dig 装配）
> 选定方案并落地。既是结论，也保留推导过程。
>
> 适用范围：`internal/common/log`（包别名 `applog`）及其调用方。

## 目录

1. [缘起：两种生态的取向差异](#1-缘起两种生态的取向差异)
2. [问题一：logger 放 ctx 是 Go 的最佳实践吗？](#2-问题一logger-放-ctx-是-go-的最佳实践吗)
3. [问题二：Go 支持按类型/包/文件获取 logger 吗？](#3-问题二go-支持按类型包文件获取-logger-吗)
4. [方案 A 与方案 B](#4-方案-a-与方案-b)
5. [实现：A、B 并存](#5-实现ab-并存)
6. [很多函数/domain 方法要打日志，B 还合适吗？](#6-很多函数domain-方法要打日志b-还合适吗)
7. [component 标签的三种思路](#7-component-标签的三种思路)
8. [结论与落地清单](#8-结论与落地清单)

---

## 1. 缘起：两种生态的取向差异

**Java/Log4j**：每个类 `getLogger(Class)` 拿自己的 logger，日志自带类名、方法名、
行号；请求级信息（`request_id`）放线程本地（logback **MDC**）；格式、目的地、
rolling、按包路由全部 XML 可配。

**Go/slog（本项目现状）**：把全局 logger 放进请求 ctx，handler、service 从 ctx 取用；
格式、目的地、rolling 尚未配置。

| | Java/Log4j | Go/slog |
|---|---|---|
| 模型 | 重配置、按 logger-name 驱动一切、运行时可调 | 轻内核、配置即代码、用 Handler 拼 |
| 中心 | 中心化（LogManager 注册表） | 去中心化（显式 Handler 组合） |

Java 的 per-class logger 加 XML 路由在 Go 里没有直接对应物——需用其他机制拼近似效果，
即下文主题。

---

## 2. 问题一：logger 放 ctx 是 Go 的最佳实践吗？

**结论：不是官方推荐，是有争议的社区模式。标准库 `log/slog` 刻意没有提供
logger-in-ctx 的 API。**

这里要区分两种「用 ctx」（对应下文[方案 A 与 B](#4-方案-a-与方案-b)）：

- **A**：把 **logger 本身**存进 ctx，下游取回来用。
- **B**：logger 显式持有，只把 **ctx 喂给日志调用**（`logger.InfoContext(ctx, ...)`），
  让 Handler 从 ctx 捞 `request_id`。

标准库给了 `slog.InfoContext` 这组带 ctx 的方法，意图正是「传 ctx 给 Handler 读」，
而非取 logger——**官方设计的是 B，不是 A**。

官方不做 A 的原因：

1. `context` godoc 明言 ctx 只放 request-scoped data，不传依赖；logger 更像依赖。
2. slog 提案早期有 `slog.FromContext`，发布前（Go 1.21）被砍——不想钦定该模式。
3. `f(ctx)` 的签名看不出它依赖 logger（隐式依赖）。

但 A 也不算错：本项目存进 ctx 的是「已绑 `request_id` 的**请求级** logger」，本身就是
request-scoped data，属于 A 里最站得住脚的那种。

---

## 3. 问题二：Go 支持按类型/包/文件获取 logger 吗？

**结论：不支持。Go 没有「按 class/package 命名加层级 level 继承加按名字路由」的
named-logger registry。** Java 的 `getLogger(Class)` 里那个名字身兼三职（配置 key、
输出字段、路由依据），Go 没有这个「名字到行为」的注册表。

| Java/Log4j | Go/slog 对应 | 内建？ |
|---|---|---|
| 类名、方法名、行号 | `HandlerOptions{AddSource: true}` → `source={file,line,function}` | ✅ |
| `getLogger(Class)` 每类命名 logger | 手动 `logger.With("component", ...)`，自己持有派生 logger | ⚠️ 手动，是属性非 registry key |
| 不同包不同 level | 自己写 Handler 按 `source`/`component` 前缀过滤 | ❌ |
| 不同包不同目的地 | `samber/slog-multi` 等 fanout/routing，或自己组合 Handler | ❌ |
| MDC | ctx 值加 context-aware Handler（[方案 B](#4-方案-a-与方案-b)） | ⚠️ 自己搭 |

**为什么没有 `getLogger(Class)`**：Go 运行时没有 class 元数据；`runtime.Caller` 只能
拿源位置（`AddSource` 即此），不是可做层级配置的 named hierarchy。

**配置与 rolling**：stdlib 不管。格式、目的地在代码里 wire（本项目的 `applog.Init`）；
rolling 用 `lumberjack` 当 `io.Writer`，或更常见地——程序只往 stdout 打 JSON，切割、
收集交给外部（k8s 加 Loki/ELK）。

---

## 4. 方案 A 与方案 B

把「用 ctx 携带请求级属性」具体化成两个可互换方案。

**方案 A —— logger in context**：中间件派生一个绑了 `request_id` 的 logger，把 logger
**本身**存进 ctx；下游用 `FromCtx` 取回。

- 优点：零接线，任何拿到 ctx 的代码都能免费带请求属性打日志。
- 缺点：把依赖存进 ctx（违背 ctx 用途）；日志依赖在签名里不可见。

**方案 B —— values in context + enriching handler**：中间件把请求级**值**存进 ctx；
logger 保持显式（`slog.Default()` 或注入）。`ctxHandler`（`Init` 装为默认 handler）
在每条记录落地时从 ctx 捞值加成 attr。

- 优点：请求级数据待在 ctx（本职）；logger 是显式依赖；**任何**用了 `ctxHandler` 的
  logger 都被富化，连裸 `slog.InfoContext(ctx, ...)` 都带 `request_id`；无每请求 logger
  分配。
- 缺点：多一个 handler 类型；注入字段集合固定在 `Handle` 里。

### 选型：倾向 B

1. **本项目刚上了 dig**——logger 即一个 provider，A「省传 logger 样板」的卖点消失。
2. **关键洞察**：`ctxHandler` 装成 default 后，B **不强制到处注入 logger**——`request_id`
   作为值进 ctx 自动注入，任何 `applog.InfoAttrs(ctx, ...)` 都白嫖；想打 `component`
   标签时才注入（可选）。
3. **改动极小**：wrapper 已是 `InfoAttrs(ctx, ...)`，只动 `applog` 内部，调用点不改
   （机制见 [§5](#5-实现ab-并存)）。

**A 何时更合适**：无 DI 的小程序/CLI；或大量叶子函数都要打日志、穿 logger 真的烦的
代码库。都不是本项目（日志集中在边界、domain 不打日志、注入面小）。

---

## 5. 实现：A、B 并存

**决定：A 保持活跃（运行路径不变），B 完整实现加安装加测试**，两者独立文件加头注分开。

| 文件 | 角色 | 状态 |
|---|---|---|
| `ctx_logger.go` | 方案 A：`IntoCtx`/`FromCtx` | 运行中 |
| `ctx_handler.go` | 方案 B：`WithRequestID`/`RequestIDFromCtx`/`ctxHandler` | 已实现加安装加测试 |
| `log.go` | 共享：`Init`、4 个 wrapper、`logAttrs`、`parseLevel` | 共用 |
| `err_attrs.go` | `ErrAttrs`：把 `AppError`/`RemoteError` 拆成结构化字段 | —— |
| `ctx_handler_test.go` | 证明 B：注入生效、无值 no-op、`.With` 不丢装饰器 | 通过 |

**关键设计：调用点零改动，单一切换点。** `logAttrs` 走 `FromCtx(ctx)`，而 `FromCtx`
没绑 logger 时回退 `slog.Default()`，于是同一套 wrapper 同时服务两者：

- A 活跃：`FromCtx` 返回 ctx 里绑了 `request_id` 的 logger。
- B 活跃：ctx 没绑 logger → 返回 `Default()`（被 `ctxHandler` 包过）→ handler 从 ctx
  的 `request_id` 值注入。

```go
func logAttrs(ctx context.Context, level slog.Level, event, msg string, attrs ...slog.Attr) {
    all := append([]slog.Attr{slog.String("event", event)}, attrs...) // event 前置
    FromCtx(ctx).LogAttrs(ctx, level, msg, all...)                    // 同一行服务 A、B
}
```

**唯一切换点**在 `request_logger.go`：

```go
ctx = applog.IntoCtx(ctx, slog.Default().With(slog.String("request_id", reqID))) // A（当前）
// ctx = applog.WithRequestID(ctx, reqID)                                        // B（换这行）
```

两点保证并存安全：

- `Init` 无条件包 `ctxHandler`，但 A 活跃时 ctx 无 `request_id` **值**，handler 不加 →
  `request_id` 只来自 A 的 `.With`，**不重复**。
- `ctxHandler` override 了 `WithAttrs`/`WithGroup` 重新包装自己，否则 `logger.With(...)`
  会返回裸 base handler 丢掉注入能力（`TestCtxHandlerSurvivesWith` 盯此）。

---

## 6. 很多函数/domain 方法要打日志，B 还合适吗？

直觉：command 由容器构造、传 logger 方便；但自由函数不需构造、domain 对象运行期动态
创建，传 logger 不便。**纠正前提后，B 反而更合适。**

**误解：B 不等于到处传 logger。** B 有两个独立部分：

1. **请求级数据（`request_id`）靠 ctx 加 handler**——不需要任何 logger 对象，有 ctx 就
   `applog.InfoAttrs(ctx, ...)`。
2. **注入带 `component` 标签的 logger**——才需要传 logger，是可选增强，且只在 DI 顶层
   组件做（详见 [§7](#7-component-标签的三种思路)）。

自由函数、domain 只碰第 1 部分：不构造、不注入。两类数据的区别：

| | 性质 | 进日志方式 | 自由函数白嫖？ |
|---|---|---|---|
| `request_id` | 请求级（一次请求内不变，跨组件流动） | ctx 值加 handler | ✅ |
| `component` | 结构级（代码出处，与请求无关） | 必须附在某个 logger 上 | ❌ |

**A/B 在此正交**：都靠 ctx 透传，都不往自由函数注入 logger；真正约束是「有没有 ctx」。
**但 B 更优**：A 的 `request_id` 藏在私有 key 下，只有调 `FromCtx` 的代码拿得到；B 里
它是 ctx 值，连裸 `slog.InfoContext(ctx, ...)`、第三方库代码都自动带。代码越杂越值钱。

**domain 对象：先问该不该打日志。** domain 方法按六边形规矩不接 ctx，A/B 都给不了
`request_id`——这是设计信号。首选：domain 不打日志，返回富 error（`AppError` 带
event/code/case）或 domain event 上抛，由 application 边界（有 ctx）打。

**决策表：**

| 代码位置 | 有 ctx？ | 怎么打 | 注入 logger？ |
|---|---|---|---|
| handler/command（DI 构造） | 有 | `applog.XxxAttrs(ctx, ...)` | 否（除非要 `component`） |
| 自由函数 | 有 | `applog.XxxAttrs(ctx, ...)` | 否 |
| 自由函数/启动代码 | 无 | `applog.XxxAttrs(context.Background(), ...)` | 否 |
| domain 方法 | 通常无 | 别打，返回 error/event 上抛 | 否 |

---

## 7. component 标签的三种思路

承上：`component`（结构级）是唯一需要「载体」的字段。对比三种拿法。

**思路 1 —— 注入带标签的 logger**：DI 组件构造时注入 `.With("component", ...)` 的
logger 存字段。语义是词法（绑死 struct）；自由函数 ✗ 拿不到。

**思路 2 —— `AddSource`**：开 `HandlerOptions{AddSource: true}`，每条自动带
`source={function,file,line}`。这是 Go 对 Java `%logger`（类名）的等价物。语义是词法、
物理出处；自由函数 ✅ 零传递、永远正确。另外，wrapper 的 **`event` 参数本身就是逻辑
分组键**——`applog.InfoAttrs(ctx, "billing.charge", ...)` 已给出比 component 更贴操作
语义的维度，也零传递。

**思路 3 —— component-in-ctx**：组件入口派生带 `component` 标签的子 ctx，handler 检查
ctx 有就加；往下调用（含自由函数）自动继承，进更深组件时被覆盖。本质是 **baggage**
（分布式追踪里的上下文传播），正经模式。语义是动态、调用流；自由函数 ✅ 自动继承。

- **独有价值**：给「有自己 `event` 的子操作」补父流程上下文——深层 helper 自己的
  `event="db.query"`，而 `component=rest.account` 告诉你这次查询发生在哪个流程里，这是
  `event` 和 `AddSource` 都给不了的。
- **代价**：靠纪律（每入口要 stamp，漏了静默继承上游）；共享 helper 被调用方染色；
  同名 key 需去重。

| 思路 | 回答的问题 | 语义 | 传 logger？ | 自由函数 | 正确性 |
|---|---|---|---|---|---|
| 1 注入 logger | 这个 struct 的日志 | 词法 | 是 | ✗ | 构造即对 |
| 2 AddSource | 这行代码在哪 | 词法/物理 | 否 | ✅ | 自动，永远对 |
| 3 component-in-ctx | 在哪个流程执行期间 | 动态/调用流 | 否 | ✅ | 靠纪律 |

三者答不同问题，可共存：`AddSource` 兜底物理正确性，component-in-ctx 给动态流程视角。

**推荐：做成通用「ctx 属性轨道」，而非只为 component。** `request_id` 和 component 同
机制，不必加第二个 ctx key：

```go
type ctxAttrsKey struct{}

// WithLogAttrs 派生带额外日志属性的 ctx，handler 注入每条记录。
// 同名 key 覆盖（内层胜），component 重复 stamp 不产生重复键。
func WithLogAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
    return context.WithValue(ctx, ctxAttrsKey{}, mergeByKey(logAttrsFromCtx(ctx), attrs))
}
// ctxHandler.Handle 里 drain：if a := logAttrsFromCtx(ctx); len(a) > 0 { r.AddAttrs(a...) }
```

```go
ctx = applog.WithLogAttrs(ctx, slog.String("request_id", reqID))         // 中间件
ctx = applog.WithLogAttrs(ctx, slog.String("component", "rest.account")) // 组件入口
```

这条轨道吃掉了 `WithRequestID`，且天然是**以后接 OpenTelemetry baggage 的入口**。

---

## 8. 结论与落地清单

**已落地（代码中）：**

- A、B 并存，独立文件加头注分开；A 活跃，B 已实现加安装加测试。
- 调用点统一为 `applog.{Debug,Info,Warn,Error}Attrs(ctx, event, msg, attrs...)`
  （`event` 必填、`msg` 可空）；经 `FromCtx` 回退，A/B 共用、调用点零改动。
- 切换 A→B 只改 `request_logger.go` 一行；`ctxHandler` 已装 default 且对 A 无害。

**推荐/待定（需拍板）：**

- [ ] 打开 `AddSource: true`，让 domain、自由函数即使不打 component 也有物理出处。
- [ ] 把 B 设为活跃路径。
- [ ] 把 component-in-ctx 做成通用 `WithLogAttrs` 轨道，`WithRequestID` 并入，并示范
      stamp `component`。
- [ ] （远期）`WithLogAttrs` 轨道过渡到 OpenTelemetry baggage。

**四类字段各归其位**（贯穿全文的核心）：`request_id` 走 ctx 值加 handler；`source` 靠
`AddSource` 自动；`event` 是 wrapper 必填的主聚合键；`component` 可选（DI 顶层用注入、
跨自由函数用 baggage）。domain 基本不打日志，靠返回 error/event 上抛。

---

## 9. 格式与输出目的地的配置

日志的「长什么样」和「写到哪」是两个正交维度，都由 env 配置（`assembly.Config`
读取，命名沿用 `SECTION__KEY` 双下划线约定），互不影响。

### 9.1 格式（format）—— `LOG__FORMAT`

| 值 | handler | 形态 | 用途 |
|---|---|---|---|
| `text` | `slog.TextHandler` | logfmt，嵌套 group 扁平成点号键：`event=account.create err.code=… req.method=POST duration_ms=1000` | 本地开发，人读 |
| `json` | `slog.JSONHandler` | 嵌套对象：`{"event":…,"err":{…},"req":{…}}` | 部署测试 / 生产，机器收集 |

两种格式输出**同一套结构化字段**，差别只在序列化：text 把 `err` / `req` 组扁平成
`err.code` / `req.method`，json 保持嵌套对象。`event` 在两者里都留顶层主键。

### 9.2 目的地（destination）—— `LOG__OUTPUT`

逗号分隔的集合，取值 `console` / `file`，可同时开：

- `console` → `os.Stdout`（容器运行时默认采集的就是它）。
- `file` → 滚动文件，由 `lumberjack` 在进程内切割。
- `console,file` → 经 `io.MultiWriter` 同时落两处，逐字节一致。
- 空 / 全是未知值 → 退化到 `os.Stdout`（绝不让进程「静默无日志」）。

文件目的地的滚动参数（仅 `LOG__OUTPUT` 含 `file` 时生效）：

| env | 默认 | 含义 |
|---|---|---|
| `LOG__FILE_PATH` | `logs/app.log` | 路径，父目录在启动时自动创建（不可写则启动即失败，fail-fast） |
| `LOG__FILE_MAX_SIZE_MB` | `100` | 单文件达到此大小后切割 |
| `LOG__FILE_MAX_BACKUPS` | `7` | 保留的旧文件数（0 = 全留） |
| `LOG__FILE_MAX_AGE_DAYS` | `30` | 旧文件最长保留天数（0 = 不按时间删） |
| `LOG__FILE_COMPRESS` | `false` | 是否 gzip 旧文件 |

### 9.3 落地位置

- `internal/common/log/output.go` —— `NewWriter(Output) (io.Writer, func() error, error)`：
  把目的地配置构造成 writer（含 lumberjack + MultiWriter）和一个 Close（关文件 sink，
  幂等）。这是「编写日志输出」的组件，**不依赖 assembly**，只收中性参数。
- `assembly.Config` + `InitLogger` —— 把 env 映射成 `applog.Output`，调 `NewWriter`
  拿到 writer，喂给 `applog.Init`（slog）和 hlog 桥接，让两者共用同一组 writer /
  format / level；返回 Close。
- `cmd/server/main.go` —— 改成 `main → os.Exit(run())`，`run` 里 `defer closeLog()`，
  使文件 sink 在每条退出路径都被关闭（`os.Exit` 会跳过 defer，故下沉到 `run`）。

`applog.Init` 的签名不变（仍接受 `io.Writer`）：目的地的构造被隔离在 `NewWriter`，
`Init` 只管「给定 writer 装配 handler」，职责清晰。

> 典型组合：本地 `LOG__FORMAT=text` + `LOG__OUTPUT=console`；k8s `json` + `console`
> （切割收集交给集群）；传统部署 `json` + `console,file`（终端看 + 文件留存滚动）。
