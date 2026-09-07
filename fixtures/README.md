# Fixtures

Each fixture project pairs seeded source with a coverage source under
`docs/superpowers/coverage/`, so the verifier's ops can run against it exactly as they
would against a real repository. Every oddity in these fixtures is deliberate; do not
"fix" them.

## `go-project/`

Layer 1's fixture: two Go packages (`orders`, `payments`) sharing one module, exercised by
`baseline` and `trace` in `TestE2E_FixturePipeline` and by `finalize` in
`TestE2E_FinalizePipeline`. It proves the structural `covers:` binding and every finding
kind the trace op emits:

- `SC-01` is bound and passes — also the fixture that pins the trace evidence's primary
  binding site (`file`, `line`, `endLine`) to an exact range in `orders/orders_test.go`.
  It is a two-contributor row: `TestOverclaimedCoverage` cites it as well.
- `SC-02` and `SC-05` are bound and fail, yielding `red` findings. `SC-05` is the row whose
  failing contributor (`TestAmountValidation`) is the primary while a second, passing
  contributor (`TestOverclaimedCoverage`) also cites it.
- `SC-03` and `SC-06` are cited by a skipped test, yielding `gap` findings. Both keep the
  bound site in the dispatch inputs: only a row with no bound site is sourceless.
- `SC-04` is cited by no test at all, yielding `gap`. Its `// covers: SC-04` comment is
  free-floating between declarations, which is also the fixture's only `ambiguity`
  finding — the one the tag scanner raises, not one the join invents from cardinality.
- `SC-08` is cited by two different tests in two different packages
  (`TestAmbiguousBindingOne` in `orders`, `TestAmbiguousBindingTwo` in `payments`). Both
  credit the row: one `RowResult` with two contributors, no `ambiguity`.
- `SC-90` is retired but still cited by a test, and `SC-77` is cited by no row at all;
  both yield `drift` findings, and `SC-77` classifies as an unknown ID.
- `TestOverclaimedCoverage` cites three active IDs on one test. Every distinct active ID a
  site cites binds, so it credits `SC-07` on its own and joins `SC-01` and `SC-05` as a
  second contributor. It is the fixture that proves cardinality alone is never a finding.
  `SC-07` is `risk: high`, so crediting it also makes it layer-4 applicable.
- `SC-09` is a test name (`TestSameName`) declared identically in both `orders` and
  `payments`; only the `orders` copy is tagged. This is the row that proves the join is
  scoped by the tag's file, not by the test name alone.
- `SC-10` is one of the fixture's two bound `risk: high` rows — `SC-07` is the other — and
  the two-guard multi-site fixture: its tagged test, `TestReserveGuard`, exercises only
  `Reserve`, while `orders.go` guards both `Reserve` and `ReserveBatch` with the same
  shape of check. It carries the obligation from
  `docs/superpowers/specs/2026-08-09-multi-site-mutation-design.md` — each guard must be
  mutated and evaluated site-at-a-time, not collapsed into one row-level verdict —
  proved end-to-end by `TestE2EMutation_GoTwoGuardMultiSite`, which catches the `Reserve`
  mutant and lets the `ReserveBatch` mutant survive under its own site name. Separately,
  `TestE2E_FinalizePipeline` derives its layer-4 set from the trace — every active
  `risk: high` row layer 1 did not leave as a gap, currently `SC-07` and `SC-10` — and
  reports each row's mutation run `environmental` to reach a `BLOCKED` verdict without
  invoking the `mutator` agent.

`TestE2E_FixturePipeline` also asserts that two `trace` runs against the same repository
state produce byte-identical evidence apart from timestamps and run IDs, and that `trace`
itself writes nothing into the repository it inspects.

## `maven-java-project/`

Layer 1's JVM fixture for Maven, exercised by `baseline` in `baselinejvm_test.go` and by
`trace` in `TestTraceJVM_MavenPipeline`: a nested test class
bound by `Outer$Inner` (`SC-04`), a parameterized test whose invocations collapse to one
family verdict (`SC-05`), a passing, a failing, and a `@Disabled` test (`SC-01`–`SC-03`),
an uncited row (`SC-06`), and one bound `risk: high` row (`SC-07`). Skipped when `mvn` is
not on `PATH`.

