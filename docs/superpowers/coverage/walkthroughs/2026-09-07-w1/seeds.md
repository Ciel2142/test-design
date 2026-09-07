# W-1 seeds: the answer key

Written together with `input-spec.md`, `draft-a.md`, `draft-b.md` and `risk-contrast.md`
and committed before the reviewer's instructions existed, so no seed could be tuned to the
instructions. Nothing here is repeated or hinted at in the inputs themselves.

Finding kinds are the review vocabulary: MERGE, DROP, REWRITE, ADD, RISK, LIVE, ESCALATE.

## Scoring rules for unseeded findings

A reviewer will say more than this key anticipates. Every finding is recorded as exactly
one of five outcomes, and only the first two bear on whether the walkthrough succeeded:

- **seed hit** — the finding cites a seeded defect by row, with a kind this key allows.
- **seed miss** — a seeded defect no finding cites. Reported as such; never repaired after
  the fact.
- **borderline** — a finding on a row this key marks borderline. Either resolution is
  scored correct and neither is a hit or a miss.
- **unseeded, true-extra** — a correct observation that is not about a defect in the table.
- **unseeded, false-positive** — a finding that would merge away, drop or weaken a row this
  key calls good, or that invents a defect the inputs do not contain.

### Findings that are true-extra, never hits and never misses

1. **The shipped code contradicts a spec-faithful row.** `draft-a.md:38` SC-02 and
   `draft-a.md:52` SC-06 assert that `ValidateQuantity(0)` and `ValidateAmount(0)` return
   errors. `input-spec.md` R-1.2 and R-3.2 require exactly that, but the shipped guards at
   `fixtures/go-project/orders/orders.go:6` and `payments/payments.go:6` compare with `< 0`,
   so today both calls return nil. The same holds for `draft-b.md` SC-02, SC-06 and their
   duplicates SC-19, SC-20, SC-33 and SC-34. A reviewer that points this out has found a
   production bug, which is a correct and useful observation about the **subject**. The rows
   are right and the code is wrong; the rows are not defects and must not be counted as
   seeds, as misses, or as false positives.
2. **Rows whose behavior is not implemented.** The charge-idempotency and retry-backoff
   rows in both drafts describe `input-spec.md` sections 4 and 5, which no function in
   `fixtures/go-project` implements. Coverage design is written against the approved spec,
   not against the existing suite, so these rows are legitimate. A finding that the code is
   missing is true-extra; a finding that the rows should therefore be dropped is a
   false-positive.
3. **Harness placeholders.** `PENDING_BASELINE_COMMIT`, `PENDING_BASELINE_SHA256` and the
   `draft-a-baseline.json` / `draft-b-baseline.json` paths that do not exist are properties
   of a pre-approval draft, not of the table. True-extra.
4. **Arguing against a deferral draft A names.** See the draft A section below.

### Reading the hit rate

The four draft A seeds are not equally hard and the rate is interpreted per seed, never as
a single total.

- **A-1** is the mechanically easy one: SC-10 and SC-06 carry a byte-identical
  Verification-mutation cell, so a reviewer that merely groups rows by that column finds it
  without understanding either row. A hit on A-1 alone is weak evidence.
- **A-2**, **A-3** and **A-4** each need the reviewer to hold a second document against the
  table — the spec for A-2 and A-4, the notion of an observable outcome for A-3 — and A-4
  additionally needs it to notice an absence rather than judge a present row. Hits on those
  are the load-bearing ones.

Missing A-1 while catching A-4 is a different and better signal than the reverse, and the
resolution says which seeds were hit rather than how many.

## draft-a.md — exactly four seeded defects, and nothing else

15 rows. `SC-09`, `SC-10` and `SC-13` are the three defective rows; the fourth seed is an
absence, not a row. Every other row is intended to be a good row: an observable expected
result, a mutation that would really break it, a risk that follows the consequence.

