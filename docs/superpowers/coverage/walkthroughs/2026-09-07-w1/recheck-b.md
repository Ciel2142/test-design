# W-1 recheck of repaired-b.md — revision 1, mode `recheck`

Agent type: `general-purpose` (session model, no pin). Inputs: input-spec.md, repaired-b.md,
review-b.md (previous findings). Final message verbatim (angle brackets restored where the
harness transport HTML-escaped them):

```
MERGE SC-01 SC-16 SC-17
  OPEN — SC-17 is retired, but SC-01 (ValidateQuantity(5)) still stands beside SC-16 (ValidateQuantity(1)); a guard at orders.go:6 that rejects positives fails SC-16 as well, and the `<= 1` off-by-one fails only SC-16, so SC-01 catches nothing SC-16 misses and is the duplicate the merge was to remove
REWRITE SC-18
  OPEN — the row was retired into SC-01 instead of rewritten; no surviving row accepts a quantity above 5, so a ceiling added at orders.go:6 (tabled once at fixtures/go-project/docs/superpowers/coverage/2026-08-30-fixture-coverage.md:29) survives the table and R-1.1's "or more" is unguarded
MERGE SC-02 SC-19 SC-20
  RESOLVED — SC-19 and SC-20 retired into SC-02, which carries the message
REWRITE SC-21
  RESOLVED — the mutation now names `== 0` at the guard, the defect only -1 catches
MERGE SC-03 SC-22
  OPEN — SC-22 is retired, but the surviving row still carries Reserve(5); a `<= 1` at orders.go:13 passes SC-03, SC-04 and SC-47 alike, so a one-unit reservation refused in production goes unnoticed — make Reserve(1) the value
MERGE SC-04 SC-23 SC-24
  RESOLVED — both retired into SC-04, which carries the message
ADD Reserve(-1) is rejected with the positive-quantity message
  RESOLVED — SC-47 asserts Reserve(-1) errors with the message and names `== 0` as the mutation
REWRITE SC-29
  OPEN — SC-29 was retired into SC-26 rather than moved to the middle position; no batch row now carries a non-positive quantity anywhere but first or last, and none carries a negative, so R-2.4's middle case is silent and a `== 0` guard at orders.go:21 passes every surviving batch row ([0,3,4], [2,3,0], [2,3,4], [])
REWRITE SC-30
  OPEN — unchanged: still integration for two goroutines against one in-process value, and Reserve(qty int) at orders.go:12-17 still holds no stock, so "one unit available" and "the reserved total ends at one" have no public way to be set or read
MERGE SC-05 SC-31 SC-32
  OPEN — SC-32 is retired, but SC-05 (ValidateAmount(100)) still stands beside SC-31 (ValidateAmount(1)); every guard defect at payments.go:6 that fails SC-05 fails SC-31, and the raised minimum fails only SC-31, so SC-05 is the duplicate
MERGE SC-06 SC-33 SC-34
  RESOLVED — both retired into SC-06, which carries the message
REWRITE SC-35
  RESOLVED — the mutation now names `== 0` at payments.go:6
MERGE SC-08 SC-38
  RESOLVED — SC-38 retired into SC-08
MERGE SC-11 SC-41
  RESOLVED — SC-41 retired into SC-11
RISK SC-12 med→high
  OPEN — SC-12 is still med; its own mutation (seen set keyed by amount) refuses a distinct second payment of the same amount as a duplicate, which is exactly the dropped-payment consequence the new rationale gives for SC-43 high
REWRITE SC-42
  OPEN — still integration; two callers against one in-process instance is a unit test with two goroutines, as the amended live reason now says in so many words
MERGE SC-14 SC-44
  OPEN — SC-44 is retired and the Expected now reads against the schedule, but the mutation still says "a base other than the configured one"; R-5.1 defines a schedule lookup, not a formula, so there is no base to change — name the lookup defect (attempt n reads slot n-1 or n+1)
ADD a schedule value larger than the configured maximum is clamped to the maximum
  RESOLVED — SC-48 asserts an in-schedule value above the maximum is observed as the maximum, with the cap narrowed to past-the-end as the mutation
REWRITE SC-46
  OPEN — still integration for the same in-process reason; "each observed delay still matches the schedule" still does not say which slot the second loop's first wait takes; and the mutation (a shared counter without synchronisation) loses increments and so counts fewer attempts, not the doubled count the Expected guards against — a counter per loop is the defect that doubles it
ESCALATE SC-46 high carries no impact rationale
  OPEN — the draft added a rationale for SC-43 only; SC-46 stays high with none, and whether doubled attempts can settle a charge outside R-4.2's dedupe is still unstated
LIVE not-applicable reason contradicts the idempotency and reservation rows
  RESOLVED — the no-state clause is gone; the decision now rests on in-process execution and the absence of an endpoint, which nothing in the table contradicts
ESCALATE orders.go:6 and payments.go:6 accept zero today (`< 0`)
  OPEN — the tree is unchanged (`qty < 0` at orders.go:6, `cents < 0` at payments.go:6), SC-02 and SC-06 are still red on it, and their mutation is still the code as it stands; the draft does not say whether the code or the requirement is wrong
ESCALATE R-2.6, R-4 and R-5 have no implementation under fixtures/go-project
  OPEN — a grep of fixtures/go-project for charge, idempot, retry, backoff, stock, mutex or sync still returns nothing; SC-30, SC-11, SC-12, SC-42, SC-43, SC-14, SC-15, SC-45, SC-46 and SC-48 (ten rows, all five high among them) still name functions and state with no file:line and mutations with no target
ESCALATE draft IDs SC-01–SC-08 are already bound in the tree to different behaviors
  OPEN — the Retired section lists this draft's own merges and nothing else; orders_test.go:19-22 still marks SC-03 on a skipped test, orders_test.go:45-51 still leaves SC-04 orphaned over ValidateQuantity(7), payments_test.go:31 still credits SC-07/SC-01/SC-05 to ValidateCurrency("USD"), and 2026-08-30-fixture-coverage.md:26-40 still defines the same IDs differently; verification on this table credits SC-07 (EUR) to a USD assertion and SC-03 (Reserve(5)) to a skip

REWRITE SC-45
  "the cap is applied before the schedule value is computed" cannot be applied without guessing what is being capped; the past-the-end defect is that the last schedule value is returned instead of the maximum — name that as the mutation
COUNTS scenarios=31 high=5 journeys=0
```
