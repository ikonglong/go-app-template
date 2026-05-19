# AppError:抛 panic vs 返回 error 的设计讨论

本文档记录"是否把 AppError 直接 panic 出去、在边界 recover 兜底"这一设
计选项的讨论。结论先行:**最终决定 errors-as-value 作为主路径,panic
仅保留给安全网和断言场景**;详细论证见下。

本文档不是规范(规范见 `error_handling_guide.md`),而是这条规范背后的
设计 rationale。新人对"为什么不直接 panic"有疑问时读这里。

---

## 0. 起源:为什么会考虑用 panic

讨论由这个观察发起:

> 当你调用一个方法,遇到错误时,可以无脑往上抛,到达接口层后,会被
> 统一捕获,写日志,生成 error response。因为错误无非分两类:
>
> 1. 编程时无意识引入的错误
> 2. 无论如何努力,总归会发生的(网络失败、磁盘出问题等)
>
> 第一类,你用返回值接收或者捕获了,也做不了什么,最终也还是给错误响
> 应,写日志。第二类,对于网络失败、磁盘出问题,一般在发生的地方会退避
> 重试 2 次,尽力后如果仍失败,最终也是需要给错误响应,写日志,除此之
> 外,做不了什么。
>
> 因此当产生一个 AppError 对象时,我倾向于直接把它 panic(appError)
> 出去,在接口层通过 recover 捕获它,写日志,生成错误响应。

核心动机有二:
- **减少 `if err != nil { return err }` 的视觉噪声**
- **集中化错误处理**:所有错误最终都在边界做同一件事(log + response),
  那中间层的传递就是冗余的

这两点是真实的痛点。Go 的错误处理确实啰嗦。问题在于:**全局改用 panic
能否在不引入更大代价的前提下解决这两个痛点?** 下面四个场景逐一检验。

---

## 1. 场景一:Finder 用哨兵错误表示"未找到"

### 原始 setup

最初的 `IAccountRepo`:

```go
FindByEmail(ctx context.Context, email string) (*Account, error)
// 找不到 → 返回 (nil, ErrAccountNotFound)
// DB 挂  → 返回 (nil, 其它 err)
```

`SignUpService.assertCredentialAvailable` 必须三路分支:

```go
func (s *SignUpService) assertUnused(_ *domain.Account, err error) error {
    switch {
    case errors.Is(err, domain.ErrAccountNotFound):
        return nil                              // 找不到 = credential 可用
    case err != nil:
        return err                              // 真错(DB 挂)
    default:
        return domain.ErrAccountCredentialTaken // 找到了 = credential 已被占
    }
}
```

我把这种丑写法当成"errors-as-value 模型必须的复杂度",列为反对 panic
方案的第一个理由。

### 讨论

用户反驳:

> 既然是 find,找不到应该返回 nil 或 Optional,而不是通过一个
> `domain.ErrAccountNotFound` 错误告诉调用者找不到

被说服。这不是 panic vs error 之争,是 **API 设计本身错了**:把"找不到"
(正常 lookup outcome)和"DB 挂了"(operational failure)混进了一个
`err != nil` 通道。

### 重构后

```go
FindByEmail(ctx context.Context, email string) (*Account, error)
// (nil, nil)         → 找不到(正常)
// (non-nil, nil)     → 找到
// (anything, err)    → 真错(operational)
```

调用方:

```go
func (s *SignUpService) assertUnused(acct *domain.Account, err error) error {
    if err != nil {
        return err                               // 真错
    }
    if acct != nil {
        return domain.ErrAccountCredentialTaken  // 占用
    }
    return nil                                   // 可用
}
```

### 结论

**该场景不成立**(API 错而非模型错)。

相关变更:
- `internal/domain/repo.go` 改 IRepo 契约,`FindXxx` 返回 `(nil, nil)`
  表示未找到
- `internal/domain/account.go` `ErrAccountNotFound` 改用途:不再由 repo
  返回,改为 application/handler 层手动构造 404 时的预制 sentinel
- 同时把 `Update`/`Delete` 改成 `(int64, error)`,把"0 行是不是错"的
  解释权还给 caller(参见 `internal/adapter/out/db/base_repo.go`)

