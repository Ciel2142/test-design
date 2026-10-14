import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { agentConfig, guardedSkills, parseFrontmatter, pluginRoot, withExternalAccess, worktreePatterns } from "./adapter.js";
import { TestDesign } from "./plugin.js";

const agent = (frontmatter, body = "Body.\n") => `---\n${frontmatter}\n---\n\n${body}`;

test("parseFrontmatter splits single-line keys from the body", () => {
  const { meta, body } = parseFrontmatter(agent("name: x\ndescription: Does a: thing\ntools: Read, Grep"));
  assert.deepEqual(meta, { name: "x", description: "Does a: thing", tools: "Read, Grep" });
  assert.equal(body, "Body.\n");
});

test("parseFrontmatter accepts a CRLF checkout", () => {
  const { meta, body } = parseFrontmatter("---\r\ndescription: d\r\ntools: Read\r\n---\r\n\r\nBody.\r\n");
  assert.deepEqual(meta, { description: "d", tools: "Read" });
  assert.equal(body, "Body.\r\n");
});

test("parseFrontmatter rejects a file without frontmatter", () => {
  assert.throws(() => parseFrontmatter("# no frontmatter\n"), /frontmatter/);
});

test("agentConfig maps a read-only Claude tool list to an OpenCode subagent", () => {
  const cfg = agentConfig(agent("description: Reads.\ntools: Read, Grep, Glob\nmodel: sonnet"), ["/tmp/v-*"]);
  assert.equal(cfg.mode, "subagent");
  assert.equal(cfg.description, "Reads.");
  assert.equal(cfg.prompt, "Body.\n");
  assert.equal(cfg.model, undefined, "a Claude model alias is not an OpenCode provider/model id");
  assert.equal(cfg.permission["*"], "deny");
  for (const tool of ["read", "grep", "glob", "list"]) assert.equal(cfg.permission[tool], "allow");
  assert.equal(cfg.permission.edit, undefined, "edit stays under the deny-all default");
  assert.deepEqual(cfg.permission.external_directory, { "*": "ask", "/tmp/v-*": "allow" });
});

test("agentConfig grants edit only when the Claude tool list names Edit", () => {
  const cfg = agentConfig(agent("description: Edits.\ntools: Read, Grep, Glob, Edit"), []);
  assert.equal(cfg.permission.edit, "allow");
});

test("agentConfig refuses a Claude tool it has no mapping for", () => {
  assert.throws(() => agentConfig(agent("description: Runs.\ntools: Read, Bash"), []), /Bash/);
});

test("agentConfig refuses an agent without a description or tools line", () => {
  assert.throws(() => agentConfig(agent("tools: Read"), []), /description/);
  assert.throws(() => agentConfig(agent("description: x"), []), /tools/);
});

test("withExternalAccess adds allows without overriding the user", () => {
  assert.deepEqual(withExternalAccess(undefined, ["/t/v-*"]), { external_directory: { "/t/v-*": "allow" } });
  assert.deepEqual(withExternalAccess({ edit: "ask", external_directory: "deny" }, ["/t/v-*"]), {
    edit: "ask",
    external_directory: { "*": "deny", "/t/v-*": "allow" },
  });
  assert.deepEqual(
    withExternalAccess({ external_directory: { "/t/v-*": "ask", "/x/*": "allow" } }, ["/t/v-*", "/u/v-*"]),
    { external_directory: { "/t/v-*": "ask", "/x/*": "allow", "/u/v-*": "allow" } },
    "a pattern the user set keeps the user's action",
  );
  assert.equal(withExternalAccess("allow", ["/t/v-*"]), "allow", "a blanket policy is left alone");
});

test("guardedSkills names exactly the skills that disable model invocation", () => {
  const root = mkdtempSync(join(tmpdir(), "td-skills-"));
  const skill = (name, extra) => {
    mkdirSync(join(root, name));
    writeFileSync(join(root, name, "SKILL.md"), agent(`name: ${name}\n${extra}description: d`));
  };
  skill("manual", "disable-model-invocation: true\n");
  skill("auto", "");
  mkdirSync(join(root, "not-a-skill"));
  assert.deepEqual([...guardedSkills(root)], ["manual"]);
});

test("pluginRoot is the repository holding the verifier", () => {
  const root = pluginRoot();
  assert.match(readFileSync(join(root, ".claude-plugin", "plugin.json"), "utf8"), /"name": "test-design"/);
});

test("the plugin registers the repository's agents and skills without overriding user settings", async () => {
  const hooks = await TestDesign({});
  const cfg = { agent: { mutator: { model: "anthropic/claude-sonnet-4-5" } }, skills: { paths: ["/elsewhere"] } };
  await hooks.config(cfg);
  for (const name of ["coverage-critic", "live-runner", "mutator", "scenario-reviewer"]) {
    assert.equal(cfg.agent[name].mode, "subagent", name);
  }
  assert.equal(cfg.agent.mutator.model, "anthropic/claude-sonnet-4-5", "user override wins");
  assert.equal(cfg.agent.mutator.permission.edit, "allow");
  assert.deepEqual(cfg.skills.paths, ["/elsewhere", join(pluginRoot(), "skills")]);
  assert.equal(cfg.permission.external_directory[worktreePatterns()[0]], "allow");
  assert.equal(cfg.permission.external_directory[join(pluginRoot(), "agents", "*")], "allow");
  await hooks.config(cfg);
  assert.equal(cfg.skills.paths.length, 2, "a second config pass does not duplicate the path");
});

test("the plugin exports the plugin root into every shell", async () => {
  const hooks = await TestDesign({});
  const output = { env: {} };
  await hooks["shell.env"]({ cwd: "/" }, output);
  assert.equal(output.env.TEST_DESIGN_PLUGIN_ROOT, pluginRoot());
});

test("the plugin refuses a model-initiated load of an opt-in skill", async () => {
  const hooks = await TestDesign({});
  const before = hooks["tool.execute.before"];
  await assert.rejects(
    before({ tool: "skill" }, { args: { name: "coverage-verification" } }),
    /\/coverage-verification/,
  );
  await assert.rejects(before({ tool: "skill" }, { args: { name: "test-coverage-design" } }), /opt-in/);
  await before({ tool: "skill" }, { args: { name: "brainstorming" } });
  await before({ tool: "read" }, { args: { name: "coverage-verification" } });
});
