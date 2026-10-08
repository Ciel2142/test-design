# Coverage: order creation

Spec: docs/superpowers/specs/2026-07-30-order-creation-design.md
Baseline: e4a91f2 · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-07-30-order-creation-baseline.json
- baseline-sha256: 3f5b3c1a9e7d2b8f4c6a1d9e5b7f3c2a8d6e4b1f9c7a5e3d1b9f7c5a3e1d9b7f
- live-profile: .test-design/live-profiles/order-creation-local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-07-30-order-creation-coverage.md"]

## Baseline failures

None — the suite was green at the baseline commit.

## Live decision

- decision: required
- reason: the spec requires the deployed service to stay answerable while its store is down

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-05 | `POST /orders {"qty":3}` against the deployed revision with `FAIL_DB=1` answers 503, the body reports `rows: 0`, and the request message is still unacked in the queue |

## Review record

- Reviewer instructions: `agents/scenario-reviewer.md` from plugin version `1.2.0`,
  `git hash-object` `b7f0c1d4e8a26f3915c4d0a7be2f81c630d95a4e`.
- `MERGE SC-03 SC-04` — one defect, the uniqueness check outside the transaction, kills both.
  **Applied**: SC-04 retired into SC-03.
- `RISK SC-01 med→high` — a wrong bound admits a zero-quantity order. **Rejected**: the
  request is refused before any write, so nothing is lost, corrupted, or exposed and no
  boundary is crossed; `med` stands.
- `LIVE J-1 expected result is not observable on the deployed revision` — "the store call
  failed" is internal state. **Applied**: J-1 now names the status, the body, and the unacked
  message.
- Recheck: `MERGE` and `LIVE` came back `RESOLVED`; `RISK SC-01 med→high` came back `OPEN`,
  because SC-01 is still `med` and the recheck never sees why it was rejected. No `NEW GAP`.
- Carried to the user at approval alongside
  `Cost: 4 surviving scenarios, 3 high-risk obligations, 1 live journeys`: leaving SC-01
  `med` means the lower-bound guard carries no layer 4 mutation evidence, so nothing but
  SC-01's own test stands between a loosened guard and a zero-quantity order. The user kept
  `med`.

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

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | quantity at the lower bound is rejected | throws "qty must be > 0", no row persisted | unit | guard uses `<` instead of `<=` | med |
| SC-02 | write rolls back when the commit hook fails | 0 rows persisted, INV-1 holds | integration | rollback call omitted on the error path | high |
| SC-03 | two writers with one idempotency key | 1 row, second call answers 200 | integration | uniqueness check runs outside the transaction (TOCTOU) | high |
| SC-05 | dependency down answers 503, not 500 | 503, body reports `rows: 0`, message not acked | smoke | error mapped to the generic handler instead of the unavailable branch | high |

harness SC-02: in-process store, write hook throws on invocation
harness SC-03: 2 parallel clients, real store, no mocks, 50 iterations fail-if-any
harness SC-05: service started with `FAIL_DB=1`, `POST /orders {"qty":3}`

Risk rationale: SC-05 high — a 500 hides an outage as a bug, so on-call chases the service
while the dependency stays down and recovery is delayed for every caller.

### Notes on this example

IDs were allocated through `next-id -repo . -area ""` before the sweep, which returned
`next: 1` for a first coverage document — the rows below are numbered from that starting
point, not chosen by hand.

`SC-01` is `med`: a rejected order at the boundary loses nothing, corrupts nothing, and
crosses no boundary. `SC-02` and `SC-03` are `high` by consequence — a missing rollback leaves
a partially written order, and a uniqueness check outside the transaction lets two writers
create a duplicate one. `SC-05`'s consequence is outside the four always-`high` ones, so it
carries the `Risk rationale:` line under the table that the scenario reviewer checks. All
three `high` rows require layer 4 mutation evidence.

`SC-03` is `integration`, not `unit`, because two mocked concurrent calls execute
sequentially in one thread and would pass whether or not the check sits inside the
transaction. Its harness names an iteration count and fail-on-any, because a single green
run of a race test proves nothing — the racy interleaving might simply not occur on that
pass, and only a harness that fails on any iteration's failure gives the row's verdict any
weight.

`SC-05` is `smoke`, so it is a live obligation: the `## Live decision` is `required`, journey
`J-1` names it, and the `## Verification` block carries the `live-profile` layer 3 dispatches
against. A `not-applicable` decision could not stand next to this row.

Every `Verification mutation` names something a person could type into the source.
Contrast "a duplicate order is created", which is `Expected` negated and gives the mutator
nothing to apply.