---

## 2. 场景二:同步可选调用降级

### 原始 setup

请求处理中调用一个"失败可以降级"的能力。例如:

- 推荐补充:rec 引擎挂了 → 返回不带推荐的基础响应
- 个性化富文本:第三方渲染失败 → 返回纯文本
- 可选鉴权:entitlement 服务不可用 → 默认放行 + audit log

### 第一轮反驳:用 goroutine 隔离

用户的第一个回应:这类"可选"调用通常本来就是 goroutine 异步做的(埋点、
缓存预热),所以 panic 在子 goroutine 边界 recover 即可,不影响主流程。

我承认埋点 / 主动缓存预热确实是 goroutine;但**有一类"请求内、同步、必
须等结果、失败可降级"的调用根本不能扔 goroutine**,因为响应体本身依赖
结果。上面三个例子(推荐补充、富文本渲染、可选鉴权)都是。

### 第二轮反驳:把 goroutine 藏进函数内部

用户进一步反驳:

> recService.Enrich 内部必然要在新 goroutine 中调用推荐服务,因为它
> 很重,可能涉及 I/O。那就在这个 goroutine 内通过 recover 捕获,然
> 后 recService.Enrich 拿到 goroutine 结果,如果为 error,直接 panic
> 就好了。调用方无需再启动 goroutine 调用 recService.Enrich 了

### 我的反驳

**前提就是错的**:Go 里 I/O 调用不需要内部 spawn goroutine。

Node.js / Python 单线程事件循环模型下,异步 I/O 必须显式 spawn。**Go 不
是这样**。Go runtime 在 goroutine 阻塞 I/O 系统调用时,自动 park 该
goroutine、把 OS 线程让给其他 goroutine。"重 I/O"和"是否要新开 goroutine"
完全是两件事。

一个**正常的 Go I/O 函数**就是同步写:

```go
func (r *RecService) Enrich(ctx context.Context, req Request) (Recommendations, error) {
    resp, err := r.httpClient.Do(httpReq)  // 阻塞,但 runtime 自动调度
    if err != nil { return nil, err }
    return parseRecs(resp), nil
}
```

caller 单 goroutine,被 I/O 阻塞期间 runtime 在背后帮你跑别的 goroutine。
**额外开 goroutine 是 Go 反模式**(浪费调度、复杂化 cancel/timeout、
stack trace 跨边界)。

即使硬要按用户的模型实现(让 Enrich 内部 spawn + recover + 拿结果再
re-panic):

```go
func (r *RecService) Enrich(ctx context.Context, req Request) Recommendations {
    type result struct {
        v   Recommendations
        err error
    }
    ch := make(chan result, 1)
    go func() {
        defer func() {
            if r := recover(); r != nil {
                err, _ := r.(error)
                ch <- result{err: err}
            }
        }()
        resp := r.httpClient.MustDo(httpReq)  // panic 模型
        ch <- result{v: parseRecs(resp)}
    }()
    res := <-ch
    if res.err != nil {
        panic(res.err)                        // re-throw 给 caller goroutine
    }
    return res.v
}
```

整条链路:caller → spawn G2 → G2 panic → recover 转 channel →
caller 读 channel → caller panic → caller 上级 recover 接住。

**绕一圈 channel + 两次 panic + 两次 recover,等价于一行 `return err`
+ caller 一行 `if err != nil`**。Go runtime + error-as-value 两行能搞
定的事,变 20 行 + goroutine 协调开销。

### caller 端复杂度并没有消失

**关键点**:即使 `Enrich` 内部隐藏 goroutine,caller 想要"失败降级"
语义仍然必须包闭包 + sub-recover:

```go
// panic 模型,caller 端写法
func() {
    defer func() {
        if r := recover(); r != nil {
            log.Warnf("rec optional: %v", r)
        }
    }()
    base.Recommendations = recService.Enrich(ctx, req)
}()
return base
```

vs error 模型:

```go
recs, err := recService.Enrich(ctx, req)
if err != nil {
    log.Warnf("rec optional: %v", err)
} else {
    base.Recommendations = recs
}
return base
```

