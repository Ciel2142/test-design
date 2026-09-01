# Live profiles

Layer 3 of `coverage-verification` runs `smoke` and `e2e` rows against a real system. A Git
worktree isolates source, not databases, ports, brokers, or credentials, so the layer runs
only under a **live profile**: a committed JSON document that declares, ahead of time, every
command the walkthrough may use and every resource it may touch. The `live-runner` agent
composes its plan from those declared commands only; the `execute-live` op refuses anything
the profile did not declare.

A profile is project configuration. Review it like build configuration. An agent never
writes or amends one during verification — a missing or invalid profile ends layer 3 as
`PARTIAL` with attempt evidence, not with a synthesized profile.

## Where it lives and how it is selected

```text
.test-design/live-profiles/<name>.json
```

The coverage source names it in its `## Verification` block:

```text
- live-profile: .test-design/live-profiles/<name>.json
```

The value is the repo-relative path; the verifier refuses one that escapes the repository.
The key is required whenever any active row is `smoke` or `e2e`, and absent otherwise. A
source with no such row omits the key and layer 3 reports `n/a`.

## Schema

`schemaVersion` must be `"1"`. Every other field is read by the verifier
(`internal/live/profile.go`):

| Field | Type | Meaning |
|---|---|---|
| `classification` | `"local"` or `"ephemeral"` | `local`: every declared endpoint must be loopback (`localhost`, `127.0.0.1`, `::1`). `ephemeral`: endpoints are whatever the profile lists — a throwaway environment the project provisions for the run. Production classification does not exist and is not a growth point. |
| `start` | `[][]string` | Commands run in order before the walkthrough, each as an argv array. The first nonzero exit stops the sequence; the walkthrough is then skipped and every bound row is `LIVE-INCONCLUSIVE`. |
| `health` | `[][]string` | Commands run in order after `start`. Same stop-on-first-failure rule. Use this for readiness probes so a slow start is not charged to a row. |
| `walkthrough` | `[][]string` | Commands the plan may use for the rows themselves. The verifier does **not** run these; they exist to declare programs. |
| `cleanup` | `[][]string` | Commands run unconditionally at the end, whether or not `start` succeeded. Required, non-empty. |
| `cleanupEvidence` | `[]Predicate` | Predicates evaluated against the **last** `cleanup` command's result. They are the profile author's own proof that the run left nothing behind; `cleanupOK` in the evidence is exactly whether they held. Required, non-empty. |
| `envAllowlist` | `[]string` | Names of environment variables the commands inherit from the verifier's process. Everything else is stripped. See "Environment and secrets". |
| `resourcePrefixes` | `[]string` | Every name or prefix the run may create. Each entry **must** contain `{run-id}`; the verifier refuses `run-id-missing-from-resources` otherwise, so two concurrent runs can never collide on a fixture. |
| `endpoints` | `[]string` | URLs or `host:port` pairs the plan may reference. Under `local`, each must resolve to loopback. Under `ephemeral`, list what the environment exposes. May be `[]`. |

### Predicate shape

```json
{"form": "exit-code", "value": "0"}
{"form": "matches", "stream": "stdout|stderr|combined", "value": "<regex>"}
{"form": "not-matches", "stream": "stdout|stderr|combined", "value": "<regex>"}
```

Exactly these three forms. `exit-code` takes no `stream`; `matches` and `not-matches`
require one and compile `value` as a Go regular expression. Richer evaluation belongs in a
checked-in project script whose exit code or output the predicates observe.

### `{run-id}` substitution

`prepare-live` replaces every `{run-id}` in `start`, `health`, `walkthrough`, `cleanup`,
`resourcePrefixes`, and `endpoints` with the verifier-generated run ID and hands the result to
`live-runner` as the `descriptor`. Put the token in every resource a command creates —
container names, scratch files, database names, queue prefixes — so cleanup can target the
run's own residue and nothing else.

## What the verifier enforces on the plan

`live-runner` returns a plan; `execute-live` validates it against the profile before any
process starts. A refusal names the rule broken:

| Refusal | Rule |
|---|---|
| `undeclared-command` | A step's `argv[0]` must appear as `argv[0]` of some entry in `start`, `health`, `walkthrough`, or `cleanup`. Declaring `["curl", "-s", "http://localhost:8080/health"]` under `health` declares `curl` for every step. |
| `shell-invocation-rejected` | `sh -c`, `bash -c`, `zsh -c`, `dash -c`, and a single-token command containing whitespace are refused. A recipe that needs shell semantics is a checked-in script invoked by path. An interpreter such as `python3 -c` passes this guard only if the profile declares `python3`; declaring it is the project's decision. |
| `undeclared-endpoint` | Any token shaped like a URL or `host:port` must equal a declared endpoint or extend one with `/`, `?`, or `#`. |
| `inexpressible-predicate` | A predicate outside the three forms above, a non-integer `exit-code`, or a regex that does not compile. |
| `unbound-row` / `unknown-row` / `gap-row-bound` | Every applicable row not already a layer-1 gap must be bound by at least one step; no step may bind a row outside the applicable set or a row layer 1 already reported as a gap. |

