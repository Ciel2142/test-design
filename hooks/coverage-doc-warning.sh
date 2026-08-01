#!/usr/bin/env bash
# PostToolUse(Write) — warn when an implementation plan is written and no coverage doc
# exists beside it.
#
# Warning only, and structurally incapable of being anything else: PostToolUse runs after
# the write has already happened, so exit 2 feeds this text back to Claude without denying
# any tool call. That is the point. `docs/enforcement.md` defers a *denial* hook until the
# false-denial rate is measured; this one has no such rate to measure.
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

for f in "$root"/docs/superpowers/coverage/*-coverage.md; do
    [ -e "$f" ] && exit 0
done

cat >&2 <<EOF
test-design: no coverage doc found for this plan.

Wrote:    $file_path
Looked in: $root/docs/superpowers/coverage/*-coverage.md

The sequence is brainstorming -> test-design:test-coverage-design -> writing-plans, so a
plan normally arrives after a coverage doc exists and cites its SC-IDs. Two cases where
this warning is expected and you should carry on:

  - the feature meets test-coverage-design's floor rule (<=2 behaviors, no persistence, no
    concurrency, no external dependency), so its coverage table went inline into the spec
  - the coverage doc is package-relative in a monorepo, somewhere other than the repo root

Otherwise the stage was skipped. Run test-design:test-coverage-design against the approved
spec, then have the plan cite its SC-IDs.
EOF
exit 2
