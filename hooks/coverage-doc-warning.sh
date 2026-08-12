#!/usr/bin/env bash
# PostToolUse(Write) — warn when an implementation plan is written and the coverage rows it
# cites do not exist.
#
# Warning only, and structurally incapable of being anything else: PostToolUse runs after
# the write has already happened, so exit 2 feeds this text back to Claude without denying
# any tool call. That is the point, and it is now the settled design rather than a stopgap:
# `docs/enforcement.md` records the measurement that closed out the *denial* hook question.
#
# Fails open on anything it cannot read: empty or malformed payload, a `file_path` it
# cannot resolve to a repo root — all exit 0 and say nothing. A broken warning must never
# cost anyone a session. Note this is about *unreadable input*, not about jq: without jq the
# sed fallback below still resolves the path and the hook still warns.
#
# `set -u` only, deliberately. Not for the reason you might assume — `[ -e "$f" ] && exit 0`
# is a non-final command of an `&&` list and is exempt from `set -e`. The hazard is the
# `payload=$(...)` and jq assignments: under `set -e` a malformed payload aborts the script
# with a nonzero status, which the harness logs as a hook error instead of the intended
# silence.
set -u

payload=$(cat 2>/dev/null) || exit 0
[ -n "$payload" ] || exit 0

if command -v jq >/dev/null 2>&1; then
    file_path=$(printf '%s' "$payload" | jq -r '.tool_input.file_path // empty' 2>/dev/null)
    hook_cwd=$(printf '%s' "$payload" | jq -r '.cwd // empty' 2>/dev/null)
else
    # Single-line fallback extraction. Wrong on a path containing an escaped quote, which
    # is why it only ever suppresses the warning, never fabricates one.
    file_path=$(printf '%s' "$payload" | sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
    hook_cwd=$(printf '%s' "$payload" | sed -n 's/.*"cwd"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
fi

# Anchored on a path separator: `mydocs/superpowers/plans/` and `apidocs/superpowers/plans/`
# are not this repo's plan directory, and matching them would warn about a file that is not
# a plan.
case "$file_path" in
    */docs/superpowers/plans/*) rel=no ;;
    docs/superpowers/plans/*)   rel=yes ;;
    *) exit 0 ;;
esac

if [ "$rel" = yes ]; then
    # Relative path: only the session's own root can resolve it.
    root=${CLAUDE_PROJECT_DIR:-${hook_cwd:-}}
else
    # `%` not `%%`: strip at the *last* occurrence. A plan nested inside another repo's
    # plans directory belongs to the inner repo, and `%%` would resolve it to the outer one
    # and report the outer repo's missing coverage doc.
    root=${file_path%/docs/superpowers/plans/*}
fi
# Deliberately no fallback from an unresolvable absolute path to the session cwd: that
# guesses a repo the plan was never in, and a warning naming the wrong tree is worse than
# no warning.
[ -n "$root" ] && [ -d "$root" ] || exit 0

# A relative `file_path` is relative to the session root, not to this hook's cwd.
case "$file_path" in
    /*) plan_file=$file_path ;;
    *)  plan_file=$root/$file_path ;;
esac
[ -r "$plan_file" ] || exit 0

# A feature whose user declined coverage design at the brainstorming seam records the
# waiver in the plan as this literal marker. Such a plan legitimately cites no SC-IDs, so
# the checks below would warn on every one of them.
if grep -q 'Coverage: waived' "$plan_file" 2>/dev/null; then
    exit 0
fi

# Correlate the plan with its OWN coverage rows, via the SC-IDs it cites. `writing-plans` is
# told by test-coverage-design to cite them in the plan's test steps, so the plan names what
# must exist. The alternative -- matching a filename slug -- guesses a naming convention
# nothing enforces, and a slug that drifts between the two documents warns on a compliant
# branch. The earlier "any *-coverage.md exists" check was silent from a repo's second
# feature onward, including for a plan that genuinely skipped the stage.
#
# Reading the plan from disk, not from the payload: PostToolUse fires after the write, so the
# file is there, and this stays correct for Write and for any future edit-shaped tool.
#
# Two digits, not `[0-9]+`, because the documented ID format is `SC-nn`. The looser pattern
# also matched inside a shell fragment a plan happened to quote -- `grep -E '^\| SC-0[235]'`
# yields a bare `SC-0`, which nothing defines and which then warned on a compliant plan. That
# was the only false fire across all 723 plans on this machine; see docs/enforcement.md.
plan_ids=$(grep -oE '\bSC-[0-9]{2,}\b' "$plan_file" 2>/dev/null | sort -u)

if [ -z "$plan_ids" ]; then
    reason="the plan cites no SC-IDs"
    looked="Looked for: SC-nn citations in the plan's own test steps"
else
    # Specs are a defining source too: a feature meeting test-coverage-design's floor rule
    # (<=2 behaviors, no persistence, no concurrency, no external dependency) keeps its
    # coverage table inline in the spec instead of a coverage doc. Omitting specs here would
    # warn on every branch that followed that rule correctly.
    defined=$(grep -hoE '\bSC-[0-9]{2,}\b' \
        "$root"/docs/superpowers/coverage/*-coverage.md \
        "$root"/docs/superpowers/specs/*.md 2>/dev/null | sort -u)
    undefined=$(comm -23 <(printf '%s\n' "$plan_ids") <(printf '%s\n' "$defined"))
    [ -n "$undefined" ] || exit 0
    reason="the plan cites SC-IDs that nothing defines: $(printf '%s' "$undefined" | tr '\n' ' ')"
    looked="Looked in: $root/docs/superpowers/coverage/*-coverage.md
           $root/docs/superpowers/specs/*.md"
fi

cat >&2 <<EOF
test-design: this plan is not backed by a coverage design.

Wrote:  $file_path
Reason: $reason
$looked

The sequence is brainstorming -> test-design:test-coverage-design -> writing-plans, so a
plan normally arrives after the coverage rows exist and its test steps cite their SC-IDs.
Two cases where this warning is expected and you should carry on:

  - the rows live somewhere this hook does not look: a package-relative coverage doc in a
    monorepo, or a floor-rule table inline in a spec outside docs/superpowers/specs/
  - the plan is a stub whose test steps are still to be written

If the user declined coverage design at the brainstorming seam, record the waiver in the
plan as the literal line \`Coverage: waived (user decision, YYYY-MM-DD)\` and this hook
stays silent. Otherwise the stage was skipped: run test-design:test-coverage-design against
the approved spec, then have the plan cite its SC-IDs.
EOF
exit 2
