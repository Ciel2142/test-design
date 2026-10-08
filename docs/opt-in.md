# Opt-in workflow

`test-design` is passive. Installing it exposes skills and agents but does not change the
Superpowers workflow until the user explicitly invokes the coverage-design skill for a
feature. It supports Go, Maven, and Gradle projects only, and needs a Go 1.26+ toolchain
on the machine running either skill — every mechanical step runs through the plugin's own
Go executable, `go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier <op> …`, which
has no third-party dependencies and needs no build step or committed binary.

Before any such command, capture the absolute target repository while in the project,
not the plugin: `TARGET_REPO=$(git rev-parse --show-toplevel)`. Requests use that absolute
value as `repoRoot`; flags use `-repo "$TARGET_REPO"` and target-relative source paths.
The plugin's `-C` working directory is never the target root. `CLAUDE_PLUGIN_ROOT` itself
is usually empty in the Bash tool's shell; each skill opens with a step that resolves the
plugin root from the skill directory or from `~/.claude/plugins/installed_plugins.json`.

## Installation behavior

Installation does not:

- amend `superpowers:brainstorming` or require a global `CLAUDE.md` rule;
- prompt for coverage design on ordinary feature work;
- add coverage tables through a floor rule unless the skill was invoked;
- require waiver markers in plans; or
- inspect, warn about, or deny plan writes.

No hook is registered. A feature that does not opt in follows the same workflow as a
machine without this plugin.

## Opt in for one feature

After the feature spec is approved, invoke:

```text
/test-design:test-coverage-design
```

That invocation opts the feature into both stages:

1. `test-coverage-design` creates approved SC rows. Its floor rule keeps a small feature's
   coverage source inline in the spec; larger features use a dedicated coverage doc. Before
   the user approves the draft, it goes to the read-only `scenario-reviewer` agent for one
   review and exactly one recheck, and the source records a `## Live decision`: `required`,
   naming the journeys layer 3 will run over the deployed system, or `not-applicable` with
   the reason it does not apply.
2. `writing-plans` cites those SC IDs and requires matching `covers: SC-nn` tags on tests.
   Its lifecycle contract: the initial test run against untouched production code is
   either **TDD RED** (fails, as expected) or **ALREADY-GREEN** (passes, and returns to
   the parent for reconciliation). A row's **Verification mutation** — the coverage
   source's per-row code-level fault — informs test design but is applied only by final
   verification, never during implementation.
3. At `finishing-a-development-branch`, invoke
   `/test-design:coverage-verification <coverage-source>` explicitly — it runs only
   because the approved coverage source proves this feature opted in, and its own
   approval gate still applies before any verification work starts. A row's mutation
   evidence is **MUTATION-CAUGHT** only when its targeted test failed with the mutation
   applied and passed again after restore, at every site the mutation could be applied.

Not invoking the first stage means neither stage runs. No prompt, waiver, or explanation
is required.

## Composing with wave-planning

When `wave-planning` is also applicable, the opted-in sequence is:

```text
test-coverage-design → wave-planning → (per wave: writing-plans → execute)
→ coverage-verification
```

One feature branch spans all waves. `coverage-verification` runs once after the final wave.
Wave planning is an optional installed workflow, not a dependency this plugin introduces.
Without it, design hands directly to `superpowers:writing-plans`.
