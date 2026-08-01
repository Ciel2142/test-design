# Runner output: normalized contract

Layer 1 must distinguish *executed and passed* from *skipped, filtered out, or absent*.
A grep over source plus a green exit code cannot do that: a tag above a skipped test
satisfies the grep and leaves the suite green.

## The contract

Every command below prints one line per test:

    status<TAB>scope<TAB>test-name

`status` is exactly `passed`, `failed`, or `skipped`. Anything a runner calls `pending`,
`todo`, `ignored`, or `disabled` normalizes to `skipped`. A test named in a tag but absent
from this output is a `gap`.

`scope` is the runner's own locator for *where* the test lives — a file path, a package
import path, or a fully-qualified class. It exists because test names are not unique across
a repo: `rejects invalid input` lives in three files in any real codebase, and a join on
name alone can bind a tag to a different file's result. See "Joining a tag to its row".

## Emitting the contract

### vitest / jest

```bash
npx vitest run --reporter=json --outputFile=.vitest-report.json   # vitest
npx jest --json --outputFile=.vitest-report.json                  # jest: --json, NOT --reporter=json
jq -r '.testResults[] as $f | $f.assertionResults[]
       | "\(if .status == "pending" or .status == "todo" then "skipped" else .status end)\t\($f.name)\t\(.fullName)"' \
   .vitest-report.json
```

The two runners share a report shape but not a flag. `jest --reporter=json` exits 0 and
writes **no file** — `--reporter` is not a jest flag, and `--reporters` takes a module path.
The jq then reads a missing file, or worse, a stale report left from an earlier run, which
is a false pass by way of yesterday's results. Delete the report file before every run.

`scope` is `.testResults[].name`, the absolute test file path. Note `.fullName` joins the
`describe` chain to the test name with a **space**, not a delimiter — `orders validates
input` — so the chain cannot be split off, which is why the name rule below anchors on a
word boundary instead of splitting.

### go

```bash
go test -json ./... > .go-report.json
jq -r 'select(.Test != null and (.Action == "pass" or .Action == "fail" or .Action == "skip"))
       | "\(if .Action == "pass" then "passed" elif .Action == "fail" then "failed" else "skipped" end)\t\(.Package)\t\(.Test)"' \
   .go-report.json
```

`scope` is the package import path. `go test -json` emits an event for the parent test as
well as each subtest (`TestFoo`, `TestFoo/case_1`, …). **Do not bind a tag to the parent
alone.** A subtest that calls `t.Skip` leaves its parent `pass`, so a parent-only join
reports `passed` for a row whose cases never ran — a silent false pass, and table-driven
subtests are Go's dominant parametrization form. Treat `TestFoo` and every `TestFoo/...` as
one family and collapse worst-first; see "Joining a tag to its row".

### pytest

```bash
pytest --junitxml=.pytest-report.xml
```

Then the JUnit XML reader below.

### maven / gradle (surefire, failsafe)

```bash
mvn -q test                     # or: mvn -q -Pit-full verify
./gradlew test
```

Then the JUnit XML reader below, globbing `target/surefire-reports/*.xml`,
`target/failsafe-reports/*.xml`, and `build/test-results/test/*.xml`.

### JUnit XML reader (pytest, maven, gradle, and anything else emitting JUnit XML)

```bash
python3 - '<glob>' <<'PY'
import sys, glob, xml.etree.ElementTree as ET
for path in glob.glob(sys.argv[1]):
    head = open(path, 'rb').read(4096).lower()
    if b'<!doctype' in head or b'<!entity' in head:
        print(f"skipped-unsafe\t{path}: DTD/ENTITY declaration present", file=sys.stderr)
        continue
    for tc in ET.parse(path).getroot().iter('testcase'):
        if tc.find('skipped') is not None:
            status = 'skipped'
        elif tc.find('failure') is not None or tc.find('error') is not None:
            status = 'failed'
        else:
            status = 'passed'
        print(f"{status}\t{tc.get('classname')}\t{tc.get('name')}")
PY
```

`python3` and its stdlib `xml.etree` are the only requirement; no pip install.

