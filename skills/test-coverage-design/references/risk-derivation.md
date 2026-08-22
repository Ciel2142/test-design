# Risk derivation

`Risk` is **derived by rule, never judged**. It takes exactly `low`, `med`, or `high`.

## The rule

- Rows on axes **3 (failure paths), 4 (idempotency/retry), 5 (concurrency), 6 (auth)** are `high`.
- Any row whose mutation names **data loss, data corruption, or exposure** is `high`.
- Everything else is `med`, or `low` where the failure is purely cosmetic.

## Why it is not a judgment call

`high` is the only load-bearing value: it obliges the row to carry layer 4 mutation
evidence at verification. An agent that both assigns risk and later bears its cost has a
free, untraceable way to switch that gate off — mark every row `med` and layer 4 never
fires. A human reviewing a twenty-row table audits scenarios, not risk labels, so nobody
notices. Under context pressure this is not even deliberate; it is the path of least
friction.

Deriving from the axis removes the discretion that makes that possible.

## Argue-down protocol

The agent may propose lowering a row from `high` to `med`. It may never raise its own
burden, and never lower one silently.

One proposal may cover multiple rows that share **one concrete misfire reason**; a
single-row proposal is the degenerate case of this. The reason must apply to each listed
row individually — bundling unrelated rows under a vague umbrella ("these all seem
low-stakes") is not a shared reason. One explicit user approval covers the whole batch.

1. State the row(s), the rule that made each `high`, and the concrete reason the rule
   misfires for all of them.
2. Get explicit user approval.
3. Record it in the coverage doc under the behavior group, listing every covered SC-ID:

```
Risk argued down: SC-07, SC-09 high→med — axis 4 rows, but both endpoints are pure reads
with no write path, so a duplicate call has no side effect to be idempotent about. Approved
by user.
```

An argue-down with no recorded approval line is invalid; verification treats the row as
`high` — including a row omitted from a batch's approval line.
