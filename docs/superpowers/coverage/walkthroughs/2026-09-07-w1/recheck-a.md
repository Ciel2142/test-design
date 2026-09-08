# W-1 recheck of repaired-a.md — revision 1, mode `recheck`

Agent type: `general-purpose` (session model, no pin). Inputs: input-spec.md, repaired-a.md,
review-a.md (previous findings). Final message verbatim (angle brackets restored where the
harness transport HTML-escaped them):

```
DROP SC-09
  RESOLVED: retired in the Retired section with the missing observable result recorded; no surviving row cites it

DROP SC-10
  RESOLVED: retired as merged into SC-06, which keeps the input, the mutation and the message

REWRITE SC-07
  OPEN: the row still asserts ValidateCurrency("EUR") alone with "EUR is dropped" as its mutation; USD or GBP removed from the case list at payments.go:14 survives, and R-3.3 names all three

DROP SC-13
  RESOLVED: retired; no requirement or code names a loyalty tier

REWRITE SC-14
  RESOLVED: the delay is now observed through an injected sleeper and the mutation is the index shift (attempt n reads the value for n+1), which has a site once a schedule lookup exists

REWRITE SC-15
  RESOLVED: the row now fixes the count (exactly three invocations under a limit of 3) and the returned error (the third invocation's), both of which the `<`-for-`<=` mutation changes

ADD a batch whose quantities are all positive is accepted
  RESOLVED: SC-16 asserts ReserveBatch over 2, 3 and 4 returns nil with the flipped comparison at orders.go:21 as its mutation

ADD a batch with a non-positive quantity in the middle position is rejected with the R-2.2 message
  RESOLVED: SC-17 asserts ReserveBatch over 2, -1 and 4 returns the orders.go:22 message; the loop-stops-after-first-element mutation returns nil there and is killed

ADD an empty batch is accepted
  RESOLVED: SC-18 asserts ReserveBatch over an empty list returns nil with an inserted length guard ahead of orders.go:20 as its mutation

LIVE not-applicable reason claims the module "keeps no state between calls"
  RESOLVED: the clause is gone; the reason now rests on "deploys nothing and serves no endpoint", which no row and no requirement (input-spec.md lines 6-7) contradicts

ESCALATE ValidateQuantity and ValidateAmount accept zero while R-1.2 and R-3.2 reject it
  OPEN: orders.go:6 and payments.go:6 still guard with `< 0`, SC-02 and SC-06 still expect zero rejected, their `<`-for-`<=` mutation is still the code as it stands, and the baseline is still PENDING; the draft records none of this. Settled for the code, zero-quantity orders and zero-cent payments pass validation

ESCALATE SC-06 risk may be high rather than med
  OPEN: SC-06 stays med and the draft adds nothing about what consumes a passed amount; if a zero-cent amount lets a charge of nothing settle an order, goods leave with no money captured

ESCALATE R-4 and R-5 have no implementation to verify or mutate
  OPEN: payments.go:5-19 still exports only ValidateAmount and ValidateCurrency, and nothing under fixtures/go-project names a charge, an account, a key store, a sleeper or a retry loop; SC-11, SC-12 and the rewritten SC-14 and SC-15 name no public function and their mutations have no site. Settled as "the code is complete", double charges and unbounded retries go unguarded

ESCALATE the fixture's `covers:` bindings do not match this draft's IDs
  OPEN: every binding cited stands (orders_test.go:19-22, 45-51, 53; payments_test.go:19-29, 45; orders_test.go:38), and the amendment widens the gap: retired SC-09 and SC-10 are still bound at orders_test.go:24 and :53, SC-08 is credited by ValidateQuantity(3) and ValidateAmount(300) (orders_test.go:31-36, payments_test.go:38-43) which never call ValidateCurrency, and SC-16, SC-17 and SC-18 have no test at all. Verification against these credits rows with evidence that asserts none of their results

REWRITE SC-12
  the mutation keys the seen set by amount, but the row never fixes the two charges' amounts; with different amounts the mutant records both and the row passes. Give both charges one amount, or mutate "the lookup ignores the key and refuses any second charge on the account"

ESCALATE final-command names draft-a.md while the table under review is repaired-a.md
  settled by running verification on the pointer as written, the unrepaired table is what gets verified: SC-09, SC-10 and SC-13 are checked and SC-16, SC-17 and SC-18 are not

COUNTS scenarios=15 high=1 journeys=0
```
