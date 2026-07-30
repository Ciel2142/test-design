# Fixtures

`sample-project/` exists so the verification layers can be tested without dogfooding.
Every oddity in it is deliberate. Do not "fix" them.

| Test | Tag | Shape | Which layer it proves |
|---|---|---|---|
| `rejects zero quantity` | `SC-01` | passes, asserts thrown error **and** that no row persisted | layer 4: mutating `qty <= 0` to `qty < 0` must make it fail |
| `rolls back on mid-write failure` | `SC-02` | `it.skip` | layer 1: a tag above a skipped test must yield `gap`, not coverage |
| `creates an order` | `SC-03` | passes, asserts only `toHaveBeenCalledOnce` on a spy | layer 2: must be verdicted `VACUOUS` |
| `computes total` | none | passes | layer 1: an untagged test is not a finding by itself; it exists to prove the tag grep does not over-match |

`src/server.ts` provides the live surface: `GET /health`, `POST /orders`.
With `FAIL_DB=1` the write path throws and the endpoint answers `503` with `rows: 0` —
that is the dep-down walkthrough. Without it, `POST /orders {"qty":3}` answers `201`.

The mutation target for `SC-01` is `src/orders.ts` line with `if (qty <= 0)`.
