# test-design

A Claude Code plugin adding two stages to the [superpowers](https://github.com/obra/superpowers) flow.

| Stage | When | What it produces |
|---|---|---|
| `test-coverage-design` | after the spec is approved, before `writing-plans` | a coverage doc: invariants, out-of-scope, and an ID'd scenario table |
| `coverage-verification` | at the `finishing-a-development-branch` seam | a verification report across four layers |

## Why

The superpowers flow goes from spec straight to plan. Test thinking exists only as
per-task TDD steps, so nothing ever asks which tests the feature needs before tasks get
drawn — and nothing later checks that the tests written actually catch the bugs they
nominally guard against. A green suite proves tests pass, not that they bite.

## The four verification layers

1. **trace** — every scenario has a tagged test that the runner reports as *executed and passed*
2. **adversarial read** — a fresh subagent judges whether each test would fail if the named bug returned
3. **live** — smoke and e2e rows plus derived happy and non-happy walkthroughs run against the real system
4. **mutation** — for high-risk rows, the named mutation is applied and the test must fail

## Install

```bash
claude plugin marketplace add /home/igi21/experiements/test-design
claude plugin install test-design@test-design
```

Then install the gate rule — see [docs/enforcement.md](docs/enforcement.md) for the text
and for why it must name the `brainstorming` instruction it supersedes.

## Design

See `docs/superpowers/specs/2026-07-30-test-design-plugin-design.md`.
