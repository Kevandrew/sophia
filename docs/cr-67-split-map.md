# CR-67 Split Map: Capability-Oriented Service/CLI Decomposition

## Purpose

This document is the decision-complete execution map for CR-67:
"Tech debt: split large service/cli files into capability directories (scoping-first)".

Goal: reduce very large `internal/service/*` and `internal/cli/*` files by moving behavior into capability-scoped directories without changing runtime behavior, CLI semantics, JSON output, or policy/merge guardrails.

## Non-goals

- No user-facing workflow changes.
- No JSON schema/key changes.
- No policy interpretation changes.
- No merge gate/trust scoring logic changes.

## Invariants

- Existing command names/flags/exit behavior remain stable.
- Existing CR/task YAML semantics remain stable.
- Existing review/validation/trust outcomes remain functionally equivalent for unchanged repository state.
- Refactor steps are checkpointed by capability scope; each step must be independently revertible.

## Target Ownership Boundaries

### Service layer (`internal/service`)

| Capability directory | Owns | Current anchor files |
|---|---|---|
| `internal/service/cr` | CR lifecycle orchestration, CR contract updates, CR metadata mutation helpers | `service_cr.go` |
| `internal/service/diff` | diff summarization, base/head/merge-base anchors, range/rangediff plumbing | `service_diff.go` |
| `internal/service/trust` | trust scoring, review depth, trust requirements/report composition | `service_trust.go` |
| `internal/service/policy` | `SOPHIA.yaml` parsing, defaults, normalization, policy checks | `service_policy.go` |
| `internal/service/merge` | merge/recovery orchestration (`merge`, `status`, `resume`, `abort`) | `service_review_merge.go` |
| `internal/service/tasks` | task lifecycle, contracts, chunk/checkpoint orchestration | `service_tasks.go`, `service_task_checkpoint.go` |
| `internal/service/collab` | export/import/patch and HQ sync surfaces | `service_collab.go`, `service_hq.go`, `service_hq_sync.go` |

### CLI layer (`internal/cli`)

| Capability directory | Owns | Current anchor files |
|---|---|---|
| `internal/cli/cr` | CR command constructor families and command wiring | `cr_cmd_core.go`, `cr_cmd_task.go`, related `cr_cmd_*` |
| `internal/cli/json` | JSON envelope/error mapping and CR JSON mapper helpers | `json.go`, `cr_json_mappers.go` |

## Incremental Sequence (Execution Order)

Each step maps directly to CR-67 tasks and should be checkpointed as one task-sized unit.

1. Task 1: author this split map and sequencing doc.
2. Task 2: extract CR lifecycle into `internal/service/cr`.
3. Task 3: extract diff/anchor logic into `internal/service/diff`.
4. Task 4: extract trust/review-depth logic into `internal/service/trust`.
5. Task 5: extract policy parsing/normalization into `internal/service/policy`.
6. Task 6: extract merge/recovery orchestration into `internal/service/merge`.
7. Task 7: extract task lifecycle/checkpoint logic into `internal/service/tasks`.
8. Task 8: extract collab/HQ sync into `internal/service/collab`.
9. Task 9: split CR command wiring into `internal/cli/cr`.
10. Task 10: split JSON mappers/envelope helpers into `internal/cli/json`.
11. Task 11: targeted safety-net tests for wiring/output stability after moves.

## Per-Step Rollback Strategy

For every step above:

1. Keep changes scoped to the task contract paths.
2. Verify with targeted tests for touched capabilities plus `go test ./... && go vet ./...`.
3. If regressions appear:
   - immediate rollback: revert that task checkpoint commit on the CR branch;
   - if needed, re-open task and re-apply with narrower file/hunk scope.
4. Do not continue to the next extraction step until the current step is green.

CR-level rollback remains: revert `[CR-67]` merge commit if already merged.

## Review and Checkpoint Rules

- Prefer one checkpoint commit per task.
- Use explicit task scope (`--from-contract` when possible, else explicit `--path` list).
- Keep moved behavior and tests in the same task where practical.
- Avoid cross-capability edits in a single task to preserve auditability and easy revert.

## Exit Criteria

CR-67 is complete when:

- all target capabilities above are extracted into their directories,
- all existing behavior is preserved (no intentional runtime changes),
- targeted + full-suite checks pass,
- `sophia cr review 67` reports no required actions.
