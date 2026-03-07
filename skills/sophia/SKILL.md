---
name: sophia
description: Operate Sophia CLI as an intent-first abstraction over Git for local repositories. Use when the user wants to initialize Sophia, open/manage Change Requests (CRs), define CR and task contracts, checkpoint task progress with explicit scope, inspect impact/validation/review/status context, repair metadata, and merge safely.
---

# Sophia

Use Sophia as the primary workflow interface and Git as the execution engine.

This skill must stay self-sufficient. Do not assume repo-local docs exist wherever the skill is installed. The production guide surface is the CLI itself.

## Source Of Truth

Prefer CLI help and command output over external prose:

1. `sophia --help`
2. `sophia cr --help`
3. `sophia cr <command> --help`
4. `sophia cr status <id> --json`
5. In `pr_gate` repos, `sophia cr pr status <id> --json`

If command output includes `next_steps` or `freshness`, treat that as the authoritative recovery/handoff guidance.

## Default Lifecycle

1. Open intent with a CR:
   `sophia cr add "<title>" --description "<why>"`
2. Switch into the CR branch:
   `sophia cr switch <id>`
3. Set the CR contract before coding:
   `sophia cr contract set <id> --why "..." --scope <prefix> ...`
4. Add tasks and task contracts before implementation checkpoints:
   `sophia cr task add <id> "<task>"`
   `sophia cr task contract set <id> <task-id> --intent "..." --acceptance "..." --scope <prefix>`
5. Implement with task-scoped checkpoints:
   `sophia cr task done <id> <task-id> --commit-type <type> --from-contract`
6. Verify before merge handoff:
   `sophia cr validate <id>`
   `sophia cr review <id>`

Checkpoint rule for agents:
- Always pass `--commit-type <type>` to `sophia cr task done`.

## Merge Mode Decision

Read `SOPHIA.yaml` before choosing merge behavior.

- If `merge.mode=local`, use the normal local merge path after validate/review.
- If `merge.mode=pr_gate`, treat PR publication and remote merge as part of the workflow.
- Do not open/sync PRs or push remote state unless the user asked for it.

Aggregate parent rule:
- A parent CR may be a pure integration branch whose implementation is fulfilled by delegated child CRs.
- Do not create fake no-op parent checkpoints just to simulate progress.

## Current CLI Semantics

- `sophia cr add` and `sophia cr child add` do not switch branches by default.
  Use `--switch` or run `sophia cr switch <id>` before mutation commands.
- Read/context commands with explicit CR IDs are branch-agnostic.
- `sophia cr refresh <id>` is the canonical freshness sync abstraction.
- `sophia cr validate` is read-only by default; add `--record` only when an audit event is wanted.

## Restart / Resume

When resuming an existing CR:

1. `sophia cr switch <id>`
2. `sophia cr status <id> --json`
3. If the repo is `pr_gate`, also run `sophia cr pr status <id> --json`
4. If output shows stale or blocked state, prefer `next_steps.suggested_commands` and `freshness` guidance from CLI output before manual Git or `gh` steps.
