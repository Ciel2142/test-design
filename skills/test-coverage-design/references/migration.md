## Migration

A document whose scenario table still puts its mutation-naming column third and `Expected`
fifth — the pre-registry five-column order — is refused by the verifier's parser, which
names the old header it found in its error and reports that document as unparseable.
Before using these commands, use the absolute `TARGET_REPO` captured by the
[design skill](../SKILL.md#prerequisite). Migrating it:

1. **Rename and reorder the column.** Move the old third column — the one naming the
   mutation each row demonstrates — to fifth position and rename it `Verification
   mutation`; move `Expected` from fifth to third. The result is `| ID | Behavior |
   Expected | Level | Verification mutation | Risk |`, cell contents unchanged apart from
   the move — a row's mutation text does not need rewriting, only relocating.
2. **Add the `## Verification` block.** An old document captured its baseline by hand and
   carries no machine-readable runner configuration; add the block described
   [Verification block](../SKILL.md#verification-block), sourcing `runner-kind`, `runner-root`, and
   `runner-command` from however the project's suite is actually run today.
3. **Resolve a registry collision, if one exists.** The repository ID registry harvests IDs
   from both header forms so an unmigrated document's IDs stay visible to `next-id`, but two
   documents defining the same ID is a hard failure. If migration surfaces a pre-registry
   collision, renumber the newer document's rows once, in the same change as the rename,
   updating every `covers:` tag in its test source to match. This is the one exception to
   "never renumbered" — it applies only to a collision inherited from before the registry
   existed; every ID allocated through `next-id` from here on stays fixed.
4. **Prove acceptance.** Run:

   ```text
   go run -C "$CLAUDE_PLUGIN_ROOT" ./cmd/test-design-verifier preflight -source <repo-relative path> -repo "$TARGET_REPO"
   ```

   A successful `preflightOutput` — not a refusal — is the migration's done condition.