These are defects in the plan, not in the profile. The skill re-dispatches `live-runner`
quoting the refusal.

## Execution order and verdicts

1. `start` commands, in order, stop on first nonzero exit.
2. `health` commands, in order, stop on first nonzero exit.
3. If both passed: plan `steps`, in order. Each step's predicates decide `LIVE-PASS` or
   `LIVE-FAIL` for every row it binds; a step whose process never ran is
   `LIVE-INCONCLUSIVE`. A row bound by several steps takes the worst verdict.
4. Plan `cleanup` steps, unconditionally.
5. Profile `cleanup` commands, unconditionally; `cleanupEvidence` is evaluated against the
   last one.

Every command runs in the disposable worktree with the sanitized environment. Raw stdout and
stderr land under the run directory outside the repository; the evidence carries reduced,
redacted transcripts plus SHA-256s of the raw files.

## Environment and secrets

Commands see only the variables named in `envAllowlist`, taken from the verifier's own
process. `PATH` almost always belongs there. Credentials do not — a profile that allowlists a
secret so a walkthrough can authenticate has moved that secret into committed configuration
by name and into every transcript by value.

Two further guards apply regardless of the allowlist:

- Command arguments that look like credentials (`--token=…`, `--password …`, `api_key=…`,
  and similar) are redacted in the recorded command line.
- **Redaction barrier**: if a step's decisive output or command line contains the *value* of
  any parent-process variable **not** on the allowlist, the transcript is withheld entirely
  and the evidence names the variable, never the value. Design the walkthrough so this
  cannot fire: a withheld transcript is a step with no readable evidence.

## Writing one

The shipped fixture (`fixtures/live-project/.test-design/live-profiles/fixture-local.json`)
is the smallest complete profile:

```json
{
  "schemaVersion": "1",
  "classification": "local",
  "start": [["touch", "scratch-{run-id}.txt"]],
  "health": [["ls", "scratch-{run-id}.txt"]],
  "walkthrough": [["echo", "live-fixture-ready"], ["printenv", "LIVE_FIXTURE_CANARY"]],
  "cleanup": [
    ["rm", "-f", "scratch-{run-id}.txt"],
    ["find", ".", "-maxdepth", "1", "-name", "scratch-{run-id}.txt"]
  ],
  "cleanupEvidence": [
    {"form": "exit-code", "value": "0"},
    {"form": "not-matches", "stream": "stdout", "value": "scratch-"}
  ],
  "envAllowlist": ["PATH"],
  "resourcePrefixes": ["scratch-{run-id}"],
  "endpoints": []
}
```

Note how cleanup is two commands: the first removes, the second proves removal, and the
evidence predicates read the prover. A cleanup whose last command is the destructive one can
only ever prove "the delete exited 0".

A realistic `local` profile for a service under test follows the same pattern with
project-owned scripts doing the work:

```json
{
  "schemaVersion": "1",
  "classification": "local",
  "start": [["./scripts/live-up.sh", "{run-id}"]],
  "health": [["curl", "-fsS", "http://localhost:18080/actuator/health"]],
  "walkthrough": [
    ["curl", "-fsS", "http://localhost:18080"],
    ["./scripts/live-seed.sh", "{run-id}"]
  ],
  "cleanup": [
    ["./scripts/live-down.sh", "{run-id}"],
    ["docker", "ps", "-a", "--filter", "name=svc-{run-id}", "--format", "{{.Names}}"]
  ],
  "cleanupEvidence": [
    {"form": "exit-code", "value": "0"},
    {"form": "not-matches", "stream": "stdout", "value": "svc-"}
  ],
  "envAllowlist": ["PATH", "HOME", "DOCKER_HOST"],
  "resourcePrefixes": ["svc-{run-id}"],
  "endpoints": ["http://localhost:18080"]
}
```

Checklist before committing a profile:

- `schemaVersion` is `"1"` and `classification` is `local` or `ephemeral`.
- Every program the walkthrough will need appears as `argv[0]` somewhere in the four
  command lists.
- Every endpoint the walkthrough will hit is listed, loopback under `local`.
- Every created resource carries `{run-id}` and is listed under `resourcePrefixes`.
- `cleanup` ends with a command whose output proves the resources are gone, and
  `cleanupEvidence` reads that output.
- `envAllowlist` holds no credential.
- The profile is committed. Verification runs against a clean tree, and the report is only
  reproducible from configuration that is part of the repository.

`prepare-live` validates the profile before dispatch. Its refusals name the failing rule:
`unsupported-classification`, `undeclared-endpoint`, `run-id-missing-from-resources`,
`missing-cleanup`. Each writes `live-attempt.json` so the run continues as `PARTIAL`; fix the
profile and rerun verification.
