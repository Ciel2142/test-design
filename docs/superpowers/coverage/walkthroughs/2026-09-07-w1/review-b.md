# W-1 review of draft-b.md — revision 1, mode `review`

Agent type: `general-purpose` (session model, no pin). Final message verbatim (angle brackets
restored where the harness transport HTML-escaped them):

```
MERGE SC-01 SC-16 SC-17
  5, 1 and 7 all fail only when orders.go:6 rejects positive quantities; keep the boundary value 1, which also catches the off-by-one (`<= 1`) that 5 and 7 miss
REWRITE SC-18
  its mutation is SC-01's in other words, so as written it is a duplicate; it earns a row only as the no-ceiling check R-1.1 implies (a ceiling was once tabled: fixtures/go-project/docs/superpowers/coverage/2026-08-30-fixture-coverage.md:29) — mutation: an upper bound is added at orders.go:6
MERGE SC-02 SC-19 SC-20
  same input, same guard at orders.go:6, same mutation; SC-02 carries the message; the "checkout path" of SC-20 names no code in the module
REWRITE SC-21
  "inverted" turns `<=` into `>`, which SC-02 already kills; the defect only this row catches is `qty == 0` at orders.go:6 — name that as the mutation
MERGE SC-03 SC-22
  5 and 250 fail only when orders.go:13 rejects positives; one row carrying the boundary value 1 covers both and catches the off-by-one that neither does
MERGE SC-04 SC-23 SC-24
  all assert Reserve(0) errors on the same `<=` at orders.go:13; SC-04 carries the message; the "order service" of SC-24 does not exist in the module
ADD Reserve(-1) is rejected with the positive-quantity message
  R-2.2 "zero or fewer"; a guard of `qty == 0` at orders.go:13 passes -1 and every reservation rejection row uses zero
REWRITE SC-29
  as written ([0, 5]) it dies with SC-26 on the same per-element `<=`; make it the middle-position row R-2.4 names, [2, -1, 4], which also catches a `== 0` guard at orders.go:21 that every zero-carrying batch row misses
REWRITE SC-30
  integration is not earned — two goroutines against one in-process value is a unit test, as the live reason itself says; and Reserve(qty int) at orders.go:12-17 holds no stock, so "one unit available" has no public way to be set or read
MERGE SC-05 SC-31 SC-32
  100, 1 and 2599 fail only when payments.go:6 rejects positive cents; SC-31 at the boundary also catches the off-by-one the others miss
MERGE SC-06 SC-33 SC-34
  all assert ValidateAmount(0) errors on the same `<`/`<=` at payments.go:6; SC-06 carries the message; the "refund path" of SC-34 names no code in the module
REWRITE SC-35
  same defect as SC-21: "inverted" is already killed by SC-06; name `cents == 0` at payments.go:6 as the mutation
MERGE SC-08 SC-38
  JPY and XXX both fall to the same default branch at payments.go:16-17 and carry the same mutation; one row covers them
MERGE SC-11 SC-41
  identical Expected and identical mutation, word for word
RISK SC-12 med→high
  keying the seen set by amount refuses a distinct second payment as a duplicate, so a real payment is dropped and the payer told it already exists — the consequence the draft itself rates high for SC-43
REWRITE SC-42
  integration is not earned — two callers against one in-process instance is a unit test with two goroutines, by the draft's own live reasoning
MERGE SC-14 SC-44
  SC-14 already asserts attempt 2's delay under the same mutation; also the mutation speaks of a formula base while R-5.1 defines a schedule — name the lookup defect (wrong index) instead
ADD a schedule value larger than the configured maximum is clamped to the maximum
  R-5.2 first sentence; SC-45 only exercises an attempt past the schedule's end, so an unclamped in-schedule value survives it
REWRITE SC-46
  integration is not earned for the same in-process reason; and "each observed delay still matches the schedule" is ambiguous once attempts count once per charge — say which schedule slot the second loop's first wait takes
ESCALATE SC-46 high carries no impact rationale
  doubled attempts are a double charge only if an attempt can settle outside R-4.2's dedupe; if every extra attempt is idempotent-refused the consequence is provider load and a later failure, which is med — say which
LIVE not-applicable reason contradicts the idempotency and reservation rows
  "keeps no state between calls" is false for SC-11, SC-12, SC-42, SC-43 ("the account still holds…") and SC-30 ("the reserved total ends at one"); the decision stands on the in-process/no-endpoint grounds alone — strike the no-state clause
ESCALATE orders.go:6 and payments.go:6 accept zero today (`< 0`), contradicting R-1.2 and R-3.2
  SC-02 and SC-06 are red on the current tree and their mutation (`<` for `<=`) is the code as it stands, so there is nothing to apply; if the code is ruled correct, zero-unit orders and zero-cent payments pass validation
ESCALATE R-2.6, R-4 and R-5 have no implementation under fixtures/go-project
  no charge, idempotency, retry, backoff, stock or lock code exists in the module (grep: no matches), so SC-30, SC-11, SC-12, SC-41–SC-46 name functions and state with no file:line and their mutations have no target; kept as written they verify nothing, dropped they leave oversubscription and double charging unguarded
ESCALATE draft IDs SC-01–SC-08 are already bound in the tree to different behaviors, and Retired says nothing was retired
  orders_test.go:19-22 marks SC-03 on a skipped ValidateQuantity test, orders_test.go:45-51 leaves SC-04 orphaned above a ValidateQuantity(7) test, payments_test.go:31 credits SC-07/SC-01/SC-05 to one ValidateCurrency("USD") assertion, and SC-08 is claimed by ValidateQuantity(3) (orders_test.go:31) and ValidateAmount(300) (payments_test.go:38) while fixtures/go-project/docs/superpowers/coverage/2026-08-30-fixture-coverage.md defines the same IDs differently; if the draft's IDs stand, verification credits SC-07 (EUR) to a USD assertion and SC-03 to a skip
COUNTS scenarios=43 high=6 journeys=0
```
