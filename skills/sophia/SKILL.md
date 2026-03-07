---
name: sophia
description: Operate Sophia CLI as an intent-first abstraction over Git for local repositories. Use when the user wants to initialize Sophia, open/manage Change Requests (CRs), define CR and task contracts, checkpoint task progress with explicit scope, inspect impact/validation/review/status context, repair metadata, and merge safely.
---

# Sophia

Use Sophia as the primary workflow interface and Git as the execution engine.

## Core Rules

1. Open intent with a CR.
2. Set the CR contract and task contracts before implementation checkpoints.
3. Use explicit CR navigation for mutation commands, usually `sophia cr switch <id>`.
4. Complete implementation through task-scoped checkpoints, not ad-hoc commits.
5. Run `sophia cr validate <id>` and `sophia cr review <id>` before merge handoff.

Checkpoint rule for agents:
- Always pass `--commit-type <type>` to `sophia cr task done`.

## Repo-Owned Docs

Read the repo docs instead of relying on this skill as a manual:

- Docs map: `docs/index.md`
- Agent setup: `docs/agent-quickstart.md`
- First-success walkthrough: `docs/getting-started.md`
- Daily workflow: `docs/workflow.md`
- Recovery and stale-state handling: `docs/troubleshooting.md`
- Policy details: `docs/repository-policy.md`

## Merge Mode Decision

Read `SOPHIA.yaml` before choosing merge behavior.

- If `merge.mode=local`, use the normal local merge lifecycle after validate/review.
- If `merge.mode=pr_gate`, treat PR publication and remote merge as part of the workflow.
- Do not open/sync PRs or push remote state unless the user asked for it.

Aggregate parent rule:
- A parent CR may be an integration branch whose implementation is fulfilled entirely by delegated child CRs.
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
4. If output shows stale or blocked state, prefer the suggested commands from CLI output before reaching for manual Git/gh steps.