## `gradle-kotlin-project/`

Layer 1's JVM fixture for Gradle/Kotlin, exercised by `tracejvm_test.go`: a backtick-named
test function (`SC-01`), a failing test (`SC-02`), a `@DisplayName`-annotated test whose
identity cannot come from the display text (`SC-03`), and one bound `risk: high` row
(`SC-04`). Skipped when neither `gradle` nor an sdkman Gradle install is on `PATH`.

## `live-project/`

Layer 3's fixture: no source code, only the three documents `prepare-live` and
`execute-live` read. `SC-11` passes, `SC-12` is bound to a deliberately unmet expectation
so the walkthrough yields one `LIVE-FAIL`, `SC-13` is the row the trace evidence marks as
a layer-1 gap, and `SC-14` is unit-level so it must never reach layer 3 at all.

Two oddities are load-bearing:

- the plan's `printenv LIVE_FIXTURE_CANARY` step expects **exit 1**. The canary is
  exported by the test and left off the profile's `envAllowlist`; an unsanitized child
  environment makes `printenv` succeed and turns `SC-11` into `LIVE-FAIL`.
- the profile's last cleanup command is a `find` whose `cleanupEvidence` predicate
  requires empty output, so `cleanupOK` is a real proof that the run-ID-named scratch
  resource is gone rather than a claim that `rm` was invoked.

## `reports/`

Captured and hand-crafted test-runner output, consumed directly by the report-format
parsers rather than by a running suite:

- `reports/go/` — `go test -json` transcripts (`happy-mix.json`, `build-fail.json`),
  read by `internal/gotestjson`'s tests to prove the Go JSON-lines parser against a real
  mixed pass/fail/skip/subtest run and a build failure.
- `reports/jvm/` — JUnit XML captured from real Maven Surefire, Maven Failsafe, Gradle
  Java, and Gradle Kotlin builds, plus hand-crafted rejection fixtures (malformed XML, an
  XXE-style DTD entity, a namespaced report, a report missing the `classname` attribute).
  Read by `internal/junitxml`'s tests to prove the XML parser and the cross-producer
  identity normalizer against every naming and structural divergence the four real
  producers are known to emit — see `reports/jvm/README.md` for the full provenance and
  producer-divergence notes.

## `workflow-redesign-live/`

Coverage setup for the plugin's own verification-workflow redesign. Unlike the
scratch/echo fixture, `service.go` is a real quote-calculation service: three items
must total 375 cents. `recipe.py` supplies finite `deploy`, `ready`, `check`, and
`cleanup` commands, selected by the root profile
`.test-design/live-profiles/workflow-redesign-local.json`.

Requires Python 3 and Go 1.26+. Run commands from the committed subject repository:

```text
python3 fixtures/workflow-redesign-live/recipe.py deploy <run-id>
python3 fixtures/workflow-redesign-live/recipe.py ready <run-id>
python3 fixtures/workflow-redesign-live/recipe.py check <run-id>
python3 fixtures/workflow-redesign-live/recipe.py cleanup <run-id>
```

The private HTTP-over-Unix-socket service and artifacts live only under
`/tmp/test-design-workflow-<run-id>`. Run IDs contain 1–40 ASCII letters, digits or
hyphens, beginning with a letter or digit. Deployment builds exact committed service
source and records the revision and binary fingerprint. Readiness is separate from
the business assertion; `check` rejects a healthy older deployment. A private token
authenticates shutdown, so cleanup never kills a PID taken from a stale file.
Cleanup refuses unexpected files, preserves other runs, and observes absence.

The recipe is trusted local project code, not a sandbox. No remote target, TCP
listener or service credentials are needed. Always run cleanup after an attempt;
an unproven shutdown is a cleanup failure, not permission to delete resources.

Actual setup observations and the old/new-subject reproduction are recorded in
`docs/superpowers/coverage/2026-09-07-verification-workflow-redesign-setup.json`.
They prove the harness, not completion of the redesign's SC rows or mandatory
real-agent walkthroughs.
