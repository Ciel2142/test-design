# test-design

A Claude Code plugin adding two stages to the [superpowers](https://github.com/obra/superpowers) flow.

| Stage | When | What it produces |
|---|---|---|
| `test-coverage-design` | after explicit opt-in, before `writing-plans` | a coverage source: invariants, out-of-scope, and an ID'd scenario table |
| `coverage-verification` | at the `finishing-a-development-branch` seam | a verification report across four layers |

The plugin is passive. Installing it does not prompt, amend planning, or run either stage.
After a spec is approved, invoking `/test-design:test-coverage-design` opts that feature
into both coverage design and final coverage verification. Features that do not opt in
follow the normal Superpowers flow without waiver markers or warnings.

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

Installing gives you the skills and agents only. It registers no hook and requires no
global `CLAUDE.md` rule. Ordinary plan writes receive no warning or denial.

To opt one feature into the complete two-stage workflow, invoke
`/test-design:test-coverage-design` after its spec is approved. See
[docs/opt-in.md](docs/opt-in.md) for the resulting handoffs and `wave-planning`
composition.
