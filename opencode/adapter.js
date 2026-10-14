// Translates the plugin's Claude Code surfaces (agents/*.md, skills/*/SKILL.md)
// into OpenCode configuration. Kept apart from plugin.js because OpenCode calls
// every export of a plugin module as a plugin.
import { existsSync, readdirSync, readFileSync, realpathSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// Claude Code tool allowlist entry -> OpenCode permissions it needs. An agent
// naming a tool missing here fails loudly instead of silently losing it.
const TOOL_PERMISSIONS = {
  Read: ["read", "list"],
  Grep: ["grep"],
  Glob: ["glob"],
  // OpenCode's edit permission also governs write and patch; 1.19 cannot split
  // them, so the mutator may create files where Claude's Edit could not.
  Edit: ["edit"],
};

/** @returns {string} absolute repository root, symlinks resolved */
export function pluginRoot() {
  return dirname(dirname(realpathSync(fileURLToPath(import.meta.url))));
}

/**
 * Splits a Markdown file's single-line `key: value` frontmatter from its body.
 * @param {string} text
 * @returns {{ meta: Record<string, string>, body: string }}
 */
export function parseFrontmatter(text) {
  const match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n/.exec(text);
  if (!match) throw new Error("missing frontmatter");
  const meta = {};
  for (const line of match[1].split(/\r?\n/)) {
    const at = line.indexOf(":");
    if (at > 0) meta[line.slice(0, at).trim()] = line.slice(at + 1).trim();
  }
  return { meta, body: text.slice(match[0].length).replace(/^(\r?\n)+/, "") };
}

/**
 * The disposable worktrees the verifier opens under the OS temp directory, in
 * both the spelling the shell sees and the symlink-resolved one Go reports.
 * @returns {string[]}
 */
export function worktreePatterns() {
  const bases = new Set([tmpdir(), realpathSync(tmpdir())]);
  return [...bases].map((base) => join(base, "test-design-verifier-*"));
}

/**
 * OpenCode subagent config for one agents/*.md file. The Claude `model` alias
 * is dropped: OpenCode wants a provider/model id, and without one a subagent
 * runs on its caller's model.
 * @param {string} text agent file contents
 * @param {string[]} allowedDirs external directories the agent may touch
 */
export function agentConfig(text, allowedDirs) {
  const { meta, body } = parseFrontmatter(text);
  if (!meta.description) throw new Error("agent has no description");
  if (!meta.tools) throw new Error("agent has no tools line");
  const permission = { "*": "deny" };
  for (const tool of meta.tools.split(",").map((t) => t.trim())) {
    const granted = TOOL_PERMISSIONS[tool];
    if (!granted) throw new Error(`no OpenCode mapping for Claude tool ${tool}`);
    for (const key of granted) permission[key] = "allow";
  }
  permission.external_directory = { "*": "ask" };
  for (const dir of allowedDirs) permission.external_directory[dir] = "allow";
  return { description: meta.description, mode: "subagent", prompt: body, permission };
}

/**
 * Names of the skills under skillsDir whose frontmatter disables model
 * invocation: only the user may start them, by typing the slash command.
 * @param {string} skillsDir
 * @returns {Set<string>}
 */
export function guardedSkills(skillsDir) {
  const names = new Set();
  for (const name of readdirSync(skillsDir)) {
    const file = join(skillsDir, name, "SKILL.md");
    if (!existsSync(file)) continue;
    const { meta } = parseFrontmatter(readFileSync(file, "utf8"));
    if (meta["disable-model-invocation"] === "true") names.add(meta.name ?? name);
  }
  return names;
}

/**
 * Top-level permission letting every agent reach dirs outside the project the
 * skills need: the verifier's temp dirs (each run's payloads) and the
 * plugin's agents/ (the instruction files a dispatch cites). A blanket string
 * policy is the user's call and stays as is; a pattern the user set keeps the
 * user's action.
 * @param {string | Record<string, unknown> | undefined} permission
 * @param {string[]} dirs
 */
export function withExternalAccess(permission, dirs) {
  if (typeof permission === "string") return permission;
  const current = permission?.external_directory;
  const external = typeof current === "string" ? { "*": current } : { ...current };
  for (const dir of dirs) external[dir] ??= "allow";
  return { ...permission, external_directory: external };
}
