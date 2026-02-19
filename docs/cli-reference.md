# CLI Reference (Curated)

This guide maps Sophia command families for everyday operation. Use help output for full flag details:

```bash
sophia --help
sophia cr --help
sophia cr <command> --help
```

## Root Commands

- `sophia init`
  - Initialize Sophia metadata and policy templates.
- `sophia doctor`
  - Run workflow integrity diagnostics.
- `sophia log`
  - View intent-first CR history.
- `sophia repair`
  - Rebuild metadata from Git history.
- `sophia hook install`
  - Install local Git guardrails.

## CR Lifecycle

Create and navigate CR context:

```bash
sophia cr add "<title>" --description "<why>"
sophia cr switch <cr-id>
sophia cr current
sophia cr list
sophia cr search "<query>"
```

Contract and tasks:

```bash
sophia cr contract set <cr-id> --why "..." --scope .
sophia cr task add <cr-id> "<task title>"
sophia cr task contract set <cr-id> <task-id> --intent "..." --acceptance "..." --scope <prefix>
```

Task completion/checkpoint:

```bash
sophia cr task done <cr-id> <task-id> --from-contract
# alternatives:
sophia cr task done <cr-id> <task-id> --path <file>
sophia cr task done <cr-id> <task-id> --patch-file <patch-manifest>
sophia cr task done <cr-id> <task-id> --all
```

Readiness and merge:

```bash
sophia cr status <cr-id>
sophia cr validate <cr-id>
sophia cr review <cr-id>
sophia cr merge <cr-id>
```

Collaboration artifacts:

```bash
# canonical CR bundle export
sophia cr export <cr-id> --format json --out cr.bundle.json

# import bundle into local metadata
sophia cr import --file cr.bundle.json --mode create
sophia cr import --file cr.bundle.json --mode replace

# apply/preview structured collaboration patches
sophia cr patch apply <cr-id-or-uid> --file cr.patch.json
sophia cr patch preview <cr-id-or-uid> --file cr.patch.json --json
```

## Merge Recovery

When a merge stops due to conflicts:

```bash
sophia cr merge status <cr-id>
# resolve conflicts in working tree
sophia cr merge resume <cr-id>
# or abandon merge flow
sophia cr merge abort <cr-id>
```

## Stacks and Base Management

```bash
sophia cr child add "<title>" --description "<why>"
sophia cr stack
sophia cr base set <cr-id> --ref <git-ref>
sophia cr restack <cr-id>
sophia cr refresh <cr-id>
```

## Machine-Readable Outputs

Use `--json` for deterministic agent/tooling integration:

```bash
sophia cr status <cr-id> --json
sophia cr validate <cr-id> --json
sophia cr review <cr-id> --json
sophia cr import --file cr.bundle.json --mode create --json
sophia cr patch apply <cr-id-or-uid> --file cr.patch.json --json
sophia cr patch preview <cr-id-or-uid> --file cr.patch.json --json
sophia doctor --json
```

## Related Docs

- Lifecycle details: [`workflow.md`](workflow.md)
- Policy model: [`repository-policy.md`](repository-policy.md)
- First-run setup: [`getting-started.md`](getting-started.md)
