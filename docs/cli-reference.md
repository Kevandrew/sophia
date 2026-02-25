# CLI Reference

Use this page as a command map, not a tutorial.

If you are starting or implementing day-to-day work, go here first:

- Agent onboarding: [`agent-quickstart.md`](agent-quickstart.md)
- Author lifecycle: [`workflow.md`](workflow.md)

For exact flags, always use `--help`:

```bash
sophia --help
sophia cr --help
sophia cr <command> --help
```

## Root command map

- `sophia init` initialize repository metadata.
- `sophia version` print version, commit, and build date.
- `sophia doctor` run workflow integrity diagnostics.
- `sophia log` inspect intent-first history.
- `sophia repair` rebuild metadata from Git history.
- `sophia hook install` install local Git guardrails.

## CR command families

Navigation:

```bash
sophia cr current
sophia cr switch <cr-id>
sophia cr list
sophia cr search "<query>"
sophia cr status [<cr-id>|<cr-uid>]
sophia cr status [<cr-id>|<cr-uid>] --hq --json
```

Planning:

```bash
sophia cr add "<title>" --description "<why>"
sophia cr contract set <cr-id> --why "..." --scope <prefix>
sophia cr task add <cr-id> "<task>"
sophia cr task contract set <cr-id> <task-id> --intent "..." --acceptance "..." --scope <prefix>
```

Implementation and checkpoints:

```bash
sophia cr task done <cr-id> <task-id> --path <file> --path <file>
sophia cr task chunk list <cr-id> <task-id>
sophia cr task chunk export <cr-id> <task-id> --chunk <chunk-id> --out task.patch
sophia cr task done <cr-id> <task-id> --patch-file task.patch
```

Validation/review/merge:

```bash
sophia cr validate [<cr-id>|<cr-uid>]
sophia cr review [<cr-id>|<cr-uid>]
sophia cr merge <cr-id>
```

Merge recovery:

```bash
sophia cr merge status <cr-id>
sophia cr merge resume <cr-id>
sophia cr merge abort <cr-id>
```

Archive artifacts:

```bash
sophia cr archive write <cr-id>
sophia cr archive append <cr-id> --reason "Correction rationale"
sophia cr archive backfill
sophia cr archive backfill --commit
```

Archive behavior:

- `write` writes the next append-only revision (`vN`) for a merged CR.
- `append` writes `vN+1` and records the provided reason in the archive header.
- `backfill` is dry-run by default; add `--commit` to write missing `v1` archives and create one commit.

Collaboration artifacts:

```bash
sophia cr export <cr-id> --format json --out cr.bundle.json
sophia cr import --file cr.bundle.json --mode create
sophia cr patch preview <cr-id-or-uid> --file cr.patch.json --json
sophia cr patch apply <cr-id-or-uid> --file cr.patch.json
sophia cr push [<id|uid>] [--force]
sophia cr pull [<id|uid>] [--force]
sophia cr sync [<id|uid>] [--force]
```

## JSON surfaces

```bash
sophia cr status [<cr-id>|<cr-uid>] --json
sophia cr validate [<cr-id>|<cr-uid>] --json
sophia cr review [<cr-id>|<cr-uid>] --json
sophia doctor --json
```
