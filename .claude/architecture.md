# Architecture

A **general guide to the ports & adapters (Hexagonal) architectural style** — its
layers, their relationships, their responsibilities, and the rules that bind
them. It is deliberately **not specific to any project**: it says nothing about
directory layout, package/type names, or nesting depth.

A project adopting this style documents its own **structure-to-architecture
mapping** and any project-specific rules separately (in the project's own docs).
Those supplements **make these general principles concrete — turning them into
explicit, easier-to-follow guidance for that project — or extend them**, but
must never *conflict* with what is here. When a project rule and a rule here
disagree, the disagreement is a bug — surface it.

## Architectural Style

**Core = Domain + Application.** These two layers are the *inside* of the
hexagon — the application's own logic, with no knowledge of HTTP, SQL, or any
vendor. Everything else (the Interfaces layer, driven adapters, Infrastructure)
is *outside* and plugs into the Core through ports.

**Port** — an entry or exit point of the Core: a point of interaction the Core
**defines and owns**, shaped to fit its needs — *not* to mimic an external
service or infrastructure API. A port is a *logical* API; whether it must be a
language-level interface depends on its kind:

- **Inbound (driving) port** — the use-case API the Core *exposes*. Conceptual,
  not physical: usually the use-case types themselves (e.g. a sign-up command),
  needing no language-level interface — a driving adapter may depend on the Core
  directly, so there is no dependency to invert.
- **Outbound (driven) port** — an interface the Core *requires*; here a
  language-level interface *is* required, to invert the dependency so the Core
  needn't depend on the adapter that implements it (e.g. a repository, a password
  hasher, a message sender).

**Adapter** — a class that translates between the outside world and a port. It
either turns an external request into a Core call, or adapts an external API to
an outbound port (in the broad sense — "external" includes a database, a queue,
another service). Two kinds:

- **Driving adapter** — accepts external requests (HTTP, RPC, CLI, …) and calls
  an inbound port to run a use case.
- **Driven adapter** — reacts to a Core action by implementing an outbound port
  and adapting the external API behind it.

## The Layer & Ring Model

Five buckets. Read the picture two ways — as **stacked layers** (Interfaces on
top, Domain at the bottom) or as **concentric rings** (Domain at the center,
Interfaces at the rim). They describe the same ordering; this guide uses both
words interchangeably.

```
  ┌───────────────┐     ┌──────────────────────────────────────────────┐
  │ Infrastructure│ ◀── │ Interfaces  — driving adapters                │  outermost
  │               │     │   inbound protocol handlers, (de)serialize,   │
  │ shared,       │     │   error → wire mapping                        │
  │ reusable      │     └─────────────────────┬────────────────────────┘
  │ technical     │                           │ depends on inbound port
  │ services      │     ┌─────────────────────▼────────────────────────┐
  │               │     │ Application — use cases          ┐            │
  │ (depends on   │     │                                   │ = Core     │
  │  NOTHING)     │     │ Domain      — entities, VOs,      │            │  innermost
  │               │     │   aggregates, rules, invariants,  ┘            │
  │               │     │   outbound port definitions                   │
  │               │     └─────────────────────△────────────────────────┘
  │               │                           │ implements outbound port
  │               │     ┌─────────────────────┴────────────────────────┐
  │               │ ◀── │ Driven adapters — persistence, hashing,       │
  └───────────────┘     │   external-service clients                    │
                        └──────────────────────────────────────────────┘

  Every arrow is a COMPILE-TIME dependency — it points from the dependent to
  what it depends on, not a runtime call or control-flow edge.
```

The three **layers** are Interfaces, Application, and Domain (Application +
Domain = the Core). **Driven adapters** and **Infrastructure** are *not* layers:
driven adapters are the outward implementations of Core-owned ports, and
Infrastructure is shared technical code behind the adapters. Each bucket's
responsibilities are detailed in *Layer Responsibilities* below.

## Dependency Rules

The invariants. Any change must be checked against these.

1. **All source-code dependencies point inward, toward the Domain.** Equivalently:
   any element depends only on elements in its own layer or in the layers
   *beneath* it. Inward and "beneath" mean the same direction.
2. **Domain depends on nothing outside it.** It may use business-agnostic shared
   utilities, but never Application, Interfaces, adapters, or Infrastructure.
3. **Application depends only on Domain** (and shared utilities). It never
   imports an adapter, and never calls HTTP/RPC/SQL directly — it goes through
   outbound ports.
4. **The Core (Domain + Application) never depends on a concrete adapter or on
   Infrastructure.** It depends only on the ports it owns.
5. **Dependency inversion on the driven side.** For an outbound port, runtime
   control flows *outward* (Core → driven adapter → infrastructure), but the
   compile-time dependency points *inward*: the adapter depends on the
   Core-owned port, not the Core on the adapter. This is what keeps the Core
   technology-agnostic and lets adapters be swapped freely.
6. **Only adapters — driving *and* driven — may depend on Infrastructure**, and
   Infrastructure depends on no other layer. (Rule 4 already bars the Core.)