`scope` is `classname` — a dotted module for pytest (`tests.orders.test_orders`, plus
`.TestGroup` for a method inside a class), a fully-qualified class name for surefire and
failsafe. `name` is the bare method, which carries the parametrization decoration:
`test_parametrized[1]` under pytest, `absentUnderOtherProfiles(String)[1]` under a JUnit 5
`@ParameterizedTest`. Emitting `classname` and `name` as separate columns rather than
concatenating them is what lets the join strip that decoration without also eating the
class.

**The DTD guard is not optional.** `xml.etree` expands internal entities, so a report
containing nested `ENTITY` declarations can blow up memory (billion laughs / quadratic
blowup). No legitimate JUnit report needs a DTD, so refusing any file that declares one
costs nothing and closes off the common case cheaply — without pulling in `defusedxml`
and breaking the plugin's zero-dependency property.

Threat model, stated honestly: this parses the project's own test-runner output on a
developer machine, so exploiting it requires already controlling that output — a malicious
test dependency, or a report fetched from an untrusted CI artifact. Defense in depth, not a
live hole. A report rejected by the guard is reported to stderr and its tests count as
absent from the run, which surfaces as `gap` rather than as a silent pass. The guard only
inspects the first 4096 bytes, so a report padded with enough leading comment or
processing-instruction content before the DTD declaration can slip past it.

## Extracting the test name a tag refers to

The tag sits immediately above a test declaration. Read forward from the tag to the next
non-blank line and extract the name with the pattern for that language.

| Language | Pattern | Example match |
|---|---|---|
| Go | `^func (Test\w+)` | `TestRejectsZeroQuantity` |
| Python | `^\s*def (test_\w+)` | `test_partial_write_rolls_back` |
| TS/JS | `^\s*(?:it\|test)(?:\.\w+)?\(\s*['"\`](.+?)['"\`]` | `rejects zero quantity` |
| Java/Kotlin | `(?:void\|fun) (\w+)\s*\(` on the line after `@Test` | `rejectsZeroQuantity` |
| Rust | `^\s*fn (\w+)` on the line after `#[test]` | `rejects_zero_quantity` |

`\|` inside the Pattern column is markdown table-cell escaping; the actual regex uses a bare `|`.

Note the TS/JS pattern deliberately allows a modifier — `it.skip(`, `it.only(`,
`test.todo(` all match, so a skipped test is still *found* and then reported `skipped`
by the runner output. A pattern requiring `it(` exactly would silently miss it and the
row would read as `gap` for the wrong reason, hiding the real problem.

## Joining a tag to its row

You know two things about a tag: the **file** you grepped it from, and the **name** you
just extracted. Both are load-bearing. A bare suffix match on the name alone binds a tag to
whichever line happens to match first, and the runner's rows are not ordered by anything
meaningful — that is a silent false `passed` on a row whose own test is skipped.

Apply both filters, in this order.

### 1. Restrict by scope

Keep only rows whose `scope` corresponds to the tag's file:

Resolve both sides to absolute paths first. Every rule below then compares *trailing*
segments, because a runner's scope is rooted at its own rootdir/module/source-root, which is
rarely the directory you ran the grep from.

| Language | `scope` is | Corresponds when |
|---|---|---|
| TS/JS | test file path | the two absolute paths are the same file |
| Go | package import path | `scope`'s trailing segments equal the tag file's directory relative to the module root (`go list -m` gives the module path; strip it from `scope`). A root-package test has an empty directory and matches the bare module path |
| Python | dotted module, optionally `.Class` | `scope`, `.`→`/`, is a trailing-segment match for the tag file's path with `.py` stripped — `pytest`'s `classname` is relative to its **rootdir**, which in a monorepo sits above the sub-project, so exact equality fails on a correct suite |
| Java/Kotlin | fully-qualified class | `scope` up to the first `$` equals or trailing-segment-matches the tag file's path below the source root (`src/test/java`, `.../kotlin`, `.../groovy`, `.../scala`, `src/jvmTest/...`), `/`→`.`, extension stripped |
| Rust | module path | `scope` starts with the tag file's path below `src/`, `/`→`::` and `.rs` stripped |

