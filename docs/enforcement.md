# Enforcement

The two stages are wired in by skill-text handoffs plus one rule in `~/.claude/CLAUDE.md`.

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

## Not hooks, for now

A hook could enforce this deterministically — Claude Code hooks run arbitrary commands, so
one could perform the entire tag-and-runner-report check. The reason to defer is the
unknown false-denial rate: a gate that blocks legitimate work mid-flow gets disabled
permanently, and that risk is unmeasured until the stage has real mileage.

Revisit after the dogfood phases. If the stage is observed being skipped, the first step up
is a **warning** hook on writes to `docs/superpowers/plans/*` that injects context about
whether a coverage doc exists — not a denial.
