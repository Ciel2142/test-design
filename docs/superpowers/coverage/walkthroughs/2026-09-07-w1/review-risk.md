# W-1 review of risk-contrast.md — revision 1, mode `review`

Agent type: `general-purpose` (session model, no pin). Final message verbatim (angle brackets
restored where the harness transport HTML-escaped them):

```
ESCALATE SC-01 cannot be mutated as written: payments.go:6 already compares with `<`, so ValidateAmount(0) returns nil against R-3.2
  if the code is authoritative, zero-cent payments are accepted and recorded; if R-3.2 is, the row's test fails at baseline (payments_test.go:12-16 already does) and the mutation is a no-op until the guard becomes `<=`. The same conflict sits at orders.go:6 against R-1.2.

RISK SC-02 (amended) med→high
  a dropped tenant predicate hands tenant A the fields of tenant B's order — a security-boundary violation is always high; the original's `high` stands, and the downgrade would also pull the row out of the mutation-verified set.

REWRITE SC-03
  R-2.4 owes a returned error carrying the R-2.2 message when the non-positive quantity sits first, in the middle or last; nothing in the tree holds a reserved total (orders.go:19-26 is a pure loop), so "reserved total is what it was before" is unobservable and "rollback omitted" has no site. Unit level; mutation: the loop at orders.go:20 stops after the first element.

REWRITE SC-05
  the two-second bound cannot be settled by reading and passes under the mutation on any store smaller than one page, so "per-page limit removed" survives; make the deterministic observable the returned row count never exceeding the page size, and keep the timing bound only with a dataset named large enough to breach it.

ESCALATE SC-02, SC-04 and SC-05 sit outside the approved requirements and outside the tree
  input-spec.md:3-8 scopes the product to the `orders` and `payments` packages and rules transport and storage out; no tenant, order store, row lock or search exists in orders.go or payments.go, so all three mutations name code with no site. Kept, they bind to nothing; dropped, a cross-tenant read, a lost order update and an unbounded search ship unguarded when that code lands. The Subject line and the requirements must agree before approval.

ADD order quantity boundary: 1 accepted, 0 rejected with "quantity must be greater than zero" (R-1.1, R-1.2; orders.go:5-10)
  no row touches section 1; a zero-quantity order that passes is a line item for nothing. orders.go:6 reads `qty < 0`, so this row fails at baseline (orders_test.go:13-17 already does) until the guard is `<=`.

ADD reservation boundary: Reserve(1) accepted, Reserve(0) rejected with "reservation quantity must be positive" (R-2.1, R-2.2; orders.go:12-17)
  TestReserveGuard at orders_test.go:54-61 asserts an error for 0 but never the message or the accepted lower bound; a `<=`→`<` flip at orders.go:13 lets a zero reservation succeed.

ADD batch accept paths: an empty batch and an all-positive batch both return nil (R-2.3, R-2.5; orders.go:19-26)
  no row asserts the accept side; an added empty-batch refusal or an error on the fall-through at orders.go:25 blocks every valid batch unseen.

ADD concurrent reservations never oversubscribe stock and the unsatisfied caller is told (R-2.6)
  no row, and no stock state in orders.go:12-17 to bind one to; when the stock path lands, a check-then-reserve without atomicity lets two callers each take the last unit — harmful shared-state failure, high. Needs a race harness; reading cannot settle it.

ADD payment amount of one cent is accepted (R-3.1; payments.go:5-10)
  SC-01 covers only the rejected side; a guard moved to `cents <= 1` refuses every one-cent payment and neither SC-01 nor payments_test.go:7-11 (uses 100) notices.

ADD currency accept set is exactly USD, EUR, GBP (R-3.3; payments.go:13-15)
  payments_test.go:21-25 and :46-50 assert USD and GBP only; EUR dropped from the switch refuses every euro payment unseen.

ADD currency rejection: "usd", "" and an unknown code are refused and the message quotes the refused code (R-3.4; payments.go:16-17)
  no test in the tree asserts any rejection (payments_test.go:26-28 is skipped); a case-folded compare accepts "usd" and passes an ambiguous code downstream. Mutation: the switch compares `strings.ToUpper(code)`.

ADD duplicate idempotency key for one account is refused and the account holds exactly one charge for that key (R-4.2)
  no row and no charge code anywhere in payments.go; a missing dedupe double-charges the customer — irreversible external side effect, high.

ADD distinct keys on one account each record a charge (R-4.3)
  distinct defect from R-4.2: a dedupe keyed on account alone refuses every second charge a customer makes.

ADD one key used by two accounts records one charge for each (R-4.4)
  a dedupe keyed on the key alone either refuses the second account's payment or attributes it to the first account's record — cross-account shared-state failure, high.

ADD two concurrent charges under one key settle exactly once: one refused, one recorded (R-4.5)
  a check-then-record without atomicity lets both callers through — double charge, high. Needs a race harness at integration; reading cannot settle it.

ADD backoff delay for attempt n is the nth schedule value and never exceeds the configured maximum past the schedule end (R-5.1, R-5.2)
  no row and no retry code in the tree; an off-by-one index or a dropped cap hammers the upstream or waits unboundedly.

ADD retrying stops at the attempt limit and the caller receives the final attempt's error (R-5.3)
  a limit compared with `<=` retries once too often; a swallowed last error tells the caller nothing about why the charge failed.

ADD concurrent retry loops for one charge share one attempt count and each observes the scheduled delays (R-5.4)
  a per-loop counter retries a charge limit×loops times; distinct defect from R-5.3.

LIVE no `## Live decision` section is present
  every row is unit or integration and input-spec.md:6-8 rules the deployed transport out, so `not-applicable` with that reason is the consistent decision — but the Subject line's "as it is planned to be deployed" and SC-05's pooled-connection rationale describe a deployed service and contradict it; one of the two must go.

ESCALATE the document carries two risk assignments over one ID set and names neither as governing
  a verifier reading it cannot tell whether SC-02 is high (original) or med (amended), and the amended copy is the later one; the counts below read the amended table as the standing draft.

COUNTS scenarios=5 high=3 journeys=0
```
