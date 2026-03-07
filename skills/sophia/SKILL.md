---
name: sophia
description: Operate Sophia CLI as an intent-first abstraction over Git for local repositories. Use when the user wants to initialize Sophia, open/manage Change Requests (CRs), define CR and task contracts, checkpoint task progress with explicit scope, inspect impact/validation/review/status context, repair metadata, and merge safely.
---

# Sophia

Use Sophia as the primary workflow interface and Git as the execution engine.

This skill must stay self-sufficient. Do not assume repo-local docs exist wherever the skill is installed. The production guide surface is the CLI itself: help text, JSON output, and command behavior.

## Source Of Truth

Prefer discovery in this order:

1. `sophia --help`
2. `sophia cr --help`
3. `sophia cr <command> --help`
4. `sophia cr status <id> --json`
5. In `pr_gate` repos, `sophia cr pr status <id> --json`

If command output includes `next_steps`, `freshness`, `action_required`, or `suggested_commands`, treat that as the authoritative recovery and handoff guidance.

## Operating Principles

1. Open intent with a CR before implementation.
2. Write the CR contract before coding.
3. Break work into task-sized review units.
4. Set task contracts before checkpointing implementation.
5. Use Sophia task checkpoints instead of ad hoc commits.
6. Validate and review before merge handoff.

Checkpoint rule for agents:
- Always pass `--commit-type <type>` to `sophia cr task done`.

## Discoverability Map

Use this command map when you do not yet know the right leaf command:

- Start here:
  `sophia --help`
- Change-request command families:
  `sophia cr --help`
- Open or structure work:
  `cr add`, `cr child add`, `cr contract`, `cr task`
- Inspect current state:
  `cr status`, `cr why`, `cr review`, `cr validate`, `cr stack`
- Inspect diffs or checkpoints:
  `cr diff`, `cr range`, `cr rangediff`, `cr task diff`, `cr task chunk`
- Recover stale or unhealthy state:
  `cr refresh`, `cr reconcile`, `cr merge status`, `doctor`, `repair`
- PR-gated flow:
  `cr pr open`, `cr pr status`, `cr pr ready`, `cr merge finalize`

When in doubt:
- Use `status` for "what should I do next?"
- Use `review` for "is this ready?"
- Use `validate` for "what is structurally wrong?"

## Default Lifecycle

1. Open intent:
   `sophia cr add "<title>" --description "<why>"`

2. Switch into the CR branch:
   `sophia cr switch <id>`

3. Set the CR contract before coding:
   `sophia cr contract set <id> --why "..." --scope <prefix> ...`

4. Add tasks and task contracts:
   `sophia cr task add <id> "<task>"`
   `sophia cr task contract set <id> <task-id> --intent "..." --acceptance "..." --scope <prefix>`

5. Implement with task-scoped checkpoints:
   `sophia cr task done <id> <task-id> --commit-type <type> --from-contract`

6. Verify before merge handoff:
   `sophia cr validate <id>`
   `sophia cr review <id>`

## Good CR Authoring

CR contracts should be decision-complete enough that another engineer or agent can continue without guessing.

Good CRs have:
- outcome-first `why`
- explicit `scope`
- concrete `non_goals`
- concrete `invariants`
- explicit `blast_radius`
- exact `test_plan`
- exact `rollback_plan`

Smells:
- `why` says "implement X" instead of "achieve outcome Y"
- scope is vague or missing
- tasks are layer buckets instead of behavior slices
- no rollback or verification plan

## Good Task Authoring

Tasks should map cleanly to one checkpoint commit.

Good task contracts have:
- one behavior outcome in `intent`
- 2 to 6 observable acceptance criteria
- scope narrow enough for one checkpoint

Prefer behavior slices such as:
- "Define canonical bundle fingerprint"
- "Add merge blocker status JSON"
- "Surface next-step guidance in status output"

Avoid:
- "Update service layer"
- "Add tests" as one giant task

## Choosing Checkpoint Scope

Use exactly one scope mode per `task done` call:

- `--from-contract`:
  Use when the task contract scope is already correct.
- `--path`:
  Use when changed files are known explicitly.
