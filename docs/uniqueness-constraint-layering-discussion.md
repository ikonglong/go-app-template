# 唯一性约束放哪一层：email/phone 不可重复的分层归属 —— 讨论记录与共识

> 围绕一个具体问题展开：「sign up 时 email 或 phone 已存在则不允许注册」这条
> 业务约束，按本项目的架构约定应该落在哪一层？当前落在哪？如果要放 domain，
> 逻辑该放哪里才合适？既是结论，也保留推导过程，便于日后回看为什么这么定。
>
> 适用范围：`internal/application/sign_up.go`（用例编排）、`internal/domain/`
> （聚合与 port）、`internal/adapter/repo/`（持久化适配器）。配套阅读：
> `.claude/architecture.md`（分层定义）、`.claude/rules/domain.md`（domain 纯粹性
> 与聚合规则）、`.claude/CLAUDE.md`「校验分层」一节、`.claude/error_handling.md`
> （错误模型）。

## 目录

1. [现状：约束实际分散在三处](#1-现状约束实际分散在三处)
2. [为什么它进不了 Account 聚合](#2-为什么它进不了-account-聚合)
3. [若放 domain，唯一合法载体是 domain service](#3-若放-domain唯一合法载体是-domain-service)
4. [纠正一句错误的口号：「要 IO 所以不能放 domain」](#4-纠正一句错误的口号要-io-所以不能放-domain)
5. [当前不抽 domain service 的真正理由：表达力，不是纯粹性](#5-当前不抽-domain-service-的真正理由表达力不是纯粹性)
6. [不变的事实：权威保证永远在 DB](#6-不变的事实权威保证永远在-db)
7. [何时才值得迁到 domain service](#7-何时才值得迁到-domain-service)
8. [共识速查](#8-共识速查)

---

## 1. 现状：约束实际分散在三处

这条约束不是落在单一层，而是按职责分散在三处，中心在**应用层**。

| 位置 | 文件 | 承担什么 |
|---|---|---|
| 应用层（中心，判定的编排） | `internal/application/sign_up.go:51`、`:81-107` | `SignUpCmd.Run` 调 `assertCredentialAvailable`，用 `repo.FindByEmail` / `repo.FindByPhone` 查询既有账号，命中则返回 `domain.ErrAccountCredentialTaken` |
| 领域层（判定的词汇） | `internal/domain/account.go:33-37` | 定义哨兵 `ErrAccountCredentialTaken`（`CodeAlreadyExists` + case `account_credential_taken`），且故意不透露是 email 还是 phone 冲突（防枚举） |
| 数据库（权威的最终保障） | 迁移脚本中 email / phone 上的 UNIQUE 约束 | 并发 insert 场景下真正阻止重复的那道闸 |

应用层那段的形态：

```go
func (c *SignUpCmd) Run(ctx context.Context, in SignUpInput) (SignUpOutput, error) {
    if err := c.assertCredentialAvailable(ctx, in.Email, in.Phone); err != nil {
        return SignUpOutput{}, err
    }
    // ... hash 密码 → domain.CreateAccount(...) → c.repo.Add(...)
}
```

一句话定性：**有意做业务判定的是应用层，保证正确性的是 DB 的 UNIQUE 约束，领域层只提供这条规则的"语汇"（哨兵错误）。**

---

## 2. 为什么它进不了 Account 聚合

容易混淆的前提是「业务规则属于 domain」——但这不意味着所有业务规则都属于*聚合本身*。关键看这条约束的**性质**。

「email/phone 全局唯一」不是单个 `Account` 的不变量（invariant），而是**跨聚合的集合约束**（set-based / cross-aggregate constraint）。DDD 里聚合是一致性边界：聚合内部的不变量能在聚合内强一致地保证，但"在所有 account 里唯一"跨越了无数个 `Account` 实例，根本不在任何单个聚合的边界内。

落到代码上看得更清楚：`CreateAccount` 工厂（`internal/domain/account.go:93`）正是放单聚合不变量的地方（其注释明说"This is the canonical place to enforce invariants on brand-new aggregates"），但它**看不到其他 account**，所以物理上无法判断唯一性。这条规则进不了聚合不是设计取舍，是它的性质决定的。

---

## 3. 若放 domain，唯一合法载体是 domain service

`architecture.md` 自己给了载体定义：

> domain service: carries the domain logic that involves different entities, of the same type or not, and does not belong in the entities themselves.

唯一性正好是"涉及多个 `Account`、不属于任一 `Account` 自身"的逻辑。所以若一定要放 domain，唯一合法的落点是一个 domain service（或 Specification），形如：

```go
// internal/domain/ 下
type CredentialUniquenessChecker struct {
    repo IAccountRepo   // 依赖 port，不是实现
}
func (c *CredentialUniquenessChecker) Check(ctx context.Context, email, phone *string) error { /* ... */ }
```

注意：它依赖 `IAccountRepo` **不破坏 domain 纯粹性**——这个 port 本身就定义在 domain 层（`internal/domain/repo.go` 的 `IRepo[T]` + `internal/domain/account.go:265` 的 `IAccountRepo`）。domain service 依赖自己层的接口完全合法，没有 import 任何 adapter / infra。

---

## 4. 纠正一句错误的口号：「要 IO 所以不能放 domain」

讨论中一度用「唯一性要查 DB，放不进纯粹的 domain」来解释为什么放应用层。**这句口号是错的，应当纠正。** 没有任何一条约定写着"因为要 IO 所以唯一性不能放 domain"——它把两条不同的约定压缩坏了。拆开看：

**约定一（职责分配）** —— `CLAUDE.md`「校验分层」：

> Domain layer — business rules and invariants live here; **checks that need IO (e.g. uniqueness) are orchestrated by the application service.**

措辞是 "orchestrated by the application service"——把唯一性检查的**编排职责划给应用层**。它甚至*预设了*唯一性检查需要 IO，然后说这活儿由应用层编排。它**没有**说"domain 不许碰"。

**约定二（纯粹性）** —— `rules/domain.md`「Purity」+ `CLAUDE.md` 依赖规则：

> Domain code imports nothing from the rest of the project... the domain stays free of side effects.
>
> `domain/` references no infrastructure at all — not go-jet, `database/sql`, HTTP, or time-of-day side effects.

**关键辨析：约定二推不出"domain service 不能调 repo port"。** 因为 `IAccountRepo` 是 domain 自己定义的 port，不是 infrastructure。domain service 持有并调用它：

- 不违反 "references no infrastructure"（引用的是 domain 自己的接口）；
- 不违反 "imports nothing from the rest of the project"（没 import adapter / infra）。

依赖倒置（DIP）的全部意义就是这个：domain 通过自己定义的抽象触发外部行为，代码本身保持纯粹，副作用落在接口背后的 adapter 里。所以从纯粹性出发，domain service 调 repo port **在理论上是合法的**。

**那为什么项目里 domain 事实上确实不主动调 port？** 这是项目的*姿态*，不是硬禁令。看 clock 的处理就懂：`CLAUDE.md` 依赖规则说应用层"wall-clock access goes through the clock port"，而 `rules/domain.md` 说 domain 这边是：

> The clock comes in as a `time.Time` value supplied by the caller.

即**副作用的*触发*停在应用层，domain 只接受应用层求值后喂进来的纯值/纯结果**（`CreateAccount(... now time.Time)` 就是这样，见 `account.go:93`）。这比"DIP 允许 domain 调 port"更严格一档，但它是项目选定的纯粹性风格，不是"IO 在物理上没法放 domain"。

**纠正后的准确表述：**

> 不是"要 IO 所以不能放 domain"，而是"项目约定把需 IO 的唯一性检查的*编排*划给了应用层（约定一），且 domain 按惯例不主动触发副作用、只吃纯值（纯粹性姿态）"。

---

## 5. 当前不抽 domain service 的真正理由：表达力，不是纯粹性

把现在这段逻辑搬进 domain service，会发现它内部除了"调 `FindByEmail` / `FindByPhone` + 判 nil + 返回 `ErrAccountCredentialTaken`"之外**没有任何领域知识**。这种"只是在编排 repo 调用"的东西伪装成 domain service，反而是过度设计：它把应用层的本职（编排 domain + repo 完成 use case）换了个包名假装成领域逻辑。

而"查询是否已存在 → 不存在则创建 → 持久化"这整条流程，正是六边形架构里 use case orchestration 的标准形态，是应用层的天职。所以 `CLAUDE.md` 那条职责分配不是偷懒妥协，而是 deliberate 的架构决策，且有 DDD 依据。当前 `sign_up.go` 的结构恰好是它的正确落地。

> 模板哲学（`CLAUDE.md`：模板不受 rule of three 约束、"模式即交付物"）会不会要求这里完整建模成 domain service？不会。模板要展示的"规范模式"恰恰*就是*"需 IO 的唯一性检查由应用层编排"这一模式本身——它已被写成约定。完整建模 ≠ 一定要 domain service。

---

## 6. 不变的事实：权威保证永远在 DB

无论检查放哪一层，真正保证唯一性的永远是 **DB 的 UNIQUE 约束**。原因是任何"先查后插"都有 TOCTOU 竞态——`sign_up.go:16-20` 的注释已明确点出这一点：

> Concurrent-uniqueness gap: the existence checks below race against concurrent inserts. The database UNIQUE constraints on email and phone remain the authoritative guard...

domain / application 里的前置检查本质只是 **fail-fast + 友好错误信息**，不是权威保证。这反过来也说明：为这层"非权威的前置检查"专门起一个 domain service，性价比更低。

**当前状态（已实现）**：repo 适配器（`internal/adapter/repo/jet/base_repo.go`）已实现两层错误翻译——`translateConstraintErr` 将 UNIQUE 冲突包装为 `RemoteError(AlreadyExists)`，`translateError` 将其他数据库错误按 SQLSTATE 分类为对应 Code 的 RemoteError。Application 层 `wrapRepoErr`（`internal/application/repo_err.go`）再将 `RemoteError(AlreadyExists)` 包装为 `ErrAccountCredentialTaken`。竞态下并发写入者不再收到通用 500，而是收到语义正确的 `CodeAlreadyExists` 响应。

---

## 7. 何时才值得迁到 domain service

判断标准只有一个：**"什么算冲突"这件事开始包含真正的领域知识时。** 例如：

- email 规范化后再比较（大小写折叠、Gmail 的 plus-addressing / 点号归一）；
- phone 先 E.164 归一化再比较；
- 软删除的账号，其 email / phone 是否允许被复用；
- 同一规则要被多个用例复用（signup、admin 邀请、改绑邮箱……）。

到那时，"什么算同一个 credential"是值得沉淀成 domain service / Specification 的领域概念。**但即便如此，"去 repo 查一下"这一步仍由应用层编排。** 换句话说：将来要拆，拆的是*判定规则*下沉到 domain，而不是把*查询编排*搬下去。

---

## 8. 共识速查

- **现状正确**：约束分散三处——判定编排在应用层、错误词汇在领域层、权威保证在 DB UNIQUE。这符合架构约定，无需改动判定的归属。
- **进不了聚合**：唯一性是跨聚合集合约束，不是单聚合不变量，`CreateAccount` 看不到其他 account，物理上做不到。
- **放 domain 的唯一合法形态**是 domain service（依赖 `IAccountRepo` port，不破坏纯粹性），但当前逻辑太薄、无领域知识，强放属过度设计。
- **直接依据**是 `CLAUDE.md`「需 IO 的唯一性检查由应用层编排」这条**职责分配**，不是"domain 禁止 IO"（后者不存在）。
- **补上的环节**：repo 适配器（`base_repo.go`）将 DB UNIQUE 冲突翻译为 `RemoteError(AlreadyExists)`，application 层 `wrapRepoErr` 再包装为 `ErrAccountCredentialTaken`。并发竞态下不再产生通用 500。
- **下沉时机**：当"何为冲突"长出领域知识（规范化、归一化、软删复用规则）或被多用例复用时，才把*判定规则*抽进 domain service；查询编排始终留在应用层。
