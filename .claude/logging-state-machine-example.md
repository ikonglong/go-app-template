## State Machine Transition Example

State transitions are a recurring logging scenario. The key insight: each
transition is a business milestone for that specific entity — so it belongs at
**INFO** by default, even when many entities transition concurrently.

The example below uses a `Task` state machine to show how INFO, WARN, ERROR,
and DEBUG coexist around a single `try_transition()` function (the
speculative variant — see "Logging rejected transitions" below for the
naming and the strict `must_transition()` counterpart).

```
function try_transition(task, new_state, trigger):
    old_state = task.state

    // Rejected (illegal) transition. DEBUG here is consistent with the
    // speculative `try_transition()` contract — the caller may
    // legitimately attempt without knowing validity. For the strict
    // `must_transition()` variant, this branch would log ERROR instead.
    // See "Logging rejected transitions" below for how to choose.
    if not is_valid_transition(old_state, new_state):
        DEBUG("Rejected state transition: task_id={}, current={}, attempted={}, trigger={}",
              task.id, old_state, new_state, trigger)
        return false

    task.state = new_state

    // INFO: the business milestone. Each task's A→B happens once for that task,
    // so each line carries unique business value. The `trigger` field is
    // critical — A→B alone is not enough; operators need to know WHY
    // (user action, timeout, system event, upstream signal).
    INFO("Task state changed: task_id={}, from={}, to={}, trigger={}",
         task.id, old_state, new_state, trigger)

    // WARN: transitioned into a degraded/abnormal state. The operation has
    // not failed yet, but the new state warrants attention before it does.
    if new_state in (RETRYING, SUSPENDED, DEGRADED):
        WARN("Task entered degraded state: task_id={}, state={}, reason={}, retry_count={}",
             task.id, new_state, trigger, task.retry_count)

    // ERROR: transitioned into a terminal failure state. Include full
    // diagnostic context (customer_id, last_error, duration) so the failure
    // can be analyzed without needing a reproducer.
    if new_state == FAILED:
        ERROR("Task failed: task_id={}, from={}, trigger={}, error={}, customer_id={}, duration_ms={}",
              task.id, old_state, trigger, task.last_error, task.customer_id,
              now() - task.started_at)

    return true
```

## Logging rejected (illegal) transitions

There is no single right level for the `is_valid_transition` rejection
branch — it depends on the **API contract** of the transition function.
Two designs are common, and they call for opposite log levels. A clear
naming convention makes the contract visible at every call site.

### Contract A: speculative / probe-style API — `try_transition()`

Callers may legitimately call `try_transition()` without knowing whether
the move is valid. The boolean return is part of normal control flow —
the caller inspects it and proceeds either way. The `try_` prefix
signals to readers that failure is an expected outcome, not an error.

Typical scenarios:
- **Idempotent retries.** `try_transition(task, COMPLETED, "user")` is
  safe to call more than once; the second call's rejection is expected.
- **Benign races.** A timeout handler and a user action both attempt
  `→ COMPLETED`; one must lose, and that's fine.
- **Using the return value as a built-in `can_transition()` check** —
  the caller intentionally tries and falls back on the `false` branch.

→ **Log at DEBUG.** A rejection here is normal flow, not a bug. INFO
would create noise; ERROR would cry wolf.

### Contract B: command-style API — `transition()` or `must_transition()`

Callers must guarantee the transition is legal before calling — usually
by gating with `can_transition()` first, or because the surrounding
control flow has already established the source state. A rejection
means the caller has a bug: wrong state assumption, missing precheck,
lost invariant.

Either name signals at the call site that rejection is **not** an
expected outcome — that's the contract this guide cares about. *How*
the function reports rejection (error return, panic, assert, …) is an
error-handling decision outside this doc's scope; follow your project's
error-handling guidelines for that.

Typical scenarios:
- **Internal state machines** where every call site knows the current
  state by construction.
- **Workflow engines** where the next step is computed from the prior
  step's outcome.
- **Cases where rejection has no meaningful recovery at the call site**
  — it can only be treated as a bug.

→ **Log at ERROR.** This is a coding error, not a business event.
Silently swallowing it at DEBUG hides the bug in production, where
DEBUG is usually disabled.

### How to tell which contract applies

Look at the **callers** of the transition function, not the function
itself. Three questions, in order of usefulness:

1. **Is there a non-error code path that handles a rejected transition?**
   - Yes (retry, skip, fall through to a different state) → Contract A → `try_transition()` → DEBUG.
   - No — rejection can only be surfaced as a bug → Contract B → `must_transition()` → ERROR.

2. **Could the function be replaced with a strict variant (rejection
   treated as a bug) and the system still work correctly?**
   - Yes → Contract B → ERROR.
   - No (real code legitimately calls it speculatively) → Contract A → DEBUG.

3. **When you see `Rejected state transition` in the logs at 3 AM,
   is it useful information or evidence of a bug?**
   - Useful signal (race telemetry, retry stats) → DEBUG.
   - Bug → ERROR.

### When both contracts coexist

If one caller is speculative and another is strict, **don't try to
encode both behaviors in one function** — the log level becomes
ambiguous, and the function ends up logging DEBUG for real bugs.

Split the API instead:
- `try_transition()` — speculative variant; rejection is part of normal
  flow. Logs DEBUG on rejection.
- `transition()` / `must_transition()` — strict variant; rejection is a
  bug. Logs ERROR on rejection.

Each caller picks the variant that matches its assumption — the name
itself signals the contract, and the log level is unambiguous at every
call site. (The choice between `transition()` and `must_transition()`,
and how either one reports rejection to the caller, follows your
project's error-handling conventions — not covered here.)

## When NOT to use INFO for transitions

Downgrade to DEBUG only when the transition is an **implementation detail**,
not a business milestone — i.e., when operators reading the logs can safely
ignore it.

Examples that should be DEBUG, not INFO:
- Chunk-level micro-states inside a larger operation:
  `DOWNLOADING_CHUNK_1 → DOWNLOADING_CHUNK_2 → ... → DOWNLOADING_CHUNK_N`
  (the outer `PENDING → DOWNLOADING → COMPLETED` stays INFO)
- Protocol-internal states: TCP `SYN_SENT → SYN_RECEIVED → ESTABLISHED`
- High-frequency UI/system states: animation frames, heartbeat ticks

**Volume alone is NOT a reason to downgrade.** If thousands of tasks transition
A→B concurrently, each transition is still a unique business milestone for its
task — that log line is the only evidence that *this specific task* reached B.
Solve volume with infrastructure, not by suppressing logs:
- Sampling (e.g., emit 10% of high-volume transitions to disk)
- Tiered retention (INFO kept 7 days, DEBUG kept 1 day)
- Structured logging + aggregation (Kibana/Loki, filter by `task_id`)

Downgrading to DEBUG typically means losing the data in production, since DEBUG
is usually disabled there. When something breaks, you cannot answer "did task X
ever reach state B?" — because the only record of that transition is gone.

The decision rule: **can an operator safely ignore this transition?**
If yes, it is DEBUG. If no, it is INFO regardless of volume.
