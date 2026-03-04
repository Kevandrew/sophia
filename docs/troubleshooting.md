# Troubleshooting and Recovery

Use this guide when local metadata, CR linkage, or merge state is unhealthy.

## Fast triage

1. Check repository + workflow health:

```bash
sophia doctor
```

2. Repair metadata from Git history when state is missing/out-of-sync:

```bash
sophia repair
```

3. Reconcile one CR against commit graph when checkpoint linkage drifts:

```bash
sophia cr reconcile <cr-id>
```

4. If merge is in progress or conflicted:

```bash
sophia cr merge status <cr-id>
# resolve conflicts
sophia cr merge resume <cr-id>
# or cancel
sophia cr merge abort <cr-id>
```

## Common JSON error codes

| `error.code` | Meaning | Remediation |
| --- | --- | --- |
| `no_active_cr_context` | Current branch is not active CR context for mutation. | `sophia cr switch <id>` |
| `task_contract_incomplete` | Task missing required contract fields. | `sophia cr task contract set [<cr-id>|<cr-uid>] <task-id> ...` |
| `pre_staged_changes` | Index already has staged files before checkpointing. | Unstage first, then retry `task done` with explicit scope |
| `no_task_scope_matches` | Selected completion mode found no eligible files. | Use `--path`/`--patch-file`, or update task scope |
| `merge_in_progress` | Mutating command blocked during unresolved merge. | `sophia cr merge status <cr-id>` then `resume`/`abort` |
| `branch_in_other_worktree` | Target CR branch is checked out in a different linked worktree. | `sophia cr where <id>` then run the `suggested_command` from details |
| `validation_failed` | Contract/policy or change validation failed. | Run `sophia cr validate [<cr-id>|<cr-uid>]` and address required items |
| `pr_open_approval_required` | Agent/user approval needed before creating/opening PR in `pr_gate` mode. | Re-run with `--approve-pr-open` (merge) or `--approve-open` (`cr pr open`) |
| `gh_auth_required` | `gh` CLI is not authenticated for PR operations. | Run `gh auth login`, then retry |
| `pr_permission_denied` | Authenticated identity lacks permission for requested PR action. | Ask reviewer/maintainer to perform action on GitHub PR |
| `push_permission_denied` | `origin` rejected branch push during PR publish/sync. | Request push access or push via authorized credential |
| `not_found` | Requested CR/task/entity missing. | Verify ID/selector via `sophia cr list` / `sophia cr search` |

## Recovery patterns

- Stuck after conflict: use `merge status`, resolve files, then `merge resume`.
- Bad local metadata after manual Git operations: run `repair`, then `cr status [<id>|<uid>]`.
- Checkpoint mismatch or orphan suspicion: use `cr reconcile <id>`, then `cr review [<id>|<uid>]`.
- `gh` auth failure: run `gh auth status` and `gh auth login`, then retry `cr merge`/`cr pr` command.
- Missing `origin` or push denied: verify remote with `git remote -v`, then `git push -u origin <cr-branch>` and retry.
- Gate blocked in `pr_gate` mode: inspect `sophia cr pr status <id>` for approvals/checks/draft blockers.
- No CI checks shown on an open PR: confirm PR state is not draft. This repository's `CI` workflow is skipped while `pull_request.draft == true` and runs after `ready_for_review`.
- No merge permission for finalize: hand off to reviewer merge on PR page; run `sophia cr pr status <id>` afterward to reconcile local CR merged state.
- Worktree ownership conflict while switching/reopening: run `sophia cr where <id>` and use the returned `suggested_command` to route into the owner worktree.

## Useful machine-readable checks

```bash
sophia doctor --json
sophia cr status [<cr-id>|<cr-uid>] --json
sophia cr check status [<cr-id>|<cr-uid>] --json
sophia cr validate [<cr-id>|<cr-uid>] --json
```
