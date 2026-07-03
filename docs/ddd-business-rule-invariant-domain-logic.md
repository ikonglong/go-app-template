# DDD 概念辨析：business rule、business invariant、domain logic —— 讨论记录与共识

> 围绕两个具体提问展开:(1)在 DDD 里 business rule 与 business invariant 含义
> 有无不同?(2)在 DDD 里 domain logic 与 business rule 含义有无不同?把三个概念
> 及其相互关系一次讲清,既给结论,也保留辨析过程,便于日后回看为什么这么定。
>
> 适用范围:阅读 / 评审 `internal/domain/`(聚合、不变量、port)与
> `internal/application/`(用例编排)时的概念基准。配套阅读:
> `.claude/architecture.md`(分层定义)、`.claude/rules/domain.md`(domain 纯粹性
> 与聚合规则)、`.claude/CLAUDE.md`「校验分层」一节、
> `docs/uniqueness-constraint-layering-discussion.md`(一条具体约束的分层归属)。

## 目录

1. [一句话结论](#1-一句话结论)
2. [三个概念各自是什么](#2-三个概念各自是什么)
3. [关系一:invariant 与 rule —— 同维度的种属关系](#3-关系一invariant-与-rule--同维度的种属关系)
4. [关系二:rule 与 domain logic —— 不同维度的知识与实现](#4-关系二rule-与-domain-logic--不同维度的知识与实现)
5. [统一关系图](#5-统一关系图)
6. [落到本项目](#6-落到本项目)
7. [共识速查](#7-共识速查)

---

## 1. 一句话结论

- **business invariant ⊊ business rule**:**同一维度**上的种属关系。不变量是
  规则中"必须恒为真"的那一类;所有不变量都是规则,但大量规则不是不变量。
- **business rule vs domain logic**：**不同维度**，不是种属关系。前者是
  *声明式的知识*(「什么必须为真 / 该发生什么」),后者是*领域层承载的行为实现*。
  规则被领域逻辑实现,而领域逻辑做的事**多于**实现规则。

辨析这三者不是咬文嚼字:它直接决定**聚合怎么划、规则该放哪一层、什么时候用
强一致还是最终一致**。

---

## 2. 三个概念各自是什么

### business rule(业务规则)—— 一条领域**知识**

对业务的一条约束或规定,本质是**知识**,声明式、名词性,不关心"在哪实现"。
常能被领域专家用统一语言(ubiquitous language)说出来。形式多样:

| 类别 | 例子 |
|---|---|
| 校验类 | 订单金额不能为负 |
| 计算 / 推导类 | 订单总价 = 各行项小计之和 − 折扣 |
| 策略类 | VIP 客户享 9 折;周末不发货 |
| 反应 / 触发类 | 库存低于阈值触发补货;超 24 小时未支付自动取消 |
| 授权类 | 只有管理员能关闭账户 |

它的对立面是**技术规则**(连接池大小、重试次数)——区分轴是**来源:业务 vs 技术**。

### business invariant(业务不变量)—— 一类**特殊的规则**

特指**在任何「可观察的一致性时刻」都必须恒为真的状态约束**。词源自形式化方法的
invariant(契约式设计的类不变量、Hoare 逻辑里贯穿所有操作都保持的性质)。
典型不变量:账户余额不能为负;订单至少含一个行项;订单总额必须等于各行项之和;
同一时段一个会议室不能被两个会议占用。

### domain logic(领域逻辑)—— 领域层承载的**行为总和**

"实现 / 承载领域知识的那部分代码与行为",是个**按层、按种类**划分的范畴。它的
对立面是 **application logic / infrastructure logic / presentation logic**——区分轴是
**归属:哪一层、哪一类**。注意它和 business rule 不是对着说的。

---

## 3. 关系一:invariant 与 rule —— 同维度的种属关系

不变量是规则的一个**真子集**。把普通规则和不变量拉开距离的几个维度:

| 维度 | invariant | 一般的 rule |
|---|---|---|
| 时态 | 无条件、**始终**为真 | 常是有条件 / 触发式 / 时间相关 |
| 形式 | 对状态的布尔谓词(consistency condition) | 还可以是推导、计算、反应、策略 |
| 强制边界 | 绑定在**聚合**上,单事务内**原子**保证 | 可跨聚合,可最终一致 |
| 责任人 | 聚合根负责守护 | 可散落在 domain service / policy / 应用层 |

**反例(是 rule 但不是 invariant):**

- 「满 100 减 20」——计算 / 策略,不是「状态恒为真」的约束。
- 「超 24 小时未支付自动取消」——时间触发的反应规则(reactive policy)。
- 「跨聚合的库存与订单一致」——通常做成**最终一致**(领域事件 + saga),恰说明
  它不被当作必须原子守护的不变量。

判定窍门:凡你会脱口而出「这个条件**任何时候**被破坏,对象就处于非法状态」的,
基本就是不变量。

**注意 invariant ≠ validation**:validation 往往是边界上对输入的一次性检查,
且允许对象在装配过程中"暂时不合法";invariant 谈的是对象一旦存在,其状态就**恒**合法。

---

## 4. 关系二:rule 与 domain logic —— 不同维度的知识与实现

业务规则是**被实现的知识**,领域逻辑是**实现它的行为**——但领域逻辑**不止**
实现规则:

```
domain logic(领域层的全部行为)
├── 业务规则的实现   ← 这里和 business rule 重叠(含不变量、校验、策略、推导)
└── 非规则的领域行为
    ├── 纯计算 / 推导(运费 = Σ 行项重量 × 费率)
    ├── 多实体协调 / 编排(domain service:转账时驱动两个账户)
    ├── 状态机迁移、产生领域事件
    └── 聚合内导航 / 查询
```

两个方向的"不重合"反例,正好框住边界:

**① 是 domain logic,但不是 business rule**
聚合内的状态迁移、产生 `OrderPlaced` 事件、domain service 把转账拆成"扣 A、加 B"
的协调步骤——这些是领域行为,但本身不是一条"约束 / 规定"。规则(不能透支、金额
必须为正)是嵌在这套行为**里**的那几条判断。

**② 是 business rule,但跑到了 domain logic 之外**
同一条「邮箱唯一」的业务规则,可落在:domain 的工厂校验(= domain logic)、
application 层校验、DB 的 UNIQUE 约束、甚至 UI 提示。规则作为**知识**没变,但其
**实现位置**可以不在 domain logic 里。DDD 恰恰关心这一点:业务规则一旦从 domain
logic 漏到 application / UI 层,就是**贫血模型**的味道——所以这两个概念必须分得开,
你才能判断"这条规则放对地方了没有"。

---

## 5. 统一关系图

```
                 知识层(声明式)              实现层(行为)
business rule  ─────●────────────────────┐
  └ invariant      (§3:不变量 ⊊ 规则)     │  被实现为
                                          ├──────────►  domain logic
技术规则        ─────○(非业务,不进 domain)│           (与 application / infra logic 并列)
```

- **invariant ⊊ business rule**:不变量是"必须恒为真"的那类规则(种属关系,同一维度)。
- **business rule ↔ domain logic**:**不是**种属关系,而是"**知识 ↔ 承载该知识的
  行为**";domain logic 在行为上**多于**规则实现,而 business rule 作为知识可被放到
  domain logic 之外(应避免)。

---

## 6. 落到本项目

这套区分在仓库里有直接对应:

- **谈"知识该归到哪层"** —— `.claude/CLAUDE.md`「校验分层」写
  「Domain layer — business rules and invariants live here」,把规则与不变量都归到 domain。
- **谈"行为该落到哪层"** —— `internal/domain/` 被描述为
  "carries the specialized knowledge (i.e. **domain logic**)";`internal/application/`
  "delegates domain logic to domain"。用词不同,正因前者谈知识、后者谈实现归属。
- **不变量的守护时机** —— `.claude/rules/domain.md` 把不变量校验的归属定在工厂:
  `CreateAccount` 是 invariant validation 的"future home"(出生时守护),
  `RebuildAccount` 跳过(数据出库时已合法)。这正是「聚合即不变量的一致性边界」
  在本项目的落点。
- **规则放哪层的实战推导** —— 见
  `docs/uniqueness-constraint-layering-discussion.md`:邮箱 / 手机唯一这条规则
  当前分散在应用层(编排)、领域层(哨兵错误的词汇)、DB(权威 UNIQUE 保障)
  三处,是"同一条 business rule 的实现不全在 domain logic 里"的真实案例。

---

## 7. 共识速查

| 问题 | 结论 |
|---|---|
| invariant 和 rule 有区别吗? | 有。invariant ⊊ rule,同维度的种属关系 |
| invariant 的判定标准? | "任何时刻被破坏,对象即非法"的状态谓词;绑聚合、单事务原子守护 |
| rule 和 domain logic 有区别吗? | 有,且不同维度。rule 是声明式知识,domain logic 是领域层的行为实现 |
| 三者是否都是包含关系? | 否。只有 invariant ⊊ rule 是包含;rule 与 domain logic 是"知识 ↔ 实现" |
| domain logic 只做规则吗? | 否。还含计算、协调编排、状态迁移、产生领域事件等非规则的领域行为 |
| 业务规则一定在 domain logic 里吗? | 不一定,但应尽量是;漏到 application / UI 即贫血模型的信号 |
| 这套区分到底决定什么? | 聚合怎么划、规则放哪层、用强一致还是最终一致 |
