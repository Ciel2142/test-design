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

Rows at `component`, `smoke`, or `e2e` need a harness hint: environment, fixtures, what is
touched. Layer 3 executes the `smoke` and `e2e` rows.

## Concurrency exception

**Rows on axis 5 default to `integration` or above.**

A unit test against a mock cannot produce real contention. Two mocked "concurrent" calls
execute sequentially in one thread, so the test passes whether or not the lock exists —
marking such a row covered is false confidence, which is worse than an admitted gap.

The only exception: code with an **injectable scheduler or clock**, where the interleaving
is deterministic and chosen by the test. Then `unit` is honest, because the test really can
produce the interleaving that triggers the bug.

## Rendering exception

**Rows on axis 11 default to `component` or above.**

Same argument: a jsdom environment is a mock of a browser. Layout, focus order, and
breakpoint behavior asserted against jsdom prove that jsdom agrees, not that a browser
does. `unit` is not an honest home for a rendering claim.
