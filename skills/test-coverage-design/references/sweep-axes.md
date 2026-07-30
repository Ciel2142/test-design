# Sweep axes

Sweep **every behavior against all twelve axes**. The default failure mode this exists to
prevent is happy path plus one null check, then declaring done.

An axis is either swept into rows or explicitly ruled out. Silent omission is forbidden.

| # | Axis | Probes |
|---|------|--------|
| 1 | Boundaries / cardinality | 0, 1, many, max, max+1, empty, negative, huge |
| 2 | Data shape | null/missing, wrong type, unicode/emoji, whitespace, length overflow, injection payloads, bad encoding |
| 3 | Failure paths | dependency down, timeout, partial failure mid-operation, quota/disk full, malformed dependency response |
| 4 | Idempotency / retry | same request twice, retry after a timeout that actually succeeded, at-least-once delivery |
| 5 | Concurrency / ordering | see sub-axes below |
| 6 | Auth / permission | no actor, wrong actor, expired credential, insufficient scope, cross-tenant access |
| 7 | State / lifecycle | illegal transition, operation on deleted or archived entity, double-close, use-before-init |
| 8 | Time | timezone, DST, clock skew, expiry boundary, timestamp ties |
| 9 | Resource limits | oversized payload, connection exhaustion, rate limit reached |
| 10 | Contract / observability | error code and message correctness, failure actually logged and metered, API/schema backward compatibility |
| 11 | Rendering / interaction | loading, empty, and error states; focus and keyboard order; responsive breakpoints; i18n layout; optimistic-update rollback |
| 12 | Migration / rollout | forward and backward compatibility during rolling deploy, old and new code against one schema, partial-migration resume, production-scale volume |

## Axis 5 sub-axes

Enumerated separately because concurrency is the most-skipped axis and the least
catchable by a unit test.

- two writers on the same entity (lost update)
- check-then-act / TOCTOU (read-modify-write with no lock)
- out-of-order delivery (event B arrives before event A)
- duplicate delivery (at-least-once broker semantics)
- transaction isolation anomalies (dirty read, phantom, write skew)
- deadlock / inconsistent lock ordering
- concurrent initialization, cache stampede, double-start
- cancellation mid-flight (client aborts, context cancelled)
- shared mutable state across threads, goroutines, or async tasks

## Ruling out an axis

Every ruled-out axis needs a reason **citing a spec fact or a code fact**, not a judgment.

<Good>
`Ruled out: time (spec defines no date logic), auth (internal endpoint, spec §3 states no
actor concept), migration (no schema change — no new Liquibase changeset in this feature)`
</Good>

<Bad>
`Ruled out: time (n/a), auth (n/a), concurrency (no shared state)`
</Bad>

`no shared state` is unfalsifiable at design time, and axis 5 is the flagship — dismissing
it in one unchecked clause is the single easiest way to gut the sweep. **Ruling out axis 5
specifically requires naming the shared resource that is absent:** no table written, no
cache, no lock acquired, no queue consumer, no static or module-level mutable value.

## Pruning

Sweeping twelve axes across several behaviors generates far more candidates than any plan
should carry. Prune hard, immediately after sweeping:

1. Each candidate must name **one distinct code-level mutation** it would catch.
2. A candidate whose mutation cannot be named is deleted.
3. Two candidates naming the same mutation are merged; the loser's ID is retired.

The ceiling of roughly forty rows applies to survivors, never to candidates. A ceiling on
candidates would reward under-sweeping, which is backwards.
