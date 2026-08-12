# test-design

A Claude Code plugin adding two stages to the [superpowers](https://github.com/obra/superpowers) flow.

| Stage | When | What it produces |
|---|---|---|
| `test-coverage-design` | after the spec is approved, before `writing-plans` | a coverage doc: invariants, out-of-scope, and an ID'd scenario table |
| `coverage-verification` | at the `finishing-a-development-branch` seam | a verification report across four layers |

Both stages are human-gated per feature: above the floor rule, the user is asked at the
brainstorming seam whether coverage design should run. Declining records the literal line
`Coverage: waived (user decision, YYYY-MM-DD)` in the plan, silences the warning hook for
that plan, and skips `coverage-verification` at the finishing seam.

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

Clone, then point a local marketplace at the clone:

```bash
git clone http://192.168.1.34:3000/service/test-design.git
claude plugin marketplace add ./test-design
claude plugin install test-design@test-design
```

`claude plugin marketplace add` takes the path to the clone, so substitute wherever you put
it. The remote above is LAN-only; any clone of this repo works the same way.

Installing gives you the skills, the agents, and a warning hook that fires when a plan is
written and the coverage rows its test steps cite do not exist. It does **not** give you the
gate rule that puts
`test-coverage-design` between `brainstorming` and `writing-plans` — that lives in
`~/.claude/CLAUDE.md` and you have to add it. See [docs/enforcement.md](docs/enforcement.md)
for the text, for why it must name the `brainstorming` instruction it supersedes, and for
what the hook does and does not cover.
