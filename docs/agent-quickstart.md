# Agent Quickstart

This is the day-1 path: install Sophia, install the Sophia skill, then let the agent drive the normal CR lifecycle.

## 1) Install Sophia (primary path)

```bash
curl -fsSL https://sophiahq.com/install.sh | bash
sophia version
```

Expected: `sophia version` prints version, commit, and build date metadata.

## 2) Install the Sophia skill from this repo

Skill source in this repository:

- `skills/sophia/SKILL.md`

### Codex skill install (local)

```bash
mkdir -p "$CODEX_HOME/skills/sophia"
cp skills/sophia/SKILL.md "$CODEX_HOME/skills/sophia/SKILL.md"
```

### Other agent environments

Copy `skills/sophia/SKILL.md` into the agent's local skills directory and enable it for the session.

## 3) Use prompts that trigger Sophia workflows

Example prompts to your agent:

- "Create a CR for adding retry jitter to outbound API calls. Keep scope in `internal/service`."
- "Set the CR contract with test and rollback plans, then split the work into checkpointable tasks."
- "Implement task 2 and checkpoint it with explicit file scope."
- "Run validate and review, then summarize blockers."

## 4) What Sophia should do

When the skill is active, a normal implementation flow should produce:

1. A CR with intent (`why`) and explicit scope.
2. Tasks with task contracts (`intent`, `acceptance`, `scope`).
3. Task checkpoints created with explicit scope (`--path` or `--patch-file`).
4. Evidence entries for required command runs when contracts call for them.
5. `cr validate` and `cr review` results before merge.

## Where To Look Next

- First-success walkthrough: [`getting-started.md`](getting-started.md)
- Daily author loop: [`workflow.md`](workflow.md)
- Recovery and stale-state handling: [`troubleshooting.md`](troubleshooting.md)
- Full docs map: [`index.md`](index.md)