5 行 vs 8 行 + 多一层闭包。**问题不在哪儿 spawn goroutine,在于 panic
在 Go 里没法 inline 局部捕获**(不像 Python `try/except` 一行框住)。
只要中间任何一层需要"分支处理这个错",就要套一层闭包 + defer recover。

### 结论

**该场景 errors-as-value 胜**。用户让步。

---

## 3. 场景三:并行 fan-out 部分失败聚合

### 业务约束

典型 BFF 接口 `GET /users/{id}/profile`,聚合 3 个下游:

| 下游 | 失败策略 |
|---|---|
| `account`(user-service) | **必需**,挂了整个请求 5xx |
| `recent_activity`(activity-service) | **可选**,挂了该字段 null + warning,请求继续 |
| `recommendations`(rec-service) | **可选**,同上 |

三者并行(避免任一拖累 P99),聚合一个响应。

### error-as-value 版

```go
type Profile struct {
    Account         *Account         `json:"account"`
    RecentActivity  []Activity       `json:"recent_activity,omitempty"`
    Recommendations []Recommendation `json:"recommendations,omitempty"`
    Warnings        []string         `json:"warnings,omitempty"`
}

func (s *ProfileService) GetProfile(ctx context.Context, id string) (*Profile, error) {
    var (
        p        Profile
        warnings []string
        mu       sync.Mutex
    )

    g, gctx := errgroup.WithContext(ctx)

    // 必需:account。失败 → return 非 nil err → errgroup 取消 gctx
    // → 另外两个子任务收尾
    g.Go(func() error {
        acct, err := s.accountRepo.FindByID(gctx, id)
        if err != nil {
            return fmt.Errorf("account lookup: %w", err)
        }
        if acct == nil {
            return domain.ErrAccountNotFound
        }
        p.Account = acct
        return nil
    })

    // 可选:activity。吃掉错误,记 warning,return nil(对 errgroup 不算失败)
    g.Go(func() error {
        activity, err := s.activityService.RecentFor(gctx, id, 10)
        if err != nil {
            log.Warn(gctx, "activity fetch: %v", err)
            mu.Lock()
            warnings = append(warnings, "recent_activity unavailable")
            mu.Unlock()
            return nil
        }
        p.RecentActivity = activity
        return nil
    })

    // 可选:recs
    g.Go(func() error {
        recs, err := s.recService.For(gctx, id)
        if err != nil {
            log.Warn(gctx, "recs fetch: %v", err)
            mu.Lock()
            warnings = append(warnings, "recommendations unavailable")
            mu.Unlock()
            return nil
        }
        p.Recommendations = recs
        return nil
    })

    if err := g.Wait(); err != nil {
        return nil, err  // 只有 account(必需)失败才到这里
    }
    p.Warnings = warnings
    return &p, nil
}
```

### 这个场景对 panic 模型的具体挑战

1. **同一组 goroutine 里,三个不同的错误策略**:account 失败要 propagate,
   activity/recs 失败要吃掉。每个子 goroutine 自己决定"return err vs
   return nil",决策写在闭包里。panic 模型下要在每个 goroutine 顶层 defer
   recover 后,再决定是 re-panic 还是 swallow。
2. **第一个 required 失败要触发 ctx cancel**,让其他子任务及时退出
   (别白白跑完)。这是 `errgroup.WithContext` 的核心价值。panic + 自己
   写 goroutine 池协调,cancel 信号传递要手撸。
3. **可选子任务的错误信息要采集到 `warnings` 切片**。错误的 message
   是数据,要捕获并跨边界传出来。panic 模型下,recover 拿到的是 `any`,
   要类型断言到 error,再做 `.Error()` 取字符串,再 lock 写切片 —— 每
   个 goroutine 都要重复。
4. **`Wait()` 返回的 err 是聚合后的"第一个错"**,语义封装在 errgroup
   里。手撸 panic 版要自己设计这个聚合逻辑。

### 结论

**该场景 errors-as-value 胜**。用户让步。

---

## 4. 场景四:按错误类别分支的重试策略

### 业务约束

调用 `user-service.GetUser(id)`,按错误类别决定重试还是放弃:

