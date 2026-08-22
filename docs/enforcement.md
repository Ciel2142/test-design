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
That rule is amended here. After the spec is approved:

- Features meeting test-coverage-design's floor rule (≤2 behaviors, no persistence, no
  concurrency, no external dependency): append the coverage table inline to the spec and
  proceed directly to writing-plans, without asking.
- Otherwise, ask the user (AskUserQuestion): design test coverage for this feature, or
  skip it? If they choose coverage, the sequence is

    brainstorming → test-design:test-coverage-design → writing-plans

  If they skip, proceed directly to writing-plans and record the literal line
  `Coverage: waived (user decision, YYYY-MM-DD)` in the plan's header.

At the end of implementation, `superpowers:subagent-driven-development` hands off to
`finishing-a-development-branch`. For a feature with coverage rows (a coverage doc or an
inline floor-rule table), invoke `test-design:coverage-verification` at that handoff,
before the branch is finished — such a branch with no verification report is incomplete.
For a waived feature, do not invoke it and do not re-ask: state in one line that
verification is skipped because coverage was waived.
```

## Composing with wave-planning

The canonical block above is deliberately generic — it is copy-paste installable into
`~/.claude/CLAUDE.md` without the `wave-planning` plugin present.

With `wave-planning` installed, the sequence becomes
`brainstorming → test-design:test-coverage-design → wave-planning → (per wave: writing-plans → execute)`.
The waiver line is recorded in **each wave plan's** header, and `coverage-verification`
runs once, after the final wave — one feature branch spans all waves.

## The warning hook

`hooks/coverage-doc-warning.sh`, wired as `PostToolUse` on `Write`. When a file under
`docs/superpowers/plans/` is written, the hook reads the SC-IDs the plan cites and checks
that something defines them. It exits 2 — which feeds its message back to Claude — when the
plan cites no SC-IDs at all, or cites IDs that appear in no
`docs/superpowers/coverage/*-coverage.md` and no `docs/superpowers/specs/*.md`. A plan
containing the literal marker `Coverage: waived` is exempt: the user declined the stage at
the brainstorming seam, so its absent SC-IDs are the recorded decision, not a skip.

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

## Not a *denial* hook — decided, not deferred

A hook could enforce this deterministically: Claude Code hooks run arbitrary commands, so
one could perform the whole tag-and-runner-report check in `PreToolUse` and deny the write.
This was left open pending a measurement of how often the stage is actually skipped, because
a gate that blocks legitimate work mid-flow gets disabled permanently and never enforces
anything again.

**Measured 2026-08-01, and the answer is no.** `hooks/measure-skip-rate.sh` replays the
warning hook's predicate over every superpowers plan on a machine, dating each by its git
add rather than its mtime — mtime dates a plan by its last edit, which drags pre-stage plans
into the window and reads every one of them as a skip.

Over this machine: 723 plans across 77 repositories, of which 717 predate the stage
existing. In the window where the stage was live — plans first committed on 2026-07-31 or
later, the first full day after install:

| Plans | Cite SC-IDs, all defined | Would warn | Would be falsely denied |
|---|---|---|---|
| 5 | 5 | 0 | 0 |

Four repositories, 19 to 55 rows cited per plan. The skip rate in the measured window is
zero, so a denial hook would have denied nothing and its only measurable effect would have
been its own false-denial risk — which is not hypothetical: before the pattern was tightened
to two digits, the predicate matched a bare `SC-0` inside a shell fragment one plan quoted
and fired on a fully compliant document. One false fire in six is a rate a denial hook does
not survive, and tightening the pattern removes that instance without removing the class.

Read the number for what it is. n=5, one operator, all five written in sessions where the
gate rule was already in `~/.claude/CLAUDE.md`. It measures whether the skill-text handoff
plus that rule is sufficient *here*, not whether it is sufficient generally.

**Trip condition.** Re-run `hooks/measure-skip-rate.sh <root> <since>` when the corpus has
grown. Reopen the denial question if genuine skips reach 3 in a rolling window of 20
post-stage plans (15%), where genuine means the plan cites no SC-IDs, records no
`Coverage: waived` marker, and the repository has no coverage rows for that feature
anywhere — not merely that the hook fired. The script reports waived plans as their own
`waived` row, outside both the compliant and the skip counts. Below that, the
warning plus the rule is doing the work and a denial buys determinism at the cost of the
failure mode above.

Weight any future reading by the gap named earlier: the hook greps. A silent run means no
plan cited an undefined ID, not that every cited row was actually tested.
