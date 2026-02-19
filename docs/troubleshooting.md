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
| `task_contract_incomplete` | Task missing required contract fields. | `sophia cr task contract set <cr-id> <task-id> ...` |
| `pre_staged_changes` | Index already has staged files before checkpointing. | Unstage first, then retry `task done` with explicit scope |
| `no_task_scope_matches` | Selected completion mode found no eligible files. | Use `--path`/`--patch-file`, or update task scope |
| `merge_in_progress` | Mutating command blocked during unresolved merge. | `sophia cr merge status <cr-id>` then `resume`/`abort` |
| `validation_failed` | Contract/policy or change validation failed. | Run `sophia cr validate <cr-id>` and address required items |
| `not_found` | Requested CR/task/entity missing. | Verify ID/selector via `sophia cr list` / `sophia cr search` |

## Recovery patterns

- Stuck after conflict: use `merge status`, resolve files, then `merge resume`.
- Bad local metadata after manual Git operations: run `repair`, then `cr status <id>`.
- Checkpoint mismatch or orphan suspicion: use `cr reconcile <id>`, then `cr review <id>`.

## Useful machine-readable checks

```bash
sophia doctor --json
sophia cr status <cr-id> --json
sophia cr check status <cr-id> --json
sophia cr validate <cr-id> --json
```