7. **Ports live with the Core, not with the adapter.** An outbound port is
   declared in the Domain or Application layer that needs it; the adapter
   imports the port to implement it — never the reverse.

If a desired change would violate one of these, the design is wrong, not the
rule — stop and reshape it (usually: introduce or adjust a port).

**Applying rule 6 to driving adapters — default to the port.** Rule 6 lets *any*
adapter depend on Infrastructure, but a driving adapter can usually reach infra
two ways, and they are not equal:

- through the Core — `driving adapter → inbound port → Core → outbound port →
  driven adapter → infra`; or
- **directly** — `driving adapter → infra`.

**Default to the port.** If a use case or a domain rule depends on the *result*
of the infra access, that access is part of the use case and MUST go through a
Core port — routing it any other way invents a fake use case and leaks technical
detail into the Core.

**Call infra directly only for a boundary-technical concern that no use case and
no domain rule depends on** — something that *wraps or precedes* a use case
rather than being a step in it. Typical cases: rate limiting / throttling,
tracing and metrics, idempotency-key dedup before dispatch, transport-level
response caching, or streaming a large payload straight to object storage
(routing it through the Core would force buffering the whole body).

Wrapping such a direct call behind an interface for testability is fine, but that
interface is an **Interfaces-layer-local** abstraction, **not** a Core port — the
Core neither owns nor knows it, so the dependency is still `adapter → infra`.

Litmus test: *does a use case or domain rule depend on the result?* Yes → Core
port. No (pure boundary mechanics) → a direct call is allowed.

## Layer Responsibilities

What each bucket holds, and what it must *not* do.

### Interfaces (driving adapters)

- Holds everything that handles inbound interactions from other systems —
  requests coming into the app (a web app, another service, …).
- Handles interpretation, **structural validation**, and translation of incoming
  data; serialization of outgoing data; and mapping Core errors to the wire
  protocol's status.
- Trusts nothing from outside: validates external input at this boundary so the
  Core can assume well-formed input.
- **Must not** contain business rules — it calls an inbound port and shapes the
  result.

### Application

- Defines and implements the **use cases** (e.g. application services and/or
  command handlers — the logic that performs a business process).
- Holds **application logic** — orchestration, transaction control, security
  checks — and delegates **domain logic** to domain objects.
- Declares application-scoped outbound ports whose semantics are tied to the
  application's needs (e.g. a search, messaging, query, or notification
  interface).
- **Must not** call external HTTP/RPC/SQL directly, nor translate protocol
  statuses — both belong in adapters.

### Domain

- Carries the specialized **domain logic** of the problem domain — accumulated
  knowledge that changes infrequently.
- Holds entities, value objects, aggregates, domain events, **business rules and
  invariants**, domain services (logic spanning entities that belongs in no
  single one), and the **repository ports** for aggregates/events.
- **Must not** know about HTTP, RPC, databases, or any technical concern. It is
  the most stable, most isolated layer.

### Driven Adapters

- Implement the Core's outbound ports, adapting an external API to the port the
  Core defined.
- Where translation between the outside and the Core lives (e.g. persisted
  records ↔ aggregates; remote error envelopes → the app's error model).
- Named by **capability, not by vendor**. When several implementations of one
  port coexist, keep them as parallel siblings under that capability.

### Infrastructure

- Provides generic, reusable technical capabilities for adapters (driving & driven).
- **Optional, on demand** — introduce it only when a real need appears, e.g. an
  external service is used in several places and ships no usable client, or its
  API is awkward enough to warrant a shared wrapper.
- Pure technical client code: it does not know that ports exist.

## Request Flow (the shape)

This traces a typical inbound request as **runtime control flow** — the order
calls happen in, *not* the compile-time dependency the Layer & Ring Model shows.
Concrete names vary by project; the *shape* is invariant:

```
  inbound request (HTTP / RPC / CLI / message)
        │
        ▼
1. Driving adapter            Interfaces
        │   parse + structural-validate input; map to the inbound port's input
        ▼
2. Use case                   Application (inbound port)
        │   orchestration; calls domain objects for business logic
        ▼
3. Domain objects             Domain (business rules / invariants)
        │
        ▼
4. Outbound port              defined in the Core
        │                     ── Core calls the outbound port it owns
        ▼
5. Driven adapter             implements the outbound port
        │   translate Core call ↔ external API
        ▼
6. Infrastructure / external system
        │
        ▼   (return path)
   result ──▶ driving adapter serializes the response (and maps errors → API response status)
```

Errors are constructed in the inner layers and propagate outward; the driving
adapter is the only place they are translated — to the wire protocol's status —
and sanitized for the client.

Two opposite directions are in play — don't confuse them:

- **Runtime control flow** (the arrows above): control flows inward to the Core
  (1→3), then back outward to the external system (4→6); the result then returns.
- **Compile-time dependency**: always inward, toward the Domain. Even at 4→6,
  where control flows *outward*, the dependency points the *other* way — driven
  adapter (5) and infrastructure (6) depend on the Core-owned port, never the
  reverse.

That opposition — outward control over an inward dependency — *is* the
dependency inversion of rule 5.