- `--patch-file`:
  Use for hunk-level or chunk-level checkpointing.
- `--all`:
  Use only when full-stage behavior is intentional.
- `--no-checkpoint --no-checkpoint-reason`:
  Use only for metadata-only completion.

Examples:

```bash
sophia cr task done 25 1 --commit-type feat --from-contract
sophia cr task done 25 1 --commit-type fix --path internal/service/retry.go --path internal/service/retry_test.go
sophia cr task done 25 1 --commit-type refactor --patch-file /tmp/task.patch
```

## Current CLI Semantics

Assume these defaults unless `--help` says otherwise:

- `sophia cr add` and `sophia cr child add` do not switch branches by default.
  Use `--switch` or run `sophia cr switch <id>` before mutation commands.
- Read/context commands with explicit CR IDs are branch-agnostic.
- `sophia cr refresh <id>` is the canonical freshness sync abstraction.
- `sophia cr validate` is read-only by default; add `--record` only when an audit event is wanted.
- Status-oriented commands may expose `next_steps` and `freshness`; prefer those over manual playbooks.

## Merge Mode Decision

Read `SOPHIA.yaml` before choosing merge behavior.

- If `merge.mode=local`, use the normal local merge path after validate/review.
- If `merge.mode=pr_gate`, treat PR publication and remote merge as part of the workflow.
- Do not open/sync PRs or push remote state unless the user asked for it.

If `pr_gate`:
- keep draft PRs draft until reviewer handoff is intended
- use Sophia PR commands rather than raw `gh` first
- expect `cr status` and `cr pr status` to tell you the next action

## Stacked And Delegated Work

Use a parent CR when one intent needs multiple independently reviewable child CRs.

Rules:
- Parent branch is the integration branch.
- Child branches are based on the parent branch.
- Child CRs carry the real implementation slices.
- Parent may be an aggregate integration CR with delegated tasks.
- Do not create fake no-op parent checkpoints just to simulate progress.

Useful commands:

```bash
sophia cr child add "<title>"
sophia cr task delegate <parent-cr-id> <task-id> --child <child-cr-id>
sophia cr stack <id> --json
sophia cr refresh <id>
```

Interpretation rule:
- If a parent has delegated child tasks, the child CRs are usually the actionable work units.

## Verification And Evidence

Use contracts and evidence together.

When policy or task acceptance requires checks:
1. run the exact command
2. capture the output to a log file
3. attach it with `cr evidence add`

Example:

```bash
go test ./... 2>&1 | tee _docs/cr-25-evidence/go-test.log
sophia cr evidence add 25 \
  --type command_run \
  --summary "Full suite before merge" \
  --cmd "go test ./..." \
  --exit-code 0 \
  --attachment _docs/cr-25-evidence/go-test.log
```

## Restart / Resume Protocol

When resuming an existing CR:

1. `sophia cr switch <id>`
2. `sophia cr status <id> --json`
3. If the repo is `pr_gate`, also run `sophia cr pr status <id> --json`
4. If output shows stale or blocked state, prefer `next_steps.suggested_commands` and `freshness` guidance before manual Git or `gh` steps

## If You See X, Do Y

- `task_contract_incomplete`:
  Run `sophia cr task contract set <id> <task-id> ...`
- `pre_staged_changes`:
  Unstage first, then retry `task done`
- `no_task_scope_matches`:
  Use `--path` or `--patch-file`, or update task scope
- `merge_in_progress`:
  Use `sophia cr merge status <id>`, then `resume` or `abort`
- `branch_in_other_worktree`:
  Use `sophia cr where <id>` and follow the suggested command
- stale base or stale stack:
  Use `sophia cr refresh <id>`
- no linked PR in `pr_gate`:
  Use `sophia cr pr open <id> --approve-open` only if the user actually wants remote publication

## Agent Behavior Rules

- Prefer Sophia commands over raw Git for workflow state.
- Prefer JSON output when coordinating next actions.
- Prefer explicit CR IDs for reads.
- Use `sophia cr switch <id>` before writes.
- Keep user narration intent-first: what changed, what is blocked, what the next command is.