| Seed | Row | Kind | What is wrong | Expected finding |
|------|-----|------|---------------|------------------|
| A-1 | SC-10 | redundant | Same verification mutation as `SC-06` (the amount guard's `<` where `<=` is needed) and the same observable outcome (a non-nil error for an amount of 0), reworded as "below the smallest chargeable amount". One mutation cannot be caught twice by two rows that assert the same thing. | MERGE, citing SC-10 into SC-06 |
| A-2 | SC-13 | invented | A lapsed loyalty tier appears nowhere in `input-spec.md` and nowhere in `orders/orders.go` or `payments/payments.go`. There is no payer profile, no tier and no freshness check anywhere in the subject. | DROP |
| A-3 | SC-09 | unobservable expected result | The expected result asserts how the lookup is compiled — a jump table rather than a walk over the supported set. No caller of `ValidateCurrency` can observe that, and the stated mutation (rewriting the switch as a linear scan) changes no observable behaviour, so no test could ever fail on it. | REWRITE (DROP is also defensible: the row has no observable behaviour left once the implementation claim is removed) |
| A-4 | `ReserveBatch` per-element guard | omitted real behavior | `input-spec.md` R-2.3, R-2.4 and R-2.5 require a batch reservation to be accepted when every quantity is positive, rejected when any one is not positive wherever it sits in the batch, and accepted when empty. `fixtures/go-project/orders/orders.go:19-26` implements exactly that loop. Draft A's reservation group carries only `Reserve` rows (SC-03, SC-04) and no `ReserveBatch` row at all. | ADD |

Draft A carries a scope note under `## Scenarios` that defers, by name, the edge values,
the further currency spellings of R-3.4, the delay cap of R-5.2, the account scoping of
R-4.4 and the three concurrency requirements R-2.6, R-4.5 and R-5.4. Those deferrals exist
so that seed A-4 is the table's only unexplained absence. `input-spec.md` R-2.3, R-2.4 and
R-2.5 are not among the deferred rules. A reviewer that argues a named deferral should not
have been deferred — concurrency most plausibly — is making a legitimate scope argument; it
is neither a hit on a seed nor a false positive, and the resolution records it as an
unseeded extra.

Seeded risk defects in draft A: none. Its only `high` is `SC-11` (a repeated charge taking
the money twice is an irreversible external side effect, a named category, so it needs no
rationale line). Every other row is `med` because its failure is an ordinary rejected
input. A reviewer that raises a RISK finding on draft A is producing a false positive.

## draft-b.md — merge, keep or borderline for all 43 rows

Of the 43 rows, 39 are firm: 14 are superficial duplicates that should be merged away and
25 survive. The remaining four — SC-21, SC-35, SC-36 and SC-37 — are **borderline**, and
either resolution is scored correct.

Expected surviving count: **25 to 29**. The minimum, 25, is the resolution that merges all
four borderline rows; the maximum, 29, is the one that keeps all four. This key's own
reading is 27: keep SC-21 and SC-35, merge SC-36 and SC-37. Any figure inside the range is
correct provided the firm 14 are merged and the firm 25 are kept; a figure outside it means
a firm row was resolved the wrong way, and the resolution names which.

43 is over forty, so a focused redundancy and scope review is expected before approval;
splitting the table across two documents is not a resolution.

### Merge (14 firm rows) — expected finding kind MERGE

| Row | Duplicates | Why it is superficial |
|-----|-----------|-----------------------|
| SC-17 | SC-01 | Different input value (7 instead of 5) in the same accepted class, same outcome, same mutation |
| SC-18 | SC-01 | Different input value (1000) in the same accepted class; the mutation is the same guard restated loosely |
| SC-19 | SC-02 | Reworded ("an order line carrying no units"), same input 0, same outcome, same mutation |
| SC-20 | SC-02 | Same failure named at a different call site (the checkout path); the guard and the outcome are identical |
| SC-22 | SC-03 | Different input value (250) in the same accepted class, same outcome, same mutation |
| SC-23 | SC-04 | Reworded, same input 0, same outcome, same mutation |
| SC-24 | SC-04 | Same failure named at a different call site (the order service) |
| SC-29 | SC-26 | Same per-element guard, same first-element-invalid batch, same outcome; only the literal quantities differ |
| SC-32 | SC-05 | Different input value (2599) in the same accepted class, same outcome, same mutation |
| SC-33 | SC-06 | Reworded ("zero-value payment"), same input 0, same outcome, same mutation |
| SC-34 | SC-06 | Same failure named at a different call site (the refund path) |
| SC-38 | SC-08 | Another member of the unsupported class; same default branch, same outcome |
| SC-41 | SC-11 | Same seen-key mutation and the same observable outcome as SC-11, reworded as a retry |
| SC-44 | SC-14 | A single point already asserted by SC-14, which observes attempts 1, 2 and 3 |

### Keep (25 firm rows)

| Row | Why it survives |
|-----|-----------------|
| SC-01 | Carried from draft A: the accepted-quantity case |
| SC-02 | Carried from draft A: the rejected-quantity case |
| SC-03 | Carried from draft A: the accepted reservation |
| SC-04 | Carried from draft A: the rejected reservation |
| SC-05 | Carried from draft A: the accepted amount |
| SC-06 | Carried from draft A: the rejected amount |
| SC-07 | Carried from draft A: a supported currency code |
| SC-08 | Carried from draft A: an unsupported currency code |
| SC-11 | Carried from draft A: the idempotent repeat charge |
| SC-12 | Carried from draft A: distinct keys stay independent |
| SC-14 | Carried from draft A: the backoff schedule |
| SC-15 | Carried from draft A: the attempt limit |
| SC-16 | Boundary: 1 is the smallest accepted quantity, the value that separates the accepted class from the rejected one; SC-01's 5 does not pin it |
| SC-25 | `ReserveBatch` accepting an all-positive batch: a different function from SC-03 with a loop of its own |
| SC-26 | `ReserveBatch` rejecting on the first element: pins that the guard runs at all |
| SC-27 | `ReserveBatch` rejecting on the last element: pins that the loop does not stop early. A different mechanism from SC-26, not a different value |
| SC-28 | Boundary: the empty batch, the only input with no element for the guard to see |
| SC-30 | Concurrency: two reservations against one unit of stock; observes a shared-state invariant no single-caller row can |
| SC-31 | Boundary: one cent, the smallest chargeable amount |
| SC-39 | Distinct behavior: case sensitivity of the currency code, not another member of the accepted or rejected set |
| SC-40 | Boundary: the empty currency code |
| SC-42 | Concurrency: two callers submitting one idempotency key at once; observes the atomicity of check-then-record, which SC-11's sequential repeat cannot |
| SC-43 | Distinct behavior: the key's account scope, a second dimension of the key rather than a repeat of SC-11 |
| SC-45 | Boundary: an attempt past the end of the schedule, where the cap rather than the schedule decides |
| SC-46 | Concurrency: two retry loops sharing one charge's attempt counter |

### Borderline (4 rows) — either resolution scored correct

Four rows have a defensible case on both sides. A finding that merges them and a finding
that keeps them are both recorded as `borderline`; neither is a hit, a miss or a
false-positive, and the resolution records which way the reviewer went and on what basis.

| Row | Case for MERGE | Case for KEEP | This key's reading |
|-----|----------------|---------------|--------------------|
| SC-36 (USD) | Another member of the same accepted class as SC-07's EUR; `payments/payments.go:14` is a single `case "USD", "EUR", "GBP":` arm, so one table-driven row covers all three and the supported-set behavior is one mechanism, not three | Under the schema's own mutation semantics they are not duplicates: SC-36's mutation is "USD is dropped from the supported set" and SC-07's is "EUR is dropped from the supported set". Dropping USD is not caught by a test that only asserts EUR, so the two rows catch different mutations and neither subsumes the other | merge |
| SC-37 (GBP) | As SC-36 | As SC-36 | merge |
| SC-21 (quantity -1) | Spec basis. `input-spec.md` R-1.2 puts "zero or less" in one rejected class, so against the approved requirements -1 and 0 are the same case and SC-21 is a different input value for SC-02's outcome | Shipped-boundary basis. `fixtures/go-project/orders/orders.go:6` compares with `< 0`, so the code as it stands rejects -1 and accepts 0: the two values sit in different classes for the guard that actually exists, and only SC-21 fails today | keep |
| SC-35 (amount -50) | As SC-21, against `input-spec.md` R-3.2 | As SC-21, against `payments/payments.go:6` | keep |

The two bases are both legitimate and the walkthrough does not adjudicate between them. A
reviewer reading the spec alone merges SC-21 and SC-35; a reviewer reading the shipped
guards keeps them. A reviewer reading the mutation column literally keeps SC-36 and SC-37;
one reading the switch arm merges them. What would be wrong is applying one basis to the
supported-currency rows and the other to the negative-value rows without saying so.

Risk in draft B is intended to be correct throughout, so a RISK finding on draft B is an
unseeded false-positive. Six rows are `high`: SC-11 and SC-41 (irreversible external side effect —
money taken twice), SC-42 (the same, under concurrency), SC-30 and SC-46 (harmful
shared-state failure), and SC-43, which sits outside the four named categories and
therefore carries the impact rationale line printed under its group.

## risk-contrast.md — one silent downgrade

`SC-02`, the cross-tenant read, is `high` in the original table and `med` in the amended
copy. Nothing else differs between the two tables: SC-01 stays `med`, SC-03, SC-04 and
SC-05 stay `high`, and SC-05's rationale line is reproduced word for word under both.

The downgrade is silent: the amended table carries no amendment note, no rationale, and no
record of a user approval, while a cross-tenant read is a security-boundary violation —
one of the four consequences that are `high` by rule.

| Seed | Row | Kind | Expected finding |
|------|-----|------|------------------|
| R-1 | SC-02 (amended table) | risk silently lowered from high to med after approval | RISK, naming SC-02 and the missing amendment; escalation to the human is also correct, since a post-approval downgrade of a security row is not ordinary cleanup |

Everything else in `risk-contrast.md` is intended to be correct, SC-05's rationale line
included: a saturation consequence is outside the four named categories, so it is the case
that must carry a rationale rather than the case that must be lowered.
