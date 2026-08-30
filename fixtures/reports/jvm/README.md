# JVM JUnit XML fixtures

Captured JUnit XML output from four real producers, plus a set of hand-crafted
XML files exercising rejection paths (DTD/entity, namespaces, malformed XML,
missing required attribute).

## Provenance

Captured 2026-08-30 on Linux (WSL2), using locally installed tools — no
wrappers are committed; regenerate with the same tool versions or newer
compatible ones.

| Tool | Version | Source |
| --- | --- | --- |
| JDK | Temurin 25.0.3+9-LTS (`25.0.3-tem`) | sdkman, `~/.sdkman/candidates/java/current` |
| JDK (Kotlin toolchain) | Temurin 21.0.10-tem | sdkman, used only as the Gradle/Kotlin JVM toolchain target (kotlinc did not yet support JDK 25 as a compile target at capture time) |
| Maven | Apache Maven 3.8.7 | `/usr/bin/mvn` (system package, `/usr/share/maven`) |
| Gradle | 9.7.0 | installed via `sdk install gradle` |
| Kotlin Gradle plugin | 2.1.20 (`kotlin("jvm")`) | Gradle Plugin Portal |
| JUnit Jupiter | 5.11.4 (`junit-jupiter`, `junit-jupiter-params`), platform launcher via `junit-platform:junit-platform-launcher` (resolved through the `junit-bom:5.11.4`) | Maven Central |
| maven-compiler-plugin | 3.13.0 | Maven Central |
| maven-surefire-plugin | 3.5.2 | Maven Central |
| maven-failsafe-plugin | 3.5.2 | Maven Central |

## Shared test matrix

The same matrix is implemented identically (same class-name stems, method
names, and outcomes) across `maven-surefire`, `maven-failsafe`, and
`gradle-java`:

- `SimpleTest` — `passingTest` (pass), `failingAssertionTest` (assertion
  failure → `<failure>`), `throwingExceptionTest` (thrown exception →
  `<error>` on the Maven producers), `disabledTest` (`@Disabled`), and a
  `@Nested` inner class `NestedCases` with `nestedPassingTest`.
- `ParamTest` — `allPassing(int)` (`@ParameterizedTest` / `@ValueSource`, all
  three invocations pass) and `oneFailing(int)` (same source, invocation `[2]`
  fails).
- `DisplayNameTest` — class-level `@DisplayName("Custom Class Display Name")`
  and method-level `@DisplayName("custom method display")` on `customDisplay`.
- `AlphaTest.sharedMethod` / `BetaTest.sharedMethod` — identical method name
  declared in two different classes.

`maven-failsafe`'s classes are the same matrix mirrored into `*IT`-suffixed
classes (`SimpleIT`, `ParamIT`, `DisplayNameIT`, `AlphaIT`, `BetaIT`) in the
same Maven module — Failsafe's default include patterns only pick up
`*IT`/`IT*`/`*ITCase` classes, so the class-name suffix is the one place the
three Java producers necessarily diverge; method names, nested-class names,
parameterized arguments, `@DisplayName` text, and outcomes are byte-for-byte
identical to their `*Test` counterparts.

`gradle-kotlin` mirrors the same matrix (Kotlin syntax, same class/method
names and outcomes) and adds the two Kotlin-only cases named in the brief:

- `BacktickTest` (declared in `KtBackticks.kt` — file name differs from the
  class name it declares) with two backtick method names, one of them
  containing spaces: `` `passes with backticks and spaces`() `` and
  `` `alsoBackticked`() ``.

Total per producer: 6 test classes (5 shared + `NestedCases` as its own
reported suite), 14 shared-matrix test cases, plus 2 more in
`gradle-kotlin` (`BacktickTest`) for 16.

## Producer commands (exact, as run for this capture)

All commands were run with `JAVA_HOME` pointed at the sdkman "current" JDK
(25.0.3-tem) unless noted otherwise for the Kotlin toolchain.

```bash
# maven-surefire + maven-failsafe (one Maven module, both plugins bound in pom.xml)
cd fixtures/reports/jvm/capture-src/maven
export JAVA_HOME=~/.sdkman/candidates/java/current
export PATH="$JAVA_HOME/bin:$PATH"
mvn -Dmaven.test.failure.ignore=true integration-test
cp target/surefire-reports/TEST-*.xml  ../../maven-surefire/
cp target/failsafe-reports/TEST-*.xml  ../../maven-failsafe/
```

`integration-test` runs the `test` phase (Surefire, on `*Test` classes) then
the `integration-test` phase (Failsafe, on `*IT` classes) in one invocation,
so both report sets come from a single build. `-Dmaven.test.failure.ignore=true`
keeps Surefire from aborting the build before Failsafe runs; Failsafe itself
does not fail the build until the (unused here) `verify` goal.

```bash
# gradle-java
cd fixtures/reports/jvm/capture-src/gradle-java
source ~/.sdkman/bin/sdkman-init.sh
export JAVA_HOME=~/.sdkman/candidates/java/current
gradle test --continue
cp build/test-results/test/TEST-*.xml ../../gradle-java/
```

```bash
# gradle-kotlin
cd fixtures/reports/jvm/capture-src/gradle-kotlin
source ~/.sdkman/bin/sdkman-init.sh
export JAVA_HOME=~/.sdkman/candidates/java/current
gradle test --continue
cp build/test-results/test/TEST-*.xml ../../gradle-kotlin/
```