| 错误类别 | 处理 |
|---|---|
| `CodeUnavailable` / `CodeTimeout` / `CodeTooManyRequests` | **重试**,指数退避 |
| `CodeNotFound` / `CodeIllegalInput` / `CodePermissionDenied` | **立即放弃** |
| 其它未知 | 保守不重试 |

最多 3 次尝试,指数退避 + jitter。

### error-as-value 版

```go
func (c *UserServiceClient) GetUserWithRetry(ctx context.Context, id string) (*User, error) {
    const maxAttempts = 3
    var lastErr error

    for attempt := 0; attempt < maxAttempts; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(backoffWithJitter(attempt)):
            case <-ctx.Done():
                return nil, ctx.Err()
            }
        }

        user, err := c.GetUser(ctx, id)
        if err == nil {
            return user, nil
        }
        lastErr = err

        if !shouldRetry(err) {
            return nil, err  // 立即放弃
        }
    }
    return nil, fmt.Errorf("after %d attempts: %w", maxAttempts, lastErr)
}

// 先看 RemoteError(driven adapter 已翻译),再退化看普通 AppError
func shouldRetry(err error) bool {
    var re *apperror.RemoteError
    if errors.As(err, &re) {
        switch re.Canonical.Code() {
        case apperror.CodeUnavailable, apperror.CodeTimeout,
             apperror.CodeTooManyRequests:
            return true
        }
        return false
    }
    var ae *apperror.AppError
    if errors.As(err, &ae) {
        switch ae.Code() {
        case apperror.CodeUnavailable, apperror.CodeTimeout:
            return true
        }
        return false
    }
    return false
}
```

### panic 等价版

```go
func (c *UserServiceClient) GetUserWithRetry(ctx context.Context, id string) *User {
    const maxAttempts = 3
    var lastPanic any

    for attempt := 0; attempt < maxAttempts; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(backoffWithJitter(attempt)):
            case <-ctx.Done():
                panic(ctx.Err())
            }
        }

        var user *User
        succeeded := func() (ok bool) {
            defer func() {
                if r := recover(); r != nil {
                    lastPanic = r
                    ok = false
                }
            }()
            user = c.GetUser(ctx, id)  // panics on failure
            return true
        }()
        if succeeded {
            return user
        }

        // 在闭包外做分类决策(defer 里 continue/break 不影响外层 for)
        if !shouldRetryPanic(lastPanic) {
            panic(lastPanic)
        }
    }
    panic(fmt.Errorf("after %d attempts: %v", maxAttempts, lastPanic))
}

func shouldRetryPanic(r any) bool {
    err, ok := r.(error)
    if !ok {
        return false  // panic 的可能是 string/struct,保守不重试
    }
    // ... 同 shouldRetry 的 errors.As + switch
}
```

### 这个场景的具体挑战(**最弱的反例**)

老实说这个比场景 2、3 弱很多 —— panic 版结构上能做,只是多几道手续:

1. **每个 attempt 都要包一层闭包 + defer recover**(panic 没法 inline
   捕获)
2. **`lastPanic any` 丢类型**,要做 `r.(error)` 断言;万一 panic 的不
   是 error,分类 fallback 到"不重试"
3. **决策不能在 defer 里下**(defer 里 `continue`/`break` 不影响外层
   for),必须把 `lastPanic` 拎到外面再判,闭包返回值用 named return
   value 配合
4. **行数差距小**:error 版 ~25 行,panic 版 ~30 行

真正的差距在 idiom 上:error 版的 `if err != nil { if !shouldRetry(err)
return; continue }` 是平铺直叙、可读、可单元测试(直接传 error 进
`shouldRetry`);panic 版要读者理解"为啥要包 closure + defer + named
return"才能看懂控制流。

### 结论

**该场景 errors-as-value 弱胜**。用户让步。

---

## 5. 最终共识

### 决定

