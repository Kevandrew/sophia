# Collaboration

Sophia supports two collaboration modes:

- Managed remote collaboration (intent push/pull)
- Platform-agnostic collaboration (portable bundles + patches)

## Collaboration With Managed Remote

This mode treats the managed remote as canonical for intent/discussion, while Git remains canonical for code/branches.

### Setup

Configure the repo identity and (optionally) base URL:

```bash
sophia hq config set --repo-id <org/repo>
# optional (defaults to https://sophiahq.com)
sophia hq config set --base-url https://sophiahq.com
```

Login (token storage is per-user):

```bash
printf '%s' "$SOPHIAHQ_TOKEN" | sophia hq login --token-stdin
```

### Daily loop (agent-first)

1) Ask for a deterministic sync snapshot:

```bash
sophia cr status <id|uid> --hq --json
```

2) Act based on `hq_sync.state`:

- `not_configured`: set `hq` config + login
- `remote_missing`: `sophia cr push <id|uid>`
- `unlinked`: `sophia cr pull <id|uid>` (links upstream intent)
- `local_ahead`: `sophia cr push <id|uid>`
- `remote_ahead`: `sophia cr pull <id|uid>`
- `diverged`: refuse by default; reconcile in the managed remote UI or use `--force` when intentional
- `up_to_date`: no action required

### Force semantics

- `sophia cr pull --force` accepts remote intent as canonical and overwrites local intent fields (local-only metadata remains intact).
- `sophia cr push --force` overwrites remote intent even if upstream moved.

### Common failure codes (JSON)

| Code | Meaning | Recommended next step |
|---|---|---|
| `hq_upstream_moved` | Remote intent advanced since last sync | `sophia cr pull <id|uid> --json` |
| `hq_intent_diverged` | Local + remote both changed since last sync | Reconcile in UI, then `sophia cr pull`; use `--force` only when intentional |
| `hq_patch_conflict` | Patch apply rejected due to before-mismatch or remote conflict | Pull latest remote intent, then retry push |
| `hq_malformed_response` | Remote response missing required doc fields | Stop and verify remote server/version |

## Collaboration Without HQ

Sophia supports platform-agnostic collaboration using portable CR bundles and structured patches.

## Flow A: Stateless share for review

Author exports local CR state:

```bash
sophia cr export <cr-id> --format json --out cr.bundle.json
```

Reviewer imports locally:

```bash
sophia cr import --file cr.bundle.json --mode create
```

If reviewer already has matching CR UID and wants upstream state replacement:

```bash
sophia cr import --file cr.bundle.json --mode replace
```

## Flow B: Suggestion patch with preview/apply

Preview first:

```bash
sophia cr patch preview <cr-id-or-uid> --file cr.patch.json --json
```

Apply when preview is clean:

```bash
sophia cr patch apply <cr-id-or-uid> --file cr.patch.json
```

## Conflict behavior

- Patch apply compares patch `before` values against current CR/task state.
- On mismatch, Sophia returns structured conflicts and does not mutate metadata.
- Use preview output to identify conflicting fields before trying apply.
- `--force` can bypass `before` mismatch when intentional, while recording warnings.

## Recommended collaboration protocol

1. Export CR bundle after major checkpoints.
2. Reviewer imports and runs `validate` + `review`.
3. Reviewer sends back structured patch suggestions.
4. Author runs patch preview, resolves conflicts, then applies.
5. Author re-runs `validate`, `review`, and merges.

## Related docs

- Reviewer checklist: [`reviewer-workflow.md`](reviewer-workflow.md)
- Troubleshooting: [`troubleshooting.md`](troubleshooting.md)
