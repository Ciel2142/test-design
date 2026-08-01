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
`docs/superpowers/plans/` is written and no `docs/superpowers/coverage/*-coverage.md`
exists beside it, the hook exits 2, which feeds its message back to Claude.

It exists because the rule above does not ship with the plugin. Installing `test-design`
installs the skills, the agents, and nothing that makes anyone use them — and the failure is
silent, because a stage that never runs produces no error, just an absent artifact. The hook
is the only part of the gate that survives `claude plugin install`.

It cannot block. `PostToolUse` fires after the write has already happened, so exit 2 is
feedback, not a veto. It fails open on input it cannot read — an empty or malformed
payload, a `file_path` it cannot resolve to a repo root — exiting 0 and saying nothing.
(Not on a missing `jq`: without it the script's `sed` fallback still resolves the path and
still warns.) The message names the two legitimate cases (the floor rule, a
package-relative doc in a monorepo) so a correct run that trips it reads as expected rather
than as a defect.

What it does **not** do: correlate a plan with *its own* coverage doc. Any
`*-coverage.md` in the directory satisfies it, so from the repo's second feature onward the
hook is effectively silent. That is deliberate for a warning — matching plan to doc means
guessing a naming convention, and a warning that misfires gets muted — but it means the
hook is a first-plan tripwire, not a measurement of how often the stage is skipped. Do not
read its silence as evidence for or against the denial hook below.

## Not a *denial* hook, for now

A hook could enforce this deterministically — Claude Code hooks run arbitrary commands, so
one could perform the entire tag-and-runner-report check in `PreToolUse` and deny. The
reason to defer that is the unknown false-denial rate: a gate that blocks legitimate work
mid-flow gets disabled permanently, and that risk is unmeasured until the stage has real
mileage.

The warning hook above is not subject to that argument — it has no false-denial rate to
measure, because it denies nothing. Revisit the denial version after the dogfood phases,
using what the warning hook reveals about how often the stage is genuinely skipped.
