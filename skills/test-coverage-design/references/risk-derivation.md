# Risk derivation

`Risk` follows the **consequence of the failure, never the subject matter of the code**. It
takes exactly `low`, `med`, or `high`.

## The rule

- Always `high`: a **security-boundary violation**, **data loss or corruption**, an
  **irreversible external side effect**, a **harmful shared-state failure**. A row whose
  failure does one of these is `high` whichever axis produced it.
- Any other `high` carries an impact rationale of its own, on a line under the behavior
  group's table, naming what the failure does to someone:

  ```
  Risk rationale: SC-nn high — <impact>
  ```

  The scenario reviewer checks that the line exists and that it names a consequence rather
  than a topic. A `high` without one comes back as a `RISK` finding.
- Membership in an axis, a module, or a subsystem never sets `high` by itself. Axes 3, 4, 5
  and 6 are where the four consequences usually live, which makes them a place to look, not
  a verdict.
- Risk whose consequence is unknown is not downgraded to save the verification it would cost.
  Put the doubt to the user rather than resolving it downward.
- Everything else is `med`, or `low` where the failure is purely cosmetic.

## Why it is not a judgment call

`high` is the only load-bearing value: it obliges the row to carry layer 4 mutation
evidence at verification. An agent that both assigns risk and later bears its cost has a
free, untraceable way to switch that gate off — mark every row `med` and layer 4 never
fires. A human reviewing a twenty-row table audits scenarios, not risk labels, so nobody
notices. Under context pressure this is not even deliberate; it is the path of least
friction.

Deriving from the consequence, and writing that consequence down where a reviewer reads it,
removes the discretion that makes this possible.

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
3. Record it in the coverage doc under the behavior group, listing every covered SC-ID.
   When the batch spans behavior groups, record the same line under each group it covers:

```
Risk argued down: SC-07, SC-09 high→med — both endpoints are pure reads with no write path,
so a duplicate call corrupts nothing, loses nothing, and crosses no boundary. Approved by
user.
```

An argue-down with no recorded approval line is invalid; verification treats the row as
`high` — including a row omitted from a batch's approval line.

Once the user has approved the source, a downgrade is an **amendment**: state the row, the
consequence rule it was `high` under, and why that consequence does not occur; get the user's
approval again; record it. The agent that later verifies the row may never lower its
obligations — it measures them.
