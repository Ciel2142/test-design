# Coverage: order creation

Spec: docs/superpowers/specs/2026-07-30-order-creation-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Baseline failures

None — the suite was green at the baseline commit.

## Invariants

- INV-1 no partially-written order is ever visible to a reader
- INV-2 a retried create with the same idempotency key produces no second row

## Out of scope

- throughput beyond 1k rps — the spec states no performance requirement
- multi-region failover — deferred, not in this spec

## Retired

- SC-04 merged into SC-03 (same mutation: uniqueness check outside the transaction)

## Scenarios

### Behavior: create an order

Axes: SC-01 axis 1, SC-02 axis 3, SC-03 axis 4 and axis 5, SC-05 axis 3

Ruled out: data shape (spec §2 defines two scalar fields, `qty: integer` and an idempotency
key, enforced by the request schema before this handler runs — no free-text field to probe),
time (spec defines no date logic or expiry), auth (internal endpoint, spec §3 states no
actor concept), state/lifecycle (an order has no transitions after creation — no update,
cancel, or archive verb in this spec), rendering (API only, no UI surface),
contract/observability (error codes and body shape are already asserted by SC-02 and
SC-05's `Expected` column; spec defines no separate logging/metrics contract), migration
(no schema change — no new changeset in this feature), resource limits (payload is two
scalar fields)

| ID | Behavior | Bug it catches | Level | Expected | Risk |
|----|----------|----------------|-------|----------|------|
| SC-01 | quantity at the lower bound is rejected | guard uses `<` instead of `<=` | unit | throws "qty must be > 0", no row persisted | med |
| SC-02 | write rolls back when the commit hook fails | rollback call omitted on the error path | integration | 0 rows persisted, INV-1 holds | high |
| SC-03 | two writers with one idempotency key | uniqueness check runs outside the transaction (TOCTOU) | integration | 1 row, second call answers 200 | high |
| SC-05 | dependency down answers 503, not 500 | error mapped to the generic handler instead of the unavailable branch | smoke | 503, body reports `rows: 0`, message not acked | high |

harness SC-02: in-process store, write hook throws on invocation
harness SC-03: 2 parallel clients, real store, no mocks, 50 iterations fail-if-any
harness SC-05: service started with `FAIL_DB=1`, `POST /orders {"qty":3}`

### Notes on this example

`SC-01` is `med` because axis 1 is not in the high-by-rule set. `SC-02` and `SC-05` are
axis 3, `SC-03` is axis 4 and axis 5 — all `high` by rule, so all three require layer 4
mutation evidence.

`SC-03` is `integration`, not `unit`, because two mocked concurrent calls execute
sequentially in one thread and would pass whether or not the check sits inside the
transaction. Its harness names an iteration count, because a single green run of a race
test records `INCONCLUSIVE`, not `PASS`.

Every `Bug it catches` names something a person could type into the source. Contrast
"a duplicate order is created", which is `Expected` negated and gives the mutator nothing
to apply.