Two JVM traps the `$` and trailing-segment rules exist for. JUnit 5 `@Nested` classes report
as `OuterTest$InnerClass`, not `OuterTest.InnerClass` — a rule accepting only `.` gaps every
nested test in the suite. And Kotlin, Groovy, and Scala permit a file name that differs from
the class it declares, so the path cannot always derive the class: when it does not, the
match fails and the row is a `gap (ambiguous)` to be resolved by hand, not a silent bind.

Rust is the one language with a name pattern above and **no emitter** under "Emitting the
contract" — none is specified here because `cargo test`'s stable output is plain text
written by parallel threads, and a `sed` pipeline over it drops any line that interleaves,
which surfaces as a false `gap`. `cargo test` prints `test <module::path::name> ... ok |
FAILED | ignored`, so `scope` is the module path and `name` its last segment: supply an
emitter that satisfies the contract (`cargo nextest` with a machine-readable format is the
usual answer), or record its absence as attempt evidence and let the verdict fall to
`PARTIAL`. Do not hand-parse the parallel text output and call the result a clean run.

### 2. Match the name, anchored

A row's `name` matches the tag's name when either holds:

- `name` equals it, or ends with it and the character immediately before is **not**
  alphanumeric or `_`. The boundary is the point: without it, a tag on `validates input`
  also matches a test called `revalidates input`.
- the same is true after stripping a trailing **parametrization decoration** — any run of
  trailing `(...)` and `[...]` groups, or a trailing `/subtest` path under Go. Try the
  undecorated comparison first, so a test genuinely named `handles input (edge case)` is not
  truncated. This is what makes `test_parametrized[1]`,
  `absentUnderOtherProfiles(String)[1]` and `TestFoo/case_1` reachable; matching on the raw
  name reports every parametrized test as a `gap`.

Raw-first ordering has one trap: a Java class carrying both `void check()` and
`@ParameterizedTest void check(String)` extracts the same tag name for both, and the raw
phase finds the plain `check` row alone and stops — the failing `check(String)[1..n]` rows
are never seen. When the raw phase yields exactly one survivor, run the stripped phase too
and take the union; a single tag name that resolves to two different declarations is
`gap (ambiguous)`, not a pass on the one that happened to match first.

Decoration-stripping does not rescue `it.each` / `test.each` under vitest and jest, because
those interpolate their arguments into the name itself — the source says `adds %i and %i`
and the runner says `adds 1 and 2`, with no decoration to strip. Tag a non-templated
wrapper, or accept the `gap` and say why. Every other parametrization form in the table
above carries its cases as a decoration and resolves.

### 2b. Break a tie on the enclosing suite, before calling it ambiguous

Anchoring at a word boundary is necessary but not sufficient. In sentence-style JS names,
`returns 404` also anchors inside `on missing id, returns 404` — the boundary is a space —
and so does `validates input` inside `re-validates input`. Both are legal, unique test names
in the same file, and stopping here would report `gap (ambiguous)` and block a clean suite.

So when more than one row survives step 2, reconstruct the tag's expected full name from its
source: read the enclosing `describe` / class / module titles outward from the tag, join them
to the test name the way that runner does (a space for vitest, `.` for JUnit XML), and
require exact equality. Only if that still leaves more than one is it ambiguous.

### 3. Read the verdict off the survivors

| Survivors | Verdict |
|---|---|
| none | `gap` — the test is absent from the run |
| one, or several that share one `scope` and one undecorated name | that test's status, worst-first: any `failed` → `failed`, else any `skipped` → `skipped`, else `passed` |
| several spanning more than one `scope` or more than one undecorated name | `gap (ambiguous)` — report the candidates |

The parametrized set collapses worst-first on purpose: one skipped case in a parametrized
family means the row is not fully covered, and reporting it `passed` would hide exactly
what layer 1 exists to catch.

`gap (ambiguous)` is a real finding, not a fallback. It means the scope rule could not pin
the tag to one runner scope, and guessing there is how a tag ends up bound to a different
file's passing result. Fix it by making the name unique or by correcting the scope mapping —
never by picking a candidate.

## Unrunnable suites

If the suite cannot execute in this environment (CI-only, licensed dependency, hardware in
the loop), do not fabricate the contract. Record the commands attempted and their exit
codes as attempt evidence, and let the verdict fall to `PARTIAL`.
