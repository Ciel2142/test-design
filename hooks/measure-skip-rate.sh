#!/usr/bin/env bash
# Replay the warning hook's predicate over every superpowers plan under a root, so the
# skip rate that `docs/enforcement.md` bases the denial-hook decision on can be re-measured
# rather than re-argued.
#
#   hooks/measure-skip-rate.sh [search-root] [since-YYYY-MM-DD]
#
# Prints one TSV row per plan: repo, plan, created, cited, undefined, fires. `created` is the
# git add date where the plan is tracked, falling back to mtime — mtime alone dates a plan by
# its last edit, which pulls pre-stage plans into the measurement window and reads as a skip.
set -u

root=${1:-$HOME}
since=${2:-}

printf 'repo\tplan\tcreated\tcited\tundefined\tfires\n'

while IFS= read -r plan_dir; do
    repo_root=${plan_dir%/docs/superpowers/plans}
    for plan in "$plan_dir"/*.md; do
        [ -e "$plan" ] || continue

        created=$(git -C "$repo_root" log --diff-filter=A --format=%ad --date=short -1 \
                      -- "${plan#"$repo_root"/}" 2>/dev/null)
        [ -n "$created" ] || created=$(date -r "$plan" +%F)
        [ -z "$since" ] || [ "$created" ">" "$since" ] || [ "$created" = "$since" ] || continue

        # A recorded waiver is a decision, not a skip: the user declined the stage at the
        # brainstorming seam, so counting it as YES(no-ids) would inflate the skip rate the
        # denial-hook trip condition reads.
        if grep -q 'Coverage: waived' "$plan" 2>/dev/null; then
            printf '%s\t%s\t%s\t-\t-\twaived\n' \
                "${repo_root##*/}" "$(basename "$plan")" "$created"
            continue
        fi

        cited=$(grep -oE '\bSC-[0-9]{2,}\b' "$plan" 2>/dev/null | sort -u)
        if [ -z "$cited" ]; then
            printf '%s\t%s\t%s\t0\t-\tYES(no-ids)\n' \
                "${repo_root##*/}" "$(basename "$plan")" "$created"
            continue
        fi

        defined=$(grep -hoE '\bSC-[0-9]{2,}\b' \
            "$repo_root"/docs/superpowers/coverage/*-coverage.md \
            "$repo_root"/docs/superpowers/specs/*.md 2>/dev/null | sort -u)
        undefined=$(comm -23 <(printf '%s\n' "$cited") <(printf '%s\n' "$defined") | tr '\n' ' ')
        n=$(printf '%s\n' "$cited" | grep -c .)

        if [ -n "${undefined// /}" ]; then
            printf '%s\t%s\t%s\t%s\t%s\tYES(undefined)\n' \
                "${repo_root##*/}" "$(basename "$plan")" "$created" "$n" "$undefined"
        else
            printf '%s\t%s\t%s\t%s\t-\tno\n' \
                "${repo_root##*/}" "$(basename "$plan")" "$created" "$n"
        fi
    done
done < <(find "$root" -maxdepth 7 -type d -name plans -path '*superpowers*' \
              -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null | sort)
