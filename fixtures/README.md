# Fixtures

`sample-project/` exists so the verification layers can be tested without dogfooding.
Every oddity in it is deliberate. Do not "fix" them.

| Test | File | Tag | Shape | Which layer it proves |
|---|---|---|---|---|
| `rejects zero quantity` | `orders.test.ts` | `SC-01` | passes, asserts thrown error **and** that no row persisted | layer 4: mutating `qty <= 0` to `qty < 0` must make it fail |
| `rolls back on mid-write failure` | `orders.test.ts` | `SC-02` | `it.skip` | layer 1: a tag above a skipped test must yield `gap`, not coverage |
| `creates an order` | `orders.test.ts` | `SC-03` | passes, asserts only `toHaveBeenCalledOnce` on a spy | layer 2: must be verdicted `VACUOUS` |
| `computes total` | `orders.test.ts` | none | passes | layer 1: an untagged test is not a finding by itself; it exists to prove the tag grep does not over-match |
| `rolls back on mid-write failure` | `alerts.test.ts` | none | **passes**, same name as SC-02's | layer 1: the join must use the tag's file, not its name alone. `alerts` sorts first, so a name-only join takes this passing row and reports SC-02 covered |

The duplicate name in `alerts.test.ts` is the sharpest row in the table: it is the one
oddity that turns a correct-looking green report into a wrong verdict. Verified against a
real `vitest --reporter=json` run — a name-only join returns `passed` for `SC-02`, a
file-scoped join returns `skipped`, and only the second is true.

`src/server.ts` provides the live surface: `GET /health`, `POST /orders`.
With `FAIL_DB=1` the write path throws and the endpoint answers `503` with `rows: 0` —
that is the dep-down walkthrough. Without it, `POST /orders {"qty":3}` answers `201`.

The mutation target for `SC-01` is `src/orders.ts` line with `if (qty <= 0)`.

## `live-project/`

Layer 3's fixture: no source code, only the three documents `prepare-live` and
`execute-live` read. `SC-11` passes, `SC-12` is bound to a deliberately unmet expectation
so the walkthrough yields one `LIVE-FAIL`, `SC-13` is the row the trace evidence marks as
a layer-1 gap, and `SC-14` is unit-level so it must never reach layer 3 at all.

Two oddities are load-bearing:

- the plan's `printenv LIVE_FIXTURE_CANARY` step expects **exit 1**. The canary is
  exported by the test and left off the profile's `envAllowlist`; an unsanitized child
  environment makes `printenv` succeed and turns `SC-11` into `LIVE-FAIL`.
- the profile's last cleanup command is a `find` whose `cleanupEvidence` predicate
  requires empty output, so `cleanupOK` is a real proof that the run-ID-named scratch
  resource is gone rather than a claim that `rm` was invoked.
