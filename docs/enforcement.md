# Enforcement

The two stages are wired in by skill-text handoffs, one rule in `~/.claude/CLAUDE.md`, and
one warning hook the plugin ships.

## Why the rule must name what it supersedes

`superpowers:brainstorming` ends with an emphatic instruction:

> **The terminal state is invoking writing-plans.** Do NOT invoke frontend-design,
> mcp-builder, or any other implementation skill. The **ONLY** skill you invoke after
> brainstorming is writing-plans.

That text is loaded into context at exactly the moment the handoff happens, and it is more
local than a global memory rule. A gate rule that does not name it is relying on the agent
silently preferring the weaker instruction — and the resulting nondeterministic skipping
reads as "the stage got skipped" rather than "two instructions collided", which sends
whoever debugs it in the wrong direction.

## The rule

Append to `~/.claude/CLAUDE.md`:

```markdown
## Test design gate

`superpowers:brainstorming` states that the only skill it may invoke afterwards is
`writing-plans` ("The ONLY skill you invoke after brainstorming is writing-plans").
That rule is amended here. The sequence is:

  brainstorming → test-design:test-coverage-design → writing-plans

For features meeting test-coverage-design's floor rule (≤2 behaviors, no persistence, no
concurrency, no external dependency), the coverage table is appended inline to the spec
instead and brainstorming proceeds directly to writing-plans.

At the end of implementation, `superpowers:subagent-driven-development` hands off to
`finishing-a-development-branch`. Invoke `test-design:coverage-verification` at that
handoff, before the branch is finished. A plan with no coverage doc, or a branch with no
verification report, is incomplete.
```

## The warning hook

`hooks/coverage-doc-warning.sh`, wired as `PostToolUse` on `Write`. When a file under
`docs/superpowers/plans/` is written, the hook reads the SC-IDs the plan cites and checks
that something defines them. It exits 2 — which feeds its message back to Claude — when the
plan cites no SC-IDs at all, or cites IDs that appear in no
`docs/superpowers/coverage/*-coverage.md` and no `docs/superpowers/specs/*.md`.

Correlating on the plan's own citations, rather than on a filename slug, is what makes it a
per-plan check. `test-coverage-design` instructs `writing-plans` to cite SC-IDs in the plan's
test steps, so the plan names what must exist; a slug match would instead guess a naming
convention nothing enforces and warn on a compliant branch whose two documents named the
feature slightly differently. Specs count as a defining source because a floor-rule feature
keeps its coverage table inline in the spec.

It exists because the rule above does not ship with the plugin. Installing `test-design`
installs the skills, the agents, and nothing that makes anyone use them — and the failure is
silent, because a stage that never runs produces no error, just an absent artifact. The hook
is the only part of the gate that survives `claude plugin install`.

It cannot block. `PostToolUse` fires after the write has already happened, so exit 2 is
feedback, not a veto. It fails open on input it cannot read — an empty or malformed
payload, a `file_path` it cannot resolve to a repo root, a plan file it cannot read back —
exiting 0 and saying nothing. (Not on a missing `jq`: without it the script's `sed` fallback
still resolves the path and still warns.) The message names the legitimate cases (coverage
rows somewhere the hook does not look, a plan whose test steps are still to be written) so a
correct run that trips it reads as expected rather than as a defect.

What it still does **not** do: distinguish an SC-ID cited in a plan's test step from one
mentioned in its prose. It greps. The failure that leaves is a plan discussing SC-IDs it
does not actually test, which the hook passes. The reverse — a genuinely skipped stage — is
caught in both of its shapes, since such a plan either cites no IDs or cites IDs nothing
defines. Its silence is therefore usable as weak evidence for the denial-hook decision
below, in a way the earlier any-doc-exists check's was not.

## Not a *denial* hook, for now

A hook could enforce this deterministically — Claude Code hooks run arbitrary commands, so
one could perform the entire tag-and-runner-report check in `PreToolUse` and deny. The
reason to defer that is the unknown false-denial rate: a gate that blocks legitimate work
mid-flow gets disabled permanently, and that risk is unmeasured until the stage has real
mileage.

The warning hook above is not subject to that argument — it has no false-denial rate to
measure, because it denies nothing. Revisit the denial version after the dogfood phases,
using what the warning hook reveals about how often the stage is genuinely skipped. Weight
that evidence by the gap named above: a silent hook means no plan cited an undefined ID, not
that every cited row was actually tested.
