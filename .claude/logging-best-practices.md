## Log Levels

Levels used for identifying the severity of an event. Levels are organized from most severe to least:

- **FATAL**: a very severe error event that will presumably lead the application to abort.
- **ERROR**: an error event that might still allow the application to continue running, and possibly is recoverable.
- **WARN**: an event that indictes a potentially harmful situations, and might possible lead to an error.
- **INFO**: informational messages that highlight the progress of the application at coarse-grained level.
- **DEBUG**: fine-grained informational events that are most useful to debug an application.
- **TRACE**: finer-grained informational events than the DEBUG level, typically capturing the flow through the application.

## Quick Reference

| Level | What to log | Mindset |
|-------|------------|---------|
| **TRACE** | Entry/exit, every iteration, every intermediate variable | "Replay the exact execution path" |
| **DEBUG** | Decision points, computed values, key inputs/outputs | "Reconstruct what happened" |
| **INFO** | Business milestones — started, succeeded, key outcomes | "Operator dashboard in text form" |
| **WARN** | Anomalies that succeed now but may fail soon | "Early warning system" |
| **ERROR** | Operation failed, system continues — full diagnostic context | "Diagnose without a reproducer" |
| **FATAL** | Unrecoverable — system must stop, post-mortem context | "Last words before shutdown" |

## Choosing a level

There is **no single criterion** that picks among all five levels — each
adjacent pair of levels has its own boundary question. Find the boundary
you're unsure about, then apply its question.

| Boundary | Question | If "yes" → |
|----------|---------|------------|
| **DEBUG ↔ INFO** | Can an operator safely ignore this log? | DEBUG |
| **INFO ↔ WARN**  | Does this event deviate from the normal path, even though nothing has failed? | WARN |
| **WARN ↔ ERROR** | Has the current operation failed, **or** does an operator need to act now? | ERROR |
| **ERROR ↔ FATAL**| Is the system unable to continue running? | FATAL |

TRACE sits below DEBUG — choose it when the goal is to **replay the exact
execution path**, not just to reconstruct what happened.

> A common mistake is to apply one criterion (e.g., "does an operator
> need to act?") to all boundaries. That criterion only draws the
> WARN↔ERROR line; using it elsewhere either suppresses useful INFO or
> over-promotes routine events to WARN/ERROR.

**Note:** The examples below illustrate principles, not templates. Understand the reasoning behind each logging decision and adapt to your specific logging context — do not copy them verbatim.

## Examples

- [All Logging Level Examples](logging-examples-by-level.md) — each level demonstrated independently in an order processing scenario
- [Unified Example (Production Style)](logging-unified-example.md) — all levels coexisting in one function; key principle: lower levels **add detail**, not **repeat**
- [State Machine Transition Example](logging-state-machine-example.md) — logging state transitions; key principle: each transition is a business milestone (INFO), volume is not a reason to downgrade