| 关注点 | 选择 |
|---|---|
| 主路径错误传递 | **errors-as-value** + apperror.AppError / RemoteError |
| 入站 handler 错误响应 | `errors.As` 抽出 AppError → `HTTPStatusFor` → sanitize body(参见 `internal/adapter/in/rest/error_response.go`) |
| 框架边界安全网 | 保留 panic-recover(hertz `server.Default()` 自带) —— 捕获意外 panic(真 bug、第三方库炸了),转 500 + 日志 |
| 程序员合约违反 | 用 panic(`MustGet` pattern:caller 已经断言"它一定存在",违反就是 bug) |
| 业务错误 | **不用 panic** —— 业务错误是预期可能发生的 outcome,不是 bug |

### 为什么不全面 panic 化的总结

errors-as-value 的几个杀手级特性:

1. **可 inline 检查**:`if err != nil { ... }` 平铺即可分支,无需闭包 + defer
2. **类型安全**:err 是 `error` 接口,`errors.As` 拿到具体类型;panic
   值是 `any`,要类型断言才能用
3. **组合性**:errgroup、retry、AddErrCtx、Unwrap 都建立在"错误是个值,
   可以被中间任何一层 inspect、分类、转换、传递"上;panic 是控制流跳
   转,过了 recover 就丢
4. **工具支持**:`errcheck`、`golangci-lint`、`gopls`、人脑 review 全
   套都假设 error-as-value;改 panic 等于放弃静态分析对错误流的追踪
5. **生态兼容**:stdlib、`database/sql`、hertz handler、`json.Unmarshal`
   全是 error 返回。全面 panic 化要么写一堆 wrap,要么混着用,边界
   confusing

### 为什么 panic 在 Erlang 里好用、在 Go 里不行

用户提议的"failure 处直接 panic、supervisor 边界 recover 重启 / 记录"
本质是 Erlang/OTP 的 "let it crash" 范式。该范式在 Erlang 工作良好,
靠的是**语言级支撑**:

| | Erlang | Go |
|---|---|---|
| 进程创建成本 | ~ns,进程 first-class | goroutine µs 级,且共享内存 |
| 隔离性 | 进程间无共享内存,崩了就崩 | goroutine 共享地址空间,panic 跨边界要小心 |
| 监督树 | 语言内置 supervisor | 没有,得手撸 |
| 错误 pattern match | 原生 `case {ok, X} \| {error, R}` | `if err != nil` 是手写 |

把 "let it crash" 硬移植到 Go,等于**用 Go 模拟 Erlang,但不享受语言加
成**。代价大于收益。

### Go 里减少 `if err != nil` 噪声的可行方向

errors-as-value 的烦在 Go 是真烦,但目前没有更好的替代品。能做的:

1. **`errors.Join` / `cockroachdb/errors`**:聚合 / wrap 一行搞定
2. **提前 return 扁平化**:避免 `if err == nil { ... } else { ... }`
   的嵌套
3. **handler 边界统一收口**:像本项目 `renderError` 那样,handler 只
   `return err`,wire 格式转换集中一处
4. **panic-recover 作为最后安全网**(已经用了)

社区试过更激进的方案(`try` 提案、`?` 操作符)都被 reject,理由都是
"errors-as-value 是 Go 核心契约,不要轻易动"。

---

## 6. 本项目里 panic 的合法用法清单

| 场景 | 合法 panic 吗? | 例子 |
|---|---|---|
| 业务错误(credential taken、unauthenticated、validation failed) | ❌ 用 error | 见 `internal/application/account/sign_up.go` |
| 调下游真错(DB 挂、网络断) | ❌ 用 error | 见 `internal/adapter/out/db/base_repo.go` |
| 程序员合约违反 / 断言失败 | ✅ panic | `IRepo[T].MustGet` —— caller 保证 id 存在,违反 = bug |
| 框架边界兜底 | ✅ recover | hertz `server.Default()` 自带 recovery middleware |
| 真 bug(nil 解引用、数组越界、map 并发写) | ✅(runtime 自动 panic) | 由 hertz recovery 兜底转 500 |
| 配置缺失 / 启动期不可恢复 | ✅ panic 或 `log.Fatal` | `cmd/server/main.go` `hlog.Fatalf("open db: %v", err)` |

简而言之:**panic 给"逻辑上不该发生"的事;error 给"预期可能发生"的事**。
