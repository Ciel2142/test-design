# Opt-in workflow

`test-design` is passive. Installing it exposes skills and agents but does not change the
Superpowers workflow until the user explicitly invokes the coverage-design skill for a
feature.

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
   coverage source inline in the spec; larger features use a dedicated coverage doc.
2. `writing-plans` cites those SC IDs and requires matching `covers: SC-nn` tags on tests.
3. At `finishing-a-development-branch`, `coverage-verification` runs only because the
   approved coverage source proves this feature opted in. Its own approval gate still
   applies before any verification work starts.

Not invoking the first stage means neither stage runs. No prompt, waiver, or explanation
is required.

## Composing with wave-planning

When `wave-planning` is also applicable, the opted-in sequence is:

```text
test-coverage-design → wave-planning → (per wave: writing-plans → execute)
→ coverage-verification
```

One feature branch spans all waves. `coverage-verification` runs once after the final wave.
