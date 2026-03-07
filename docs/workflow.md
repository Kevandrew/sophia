# Author Workflow

This is the canonical day-to-day author loop for Sophia. Use [`getting-started.md`](getting-started.md) for the first successful run and [`troubleshooting.md`](troubleshooting.md) when local state is unhealthy.

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
sophia cr where <cr-id>
```

If the repository has not been explicitly initialized, `sophia cr add` bootstraps local metadata automatically.
Run `sophia init` when you want explicit initialization behavior (for example tracked mode setup).

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

### Updating CR scope before vs after first checkpoint

- Before first checkpoint freeze: update scope directly.
- After first checkpoint freeze: scope edits require an explicit reason and create a CR drift record that must be acknowledged.

```bash
# Before first checkpoint
sophia cr contract set [<cr-id>|<cr-uid>] --scope internal/service --scope internal/cli

# After first checkpoint (required reason)
sophia cr contract set [<cr-id>|<cr-uid>] \
  --scope internal/service \
  --scope internal/cli \
  --change-reason "Expanded scope for validated follow-up work"

# Review + acknowledge pending CR scope drifts
sophia cr contract drift list <cr-id>
sophia cr contract drift ack <cr-id> <drift-id> --reason "Accepted scope update"
```

## Checkpoint scope modes (decision guide)

Use exactly one completion scope mode per `task done` call:

- `--path` when changed files are known and explicit.
- `--patch-file` when only specific hunks/chunks should be checkpointed.
- `--from-contract` when contract scope is accurate and broad enough.
- `--all` only when full stage is intentionally required.
- `--no-checkpoint` only for metadata-only completion (must include reason).

Commit typing:

- Prefer explicit `--commit-type <type>` for checkpoint commits (`feat|fix|docs|refactor|test|chore|perf|build|ci|style|revert`).
- Resolution order when omitted: task title prefix -> task contract intent prefix -> `chore`.

Examples:

```bash
sophia cr task done [<cr-id>|<cr-uid>] <task-id> \
  --commit-type fix \
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

## Stacked parent / child workflow

Use this when one parent CR owns multiple independently reviewable child CRs.

Topology:
- Parent CR branch is the integration branch for the stack.
- Child CR branches are based on the parent branch.
- Child PRs target the parent branch.
- Parent PR targets `main`.

Typical sequence:
1. Open the parent CR and define the parent contract.
2. Create child CRs with `--parent` (or `sophia cr child add`) for each reviewable slice.
3. Delegate parent tasks to child CRs when the parent work is fulfilled in children.
4. Implement and merge children into the parent branch.
5. Refresh later children after earlier child merges so the stack stays current. Refreshing the parent CR is now the stack-wide entrypoint: parent refresh cascades through descendant child CRs, while refreshing an individual child CR remains local to that child:

```bash
sophia cr refresh <parent-cr-id>
sophia cr refresh <child-cr-id>
```

6. Reconcile remote merges into local metadata before further mutation:

```bash
sophia cr pr status <cr-id>
sophia cr status <cr-id> --json
```

Aggregate-parent semantics:
- A parent with delegated child tasks and no direct implementation checkpoints is an aggregate parent.
- Aggregate parents are valid integration CRs. They do not need fake implementation commits purely to become ready.
- Once delegated child CRs are merged, parent delegated tasks should reconcile into resolved state automatically.
- If state looks stale after remote child merges, reconcile or refresh before editing metadata by hand.

## If you see X, do Y

- `task_contract_incomplete`: run `sophia cr task contract set [<cr-id>|<cr-uid>] <task-id> ...` and provide missing fields.
- `pre_staged_changes`: unstage first (`git restore --staged <file>`), then retry with explicit scope.
- `no_task_scope_matches`: use `--path` or `--patch-file` with actual changed files, or update task scope.
- `merge_in_progress`: use `sophia cr merge status <cr-id>`, then `merge resume` or `merge abort` before other mutations.
- `branch_in_other_worktree`: run `sophia cr where <cr-id>` and execute the suggested command from the owner worktree path.

## Merge recovery

```bash
sophia cr merge status <cr-id>
# resolve conflicts
sophia cr merge resume <cr-id>
# or cancel
sophia cr merge abort <cr-id>
```

## PR-gated team flow (`merge.mode=pr_gate`)

Use this when authors cannot merge directly to `main` and reviewers merge from GitHub.

1. Finish local readiness:

```bash
sophia cr validate <cr-id>
sophia cr review <cr-id>
```

2. Publish/sync PR context:

```bash
sophia cr merge <cr-id>
```

Behavior in `pr_gate` mode:
- Pushes CR branch to `origin` if needed.
- Creates or syncs a draft PR and updates only Sophia-managed body section.
- Returns PR URL plus gate status (approvals/checks/draft readiness).
- Repository CI checks run on PR lifecycle events only after PR is non-draft (`ready_for_review` or non-draft open/sync events).

3. Optional explicit PR commands:

```bash
sophia cr pr context <cr-id>
sophia cr pr open <cr-id> --approve-open
sophia cr pr sync <cr-id>
sophia cr pr ready <cr-id>
sophia cr pr unready <cr-id>
sophia cr pr close <cr-id>
sophia cr pr reopen <cr-id>
sophia cr pr status <cr-id>
```

Lifecycle guidance:
- Use `pr ready` only for explicit reviewer handoff.
- `pr ready` can be blocked with `reason_code=pre_implementation_no_checkpoints` until at least one task checkpoint commit exists for the CR.
- Aggregate parents are the exception: if delegated child CRs provide the implementation proof and all delegated work is resolved, the parent can be ready without direct parent checkpoints.
- Keep pre-implementation PRs in draft; checkpoint implementation (`sophia cr task done ...`) before promoting ready-for-review.
- If marked ready too early, run `pr unready` to move back to draft.
- Use `pr close` when intentionally pausing/cancelling the PR without merging.
- Use `pr reopen` to resume a closed PR for the same CR branch.

4. Reviewer merges on GitHub PR page (common team path), or privileged user runs:

```bash
sophia cr merge finalize <cr-id>
```

5. Reconcile remote merge into local CR metadata:

```bash
sophia cr pr status <cr-id>
# or
sophia cr status <cr-id>
```

If PR is already merged remotely, Sophia records merged commit metadata and marks the CR merged locally.

## Related docs

- First success walkthrough: [`getting-started.md`](getting-started.md)
- Reviewer flow: [`reviewer-workflow.md`](reviewer-workflow.md)
- Collaboration without HQ: [`collaboration.md`](collaboration.md)
- Docs index: [`index.md`](index.md)
