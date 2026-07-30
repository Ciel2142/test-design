# Runner output: normalized contract

Layer 1 must distinguish *executed and passed* from *skipped, filtered out, or absent*.
A grep over source plus a green exit code cannot do that: a tag above a skipped test
satisfies the grep and leaves the suite green.

## The contract

Every command below prints one line per test:

    status<TAB>test-name

`status` is exactly `passed`, `failed`, or `skipped`. Anything a runner calls `pending`,
`todo`, `ignored`, or `disabled` normalizes to `skipped`. A test named in a tag but absent
from this output is a `gap`.

## Emitting the contract

### vitest / jest

```bash
npx vitest run --reporter=json --outputFile=.vitest-report.json
jq -r '.testResults[].assertionResults[]
       | "\(if .status == "pending" or .status == "todo" then "skipped" else .status end)\t\(.fullName)"' \
   .vitest-report.json
```

### go

```bash
go test -json ./... > .go-report.json
jq -r 'select(.Test != null and (.Action == "pass" or .Action == "fail" or .Action == "skip"))
       | "\(if .Action == "pass" then "passed" elif .Action == "fail" then "failed" else "skipped" end)\t\(.Test)"' \
   .go-report.json
```

### pytest

```bash
pytest --junitxml=.pytest-report.xml
```

Then the JUnit XML reader below.

### maven / gradle (surefire, failsafe)

```bash
mvn -q test                     # or: mvn -q -Pit-full verify
```

Then the JUnit XML reader below, globbing `target/surefire-reports/*.xml` and
`target/failsafe-reports/*.xml`.

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
        name = f"{tc.get('classname')}.{tc.get('name')}"
        if tc.find('skipped') is not None:
            status = 'skipped'
        elif tc.find('failure') is not None or tc.find('error') is not None:
            status = 'failed'
        else:
            status = 'passed'
        print(f"{status}\t{name}")
PY
```

`python3` and its stdlib `xml.etree` are the only requirement; no pip install.

**The DTD guard is not optional.** `xml.etree` expands internal entities, so a report
containing nested `ENTITY` declarations can blow up memory (billion laughs / quadratic
blowup). No legitimate JUnit report needs a DTD, so refusing any file that declares one
costs nothing and removes the class entirely — without pulling in `defusedxml` and breaking
the plugin's zero-dependency property.

Threat model, stated honestly: this parses the project's own test-runner output on a
developer machine, so exploiting it requires already controlling that output — a malicious
test dependency, or a report fetched from an untrusted CI artifact. Defense in depth, not a
live hole. A report rejected by the guard is reported to stderr and its tests count as
absent from the run, which surfaces as `gap` rather than as a silent pass.

## Extracting the test name a tag refers to

The tag sits immediately above a test declaration. Read forward from the tag to the next
non-blank line and extract the name with the pattern for that language. Join to the runner
output by **suffix match** on the name, because runners prefix with suite, class, or file.

| Language | Pattern | Example match |
|---|---|---|
| Go | `^func (Test\w+)` | `TestRejectsZeroQuantity` |
| Python | `^\s*def (test_\w+)` | `test_partial_write_rolls_back` |
| TS/JS | `^\s*(?:it\|test)(?:\.\w+)?\(\s*['"\`](.+?)['"\`]` | `rejects zero quantity` |
| Java/Kotlin | `(?:void\|fun) (\w+)\s*\(` on the line after `@Test` | `rejectsZeroQuantity` |
| Rust | `^\s*fn (\w+)` on the line after `#[test]` | `rejects_zero_quantity` |

Note the TS/JS pattern deliberately allows a modifier — `it.skip(`, `it.only(`,
`test.todo(` all match, so a skipped test is still *found* and then reported `skipped`
by the runner output. A pattern requiring `it(` exactly would silently miss it and the
row would read as `gap` for the wrong reason, hiding the real problem.

## Unrunnable suites

If the suite cannot execute in this environment (CI-only, licensed dependency, hardware in
the loop), do not fabricate the contract. Record the commands attempted and their exit
codes as attempt evidence, and let the verdict fall to `PARTIAL`.
