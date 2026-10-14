// OpenCode adapter for the test-design plugin. Link this file into
// ~/.config/opencode/plugins/ (see README "OpenCode"); everything else is read
// from the repository it lives in, so a `git pull` is the whole update.
import { readdirSync, readFileSync } from "node:fs";
import { basename, join } from "node:path";
import { agentConfig, guardedSkills, pluginRoot, worktreePatterns } from "./adapter.js";

export const TestDesign = async () => {
  const root = pluginRoot();
  const skillsDir = join(root, "skills");
  const agentsDir = join(root, "agents");
  const guarded = guardedSkills(skillsDir);

  return {
    // Agents register under their file names; a user's own `agent.<name>`
    // settings (a model, say) win key by key over the translated defaults.
    config: async (cfg) => {
      cfg.agent ??= {};
      for (const file of readdirSync(agentsDir).filter((f) => f.endsWith(".md"))) {
        const name = basename(file, ".md");
        const ours = agentConfig(readFileSync(join(agentsDir, file), "utf8"), worktreePatterns());
        cfg.agent[name] = { ...ours, ...cfg.agent[name] };
      }
      const paths = cfg.skills?.paths ?? [];
      cfg.skills = { ...cfg.skills, paths: paths.includes(skillsDir) ? paths : [...paths, skillsDir] };
    },

    // The skills resolve the plugin root from this before any `go run -C`.
    "shell.env": async (_input, output) => {
      output.env.TEST_DESIGN_PLUGIN_ROOT = root;
    },

    // OpenCode 1.19 ignores `disable-model-invocation`; refuse the model's own
    // load so the skills stay opt-in. A typed slash command bypasses the tool.
    "tool.execute.before": async (input, output) => {
      const name = output.args?.name;
      if (input.tool === "skill" && guarded.has(name)) {
        throw new Error(
          `${name} is opt-in: only the user starts it, by typing /${name}. Ask them to run it.`,
        );
      }
    },
  };
};