Both Gradle builds set `test { ignoreFailures = true }` in their build
scripts, and `gradle-kotlin`'s `build.gradle.kts` pins
`kotlin { jvmToolchain(21) }` — at capture time the Kotlin 2.1.20 compiler did
not yet support JDK 25 as a `-jvm-target` (it falls back with a warning), so
the Kotlin module's Java/Kotlin compile tasks are pinned to the sdkman JDK 21
toolchain to avoid a `compileTestJava`/`compileTestKotlin` target mismatch.
Gradle auto-detected that JDK from the sdkman candidates directory; no
explicit toolchain path was configured.

All four commands intentionally run some failing/erroring tests: each
14-test Java matrix reports 1 skip (`disabledTest`), 1 assertion failure
(`failingAssertionTest`), and 1 thrown exception (`throwingExceptionTest`,
plus one more assertion failure from `ParamTest.oneFailing`'s invocation
`[2]`) — reported as 2 failures + 1 error + 1 skip on the Maven producers,
and 3 failures + 1 skip on the Gradle producers (Gradle folds the thrown
exception into `<failure>` too — see "Observed producer divergences" below).
`gradle-kotlin` adds 2 more passing tests from `BacktickTest` (16 total).
Capturing the XML is what matters; the nonzero exit / reported failures are
expected and do not indicate a problem with the fixtures.

## Regeneration steps

1. Ensure the pinned tool versions (or newer, compatible ones) are on `PATH`:
   `mvn -version`, and `gradle -version` (install via
   `sdk install gradle` if missing).
2. Re-run the four commands above from a clean `capture-src/*` checkout (no
   `target/`, `build/`, or `.gradle/` directories are committed — see each
   project's `.gitignore`).
3. Diff the freshly generated XML against the committed files under
   `maven-surefire/`, `maven-failsafe/`, `gradle-java/`, `gradle-kotlin/`.
   Expect byte-identical structure modulo timestamps, elapsed times,
   `hostname`, and the `java.class.path`/`user.*`/`os.*` properties Surefire
   embeds (those are machine- and run-specific and are committed verbatim,
   unsanitized, per design).
4. Recopy and commit if the tool versions changed in a way that alters the
   XML shape (e.g. a Surefire/Gradle major version bump changing attribute
   sets).

## Observed producer divergences (informational, not defects)

These are real behavioral differences between the four producers' default
JUnit XML output, captured as-is because a later cross-producer equivalence
check needs real examples of what it must normalize:

- **Display names**: Maven Surefire's XML `name` attributes always use the
  technical class/method name, even when `@DisplayName` is present
  (`DisplayNameTest.xml` → `testcase name="customDisplay"`). Both Gradle
  producers substitute the `@DisplayName` text into the suite and testcase
  `name` attributes instead (`testsuite name="Custom Class Display Name"`,
  `testcase name="custom method display"`).
- **Error vs. failure**: Maven Surefire/Failsafe distinguish a thrown
  exception (`<error>`) from an assertion failure (`<failure>`). Gradle's
  JUnit XML writer reports both as `<failure>`.
- **Parameterized invocation naming**: Maven Surefire/Failsafe name
  parameterized invocations `oneFailing(int)[1]`, `[2]`, `[3]`. Gradle names
  them `[1] 1`, `[2] 2`, `[3] 3` (index + the JUnit5-generated argument
  display).

## Hand-crafted rejection fixtures

| File | Purpose | Well-formed XML? |
| --- | --- | --- |
| `dtd-entity.xml` | `<!DOCTYPE testsuite [...]>` with an external `SYSTEM` general entity (`&xxe;`), referenced in `<system-out>` — an XXE-style fixture. Verified with Python's `xml.etree.ElementTree` (which refuses to resolve external general entities by default, matching the hardened behavior a Go XML parser should also have): parsing fails with `undefined entity &xxe;`. | No (by design — a parser that resolves it instead would be the vulnerability) |
| `namespaced.xml` | Valid, namespace-prefixed JUnit-shaped report (`<j:testsuite xmlns:j="...">`, all children prefixed `j:`). | Yes |
| `malformed.xml` | Structurally broken XML — an unclosed `<testcase>` element causes a mismatched-tag parse error. | No |
| `missing-classname.xml` | Otherwise well-formed and valid JUnit-shaped XML, but the `<testcase>` element omits the `classname` attribute every real producer always emits. | Yes (well-formed XML; semantically incomplete as a JUnit report) |

All four were checked with `python3 -c "import xml.etree.ElementTree as ET; ET.parse(path)"` to confirm the well-formedness column above.

## Directory layout

```
fixtures/reports/jvm/
  capture-src/            # minimal source projects that produced the XML below (no wrappers, no build output committed)
    maven/                # Surefire (*Test) + Failsafe (*IT), one pom.xml
    gradle-java/           # Gradle Java, JUnit 5
    gradle-kotlin/         # Gradle Kotlin, JUnit 5, backtick names + filename/class-name mismatch
  maven-surefire/*.xml     # captured verbatim from capture-src/maven's target/surefire-reports
  maven-failsafe/*.xml     # captured verbatim from capture-src/maven's target/failsafe-reports
  gradle-java/*.xml        # captured verbatim from capture-src/gradle-java's build/test-results/test
  gradle-kotlin/*.xml      # captured verbatim from capture-src/gradle-kotlin's build/test-results/test
  dtd-entity.xml
  namespaced.xml
  malformed.xml
  missing-classname.xml
  README.md               # this file
```

## Note for `jvm-trace` planning

`jvm-trace` owns the decision of whether to promote `capture-src/*` into its
own live fixture projects (e.g. for a real end-to-end producer invocation) or
to create its own fixture projects from scratch. This task only captured
one-shot XML snapshots; `capture-src/*` was not designed to be a long-lived,
continuously-run project (no wrapper, no CI wiring).
