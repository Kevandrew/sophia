# Collaboration Without HQ

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
