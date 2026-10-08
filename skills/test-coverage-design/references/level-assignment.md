# Level assignment

`Level` takes exactly `unit`, `component`, `integration`, `smoke`, or `e2e`.

**Assign the cheapest level that can actually catch the named mutation.** Never a higher
level out of habit — an e2e test for an off-by-one guard is slow, flaky, and no more
truthful than the unit test.

| Level | Catches | Cost |
|---|---|---|
| `unit` | logic in one unit, no I/O | milliseconds |
| `component` | a rendered component's state and interaction | fast, in-process |
| `integration` | real collaborators — DB, broker, lock, transaction | seconds |
| `smoke` | the deployed surface answers correctly | needs the system up |
| `e2e` | a full path across services | slowest, most fragile |

Rows at `integration`, `component`, `smoke`, or `e2e` need a harness hint: environment,
fixtures, what is touched.

## Levels and the live decision

Layer 3 does not execute rows one at a time. It executes the journeys of the coverage
source's `## Live decision`, and a journey observes whichever rows it names.

A `smoke` or `e2e` row is a live obligation for as long as it is active: every one of them
appears in at least one journey, and a `not-applicable` decision cannot stand while one
exists.

A journey may also exercise `unit` or `integration` rows on its way through, and naming them
in its `Scenarios` cell records that it does. It does not relabel them — each keeps the
cheapest level that catches its mutation, and keeps being covered by its own test.

## What a live level cannot carry

`smoke` and `e2e` are earned by a positive observation on the deployed revision. Two shapes
of row cannot be observed that way and do not belong at a live level, whatever their
subject:

- **Absence as the only observable.** "No message is published", "the external call is not
  made", "no row is written" — a journey that waits and sees nothing has proved only that
  it waited. The row needs a positive signal a plan predicate can read (a count that stays
  at one, a rejection status, a recorded refusal), or it belongs at `unit` or `integration`
  where the absence is asserted directly on the collaborator.
- **A retry or timeout window as the thing under test.** A journey has one deadline per
  command and no polling predicate; backoff, retry exhaustion and timeout handling are
  `integration` rows against an injected clock or a failing collaborator.

A row that cannot be expressed through the live profile's declared commands is a row at the
wrong level, not a reason to widen the profile.

## Concurrency exception

**Rows on axis 5 default to `integration` or above.**

A unit test against a mock cannot produce real contention. Two mocked "concurrent" calls
execute sequentially in one thread, so the test passes whether or not the lock exists —
marking such a row covered is false confidence, which is worse than an admitted gap.

The only exception: code with an **injectable scheduler or clock**, where the interleaving
is deterministic and chosen by the test. Then `unit` is honest, because the test really can
produce the interleaving that triggers the bug.

`-race` complements the repeated-iteration harness above; it never replaces it. The race
detector only flags a data race it actually observes during the run — it proves nothing
about an interleaving the run never hits, so the iteration count still carries the weight of
producing that interleaving.

## Rendering exception

**Rows on axis 11 default to `component` or above.**

Same argument: a jsdom environment is a mock of a browser. Layout, focus order, and
breakpoint behavior asserted against jsdom prove that jsdom agrees, not that a browser
does. `unit` is not an honest home for a rendering claim.
