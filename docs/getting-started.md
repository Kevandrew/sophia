# Getting Started

This walkthrough is the first-success lifecycle for agent-first Sophia usage.

Use this mental model:
- your agent executes most commands,
- you review intent, scope, evidence, and merge readiness.

## Prerequisites

- `curl -fsSL https://sophiahq.com/install.sh | bash`
- a Git repository
- an agent session with the Sophia skill enabled (see [`agent-quickstart.md`](agent-quickstart.md))

## 1) Open intent (no explicit init required)

```bash
sophia cr add "Add jittered retries for outbound API calls" \
  --description "Reduce transient failure retries without overloading providers"
sophia cr switch <cr-id>
```

On first use in an uninitialized Git repository, Sophia lazily bootstraps local metadata.
Use `sophia init` only when you want explicit setup behavior (for example tracked metadata mode).

## 2) Optional explicit initialization

```bash
sophia init
```

Expected outcome:
- Explicit metadata/policy setup is created.
- Existing no-init workflow remains available.

## 3) Set CR contract before coding

```bash
sophia cr contract set [<cr-id>|<cr-uid>] \
  --why "Lower transient incident rate while preserving request ordering guarantees" \
  --scope internal/service \
  --test-plan "go test ./... && go vet ./..." \
  --rollback-plan "Revert CR merge commit"
```

Expected outcome:
- policy-required fields are complete,
- reviewer has outcome, scope boundary, and rollback plan before diffs exist.

## 4) Decompose into tasks with task contracts

```bash
sophia cr task add [<cr-id>|<cr-uid>] "Implement bounded jitter strategy"
sophia cr task contract set [<cr-id>|<cr-uid>] <task-id> \
  --intent "Bound retry jitter and preserve deterministic backoff floor" \
  --acceptance "Retries stay in configured bounds and pass existing integration tests" \
  --scope internal/service
```

Expected outcome:
- tasks become checkpoint units,
- each task has explicit acceptance and scope.

## 5) Implement and checkpoint with explicit scope

```bash
sophia cr task done [<cr-id>|<cr-uid>] <task-id> \
  --commit-type feat \
  --path internal/service/retry.go \
  --path internal/service/retry_test.go
```

For hunk-level checkpoints:

```bash
sophia cr task chunk list <cr-id> <task-id>
sophia cr task chunk export <cr-id> <task-id> --chunk <chunk-id> --out task.patch
sophia cr task done [<cr-id>|<cr-uid>] <task-id> --patch-file task.patch
```

Expected outcome:
- a checkpoint commit is created,
- scope is explicit and auditable.
- commit type is explicit and agent-reviewable in history.

## 6) Attach evidence when contracts call for specific checks

If acceptance criteria or test plans require targeted commands, persist the proof:

```bash
go test ./... 2>&1 | tee _docs/cr-<cr-id>-evidence/tests.log

sophia cr evidence add [<cr-id>|<cr-uid>] \
  --type command_run \
  --summary "Full suite before merge" \
  --cmd "go test ./..." \
  --exit-code 0 \
  --attachment _docs/cr-<cr-id>-evidence/tests.log
```

Expected outcome:
- evidence chain links intent, checkpoint, and verification outputs.

## 7) Validate, review, and merge

```bash
sophia cr validate [<cr-id>|<cr-uid>]
sophia cr review [<cr-id>|<cr-uid>]
sophia cr status [<cr-id>|<cr-uid>]
sophia cr merge <cr-id>
```

Expected outcome:
- validation errors are zero,
- review required actions are addressed,
- CR merges cleanly.

If merge conflicts occur:

```bash
sophia cr merge status <cr-id>
# resolve conflicts
sophia cr merge resume <cr-id>
# or cancel
sophia cr merge abort <cr-id>
```

## Where to go next

- Canonical author UX: [`workflow.md`](workflow.md)
- Reviewer checklist: [`reviewer-workflow.md`](reviewer-workflow.md)
- Collaboration without HQ: [`collaboration.md`](collaboration.md)
- Troubleshooting and repair: [`troubleshooting.md`](troubleshooting.md)

For CLI updates:

```bash
sophia update --check
sophia update --yes
```
