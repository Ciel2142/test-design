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

- `SC-01` is bound and passes — also the fixture that pins the trace evidence's binding
  site (`file`, `line`, `endLine`) to an exact range in `orders/orders_test.go`.
- `SC-02` and `SC-05` are bound and fail, yielding `red` findings.
- `SC-03` and `SC-06` are cited by a skipped test, yielding `gap` findings.
- `SC-04` and `SC-07` are cited by no test at all, also `gap`.
- `SC-08` is cited by two different tests in two different packages
  (`TestAmbiguousBindingOne` in `orders`, `TestAmbiguousBindingTwo` in `payments`),
  yielding `ambiguity`.
- `SC-90` is retired but still cited by a test, and `SC-77` is cited by no row at all;
  both yield `drift` findings, and `SC-77` classifies as an unknown ID.
- `TestOverclaimedCoverage` cites three active IDs on one test, yielding `overclaim`.
- `SC-09` is a test name (`TestSameName`) declared identically in both `orders` and
  `payments`; only the `orders` copy is tagged. This is the row that proves the join is
  scoped by the tag's file, not by the test name alone.
- `SC-10` is the fixture's one bound `risk: high` row; `TestE2E_FinalizePipeline` reports
  its mutation run `environmental` to reach a `BLOCKED` verdict without invoking the
  `mutator` agent.

`TestE2E_FixturePipeline` also asserts that two `trace` runs against the same repository
state produce byte-identical evidence apart from timestamps and run IDs, and that `trace`
itself writes nothing into the repository it inspects.

## `maven-java-project/`

Layer 1's JVM fixture for Maven, exercised by `baselinejvm_test.go`: a nested test class
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
