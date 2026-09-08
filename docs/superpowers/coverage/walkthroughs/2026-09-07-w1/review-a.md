# W-1 review of draft-a.md — revision 1, mode `review`

Agent type: `general-purpose` (session model, no pin). Final message verbatim (angle brackets
restored where the harness transport HTML-escaped them):

```
DROP SC-09
  no requirement — input-spec.md lines 6-7 put the shape of the code out of scope — and no
  observable result: a caller cannot tell a switch from a linear scan over three strings, and a
  three-case Go string switch compiles to comparisons, not a jump table, so the Expected is false
  on unmutated payments.go:13-18

DROP SC-10
  same input (ValidateAmount(0)), same mutation and a weaker Expected than SC-06; one defect at
  payments.go:6 kills both, and SC-06 already carries the message

REWRITE SC-07
  R-3.3 accepts USD, EUR and GBP; asserting EUR alone lets USD or GBP dropped from the case list at
  payments.go:14 survive, and the scope note does not hold those back — assert all three return
  nil, mutation: one code removed from the case list

DROP SC-13
  no requirement mentions a loyalty tier and payments.go has no tier code; a row outside the
  approved requirements whose consequence nobody approved

REWRITE SC-14
  R-5.1 defines the delay as the nth value of a configured schedule, not a formula, so "a base
  other than the configured one" has no site; mutate the index instead (attempt n reads the
  n+1th value, or every attempt reads the first) and say how the delay is observed (an injected
  sleeper records it)

REWRITE SC-15
  "no further attempt is made" also holds when the loop stops one attempt early, so the
  `<`-for-`<=` mutation survives the row as written; state the count — with limit 3 the operation
  is invoked exactly three times and the third invocation's error is returned

ADD a batch whose quantities are all positive is accepted
  R-2.3; ReserveBatch at orders.go:19-26 has its own guard at line 21, separate from Reserve's
  line 13, so SC-03 does not cover it. Mutation: the comparison at orders.go:21 is flipped to `>`

ADD a batch with a non-positive quantity in the middle position is rejected with the R-2.2 message
  R-2.4; a middle-position value kills both "only the first element is checked" and "only the
  last element is checked" with one row. Mutation: the range loop at orders.go:20 breaks after
  its first iteration

ADD an empty batch is accepted
  R-2.5; ReserveBatch(nil) returns nil. Mutation: a `len(qtys) == 0` guard returning the R-2.2
  error is inserted before the loop at orders.go:20

LIVE not-applicable reason claims the module "keeps no state between calls"
  SC-11 and SC-12 assert the account still holds one or two charges after the calls, which is
  state between calls; the decision stands on "deploys nothing, serves no endpoint" alone — drop
  the contradicting clause

ESCALATE ValidateQuantity and ValidateAmount accept zero while R-1.2 and R-3.2 reject it
  orders.go:6 and payments.go:6 guard with `< 0`, so SC-02 and SC-06 fail on unmutated code and
  their `<`-for-`<=` mutation is a no-op there (orders_test.go:13 and payments_test.go:12 already
  assert the rejection and would fail; only a run settles the pending baseline). Settled for the
  code, zero-quantity orders and zero-cent payments pass validation

ESCALATE SC-06 risk may be high rather than med
  if a zero-cent amount passing validation lets a charge of nothing settle an order, goods leave
  with no money captured — an irreversible loss; nothing in the repository shows the consumer,
  so I cannot settle it

ESCALATE R-4 and R-5 have no implementation to verify or mutate
  payments.go:1-19 exports only ValidateAmount and ValidateCurrency; there is no charge, account,
  key store or retry loop, so SC-11, SC-12, SC-14 and SC-15 name no public function and their
  mutations have no site. Settled as "the code is complete", double charges and unbounded retries
  go unguarded

ESCALATE the fixture's `covers:` bindings do not match this draft's IDs
  orders_test.go:19-22 binds SC-03 to a skipped test, orders_test.go:45-51 binds SC-04 to
  ValidateQuantity(7), orders_test.go:53 binds SC-10 to the Reserve guard, payments_test.go:19-29
  binds SC-06 to USD-accepted plus a JPY skip, and SC-77/SC-90 name no row. Verification against
  these credits SC-03, SC-04 and SC-06 with evidence that asserts none of their results

COUNTS scenarios=15 high=1 journeys=0
```
