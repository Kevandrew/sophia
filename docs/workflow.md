# Author Workflow

This is the canonical day-to-day author loop for Sophia.

## Lifecycle

1. Open CR intent.
2. Define CR contract.
3. Add tasks and task contracts.
4. Implement and checkpoint each task with explicit scope.
5. Attach evidence when checks are required.
6. Validate, review, and merge.

## Start work

```bash
sophia cr add "<title>" --description "<why>"
sophia cr switch <cr-id>
```

## Define contract and tasks

```bash
sophia cr contract set [<cr-id>|<cr-uid>] \
  --why "..." \
  --scope <prefix> \
  --test-plan "go test ./... && go vet ./..." \
  --rollback-plan "Revert CR merge commit"

sophia cr task add [<cr-id>|<cr-uid>] "<task>"
sophia cr task contract set [<cr-id>|<cr-uid>] <task-id> \
  --intent "..." \
  --acceptance "..." \
  --scope <prefix>
```

## Checkpoint scope modes (decision guide)

Use exactly one completion scope mode per `task done` call:

- `--path` when changed files are known and explicit.
- `--patch-file` when only specific hunks/chunks should be checkpointed.
- `--from-contract` when contract scope is accurate and broad enough.
- `--all` only when full stage is intentionally required.
- `--no-checkpoint` only for metadata-only completion (must include reason).

Examples:

```bash
sophia cr task done [<cr-id>|<cr-uid>] <task-id> \
  --path internal/service/retry.go \
  --path internal/service/retry_test.go

sophia cr task done [<cr-id>|<cr-uid>] <task-id> --patch-file task.patch
sophia cr task done [<cr-id>|<cr-uid>] <task-id> --all
sophia cr task done [<cr-id>|<cr-uid>] <task-id> --no-checkpoint --no-checkpoint-reason "metadata-only"
```

## Chunk flow (pre-checkpoint)

Use chunk mode when you need hunk-level control:

```bash
sophia cr task chunk list <cr-id> <task-id> [--path <file>] [--json]
sophia cr task chunk show <cr-id> <task-id> <chunk-id> [--path <file>] [--json]
sophia cr task chunk export <cr-id> <task-id> --chunk <chunk-id> --out task.patch
sophia cr task done [<cr-id>|<cr-uid>] <task-id> --patch-file task.patch
```

Chunk commands inspect unstaged working-tree changes and require a clean index.

## Evidence and readiness

```bash
sophia cr validate [<cr-id>|<cr-uid>]
sophia cr review [<cr-id>|<cr-uid>]
sophia cr status [<cr-id>|<cr-uid>]
```

If contracts name specific checks, attach logs:

```bash
sophia cr evidence add [<cr-id>|<cr-uid>] \
  --type command_run \
  --summary "targeted tests" \
  --cmd "go test ./..." \
  --exit-code 0 \
  --attachment _docs/cr-<cr-id>-evidence/tests.log
```

## If you see X, do Y

- `task_contract_incomplete`: run `sophia cr task contract set [<cr-id>|<cr-uid>] <task-id> ...` and provide missing fields.
- `pre_staged_changes`: unstage first (`git restore --staged <file>`), then retry with explicit scope.
- `no_task_scope_matches`: use `--path` or `--patch-file` with actual changed files, or update task scope.
- `merge_in_progress`: use `sophia cr merge status <cr-id>`, then `merge resume` or `merge abort` before other mutations.

## Merge recovery

```bash
sophia cr merge status <cr-id>
# resolve conflicts
sophia cr merge resume <cr-id>
# or cancel
sophia cr merge abort <cr-id>
```

## Related docs

- First success walkthrough: [`getting-started.md`](getting-started.md)
- Reviewer flow: [`reviewer-workflow.md`](reviewer-workflow.md)
- Collaboration without HQ: [`collaboration.md`](collaboration.md)
