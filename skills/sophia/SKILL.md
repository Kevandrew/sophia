# Sophia Skill

Use Sophia as the intent-first workflow layer over Git.

## Purpose

This skill is for agents and humans operating Change Requests (CRs) with explicit intent, scoped implementation, evidence, and deterministic merge readiness.

## Core Loop

1. Open intent with a CR.
2. Define CR contract (`why`, `scope`, safety and test fields required by policy).
3. Split work into small tasks.
4. Define each task contract (`intent`, `acceptance`, `scope`).
5. Implement and checkpoint each task with explicit scope (`--path` or `--patch-file`).
6. Record evidence for required checks (tests, validations, logs).
7. Validate and review (`cr validate`, `cr review`).
8. Merge (`cr merge`) or use merge recovery (`merge status/resume/abort`) when blocked.

## Daily Flow

```bash
sophia cr add "<title>" --description "<why>"
sophia cr switch <cr-id>
sophia cr contract set <cr-id> --why "..." --scope <prefix>
sophia cr task add <cr-id> "<task>"
sophia cr task contract set <cr-id> <task-id> --intent "..." --acceptance "..." --scope <prefix>
```

Implementation and checkpointing:

```bash
# file-scoped checkpoint
sophia cr task done <cr-id> <task-id> --path <file> --path <file>

# hunk/chunk-scoped checkpoint
sophia cr task chunk list <cr-id> <task-id>
sophia cr task chunk export <cr-id> <task-id> --chunk <chunk-id> --out task.patch
sophia cr task done <cr-id> <task-id> --patch-file task.patch
```

Evidence and readiness:

```bash
sophia cr evidence add <cr-id> \
  --type command_run \
  --summary "targeted tests" \
  --cmd "go test ./..." \
  --exit-code 0 \
  --attachment _docs/evidence/tests.log

sophia cr validate <cr-id>
sophia cr review <cr-id>
sophia cr status <cr-id>
sophia cr merge <cr-id>
```

## Operating Rules

- Treat contracts as the source of truth.
- Keep checkpoint scope explicit and minimal.
- Prefer deterministic command outputs (`--json`) when integrating with tools.
- If merge conflicts occur, use:
  - `sophia cr merge status <cr-id>`
  - resolve conflicts
  - `sophia cr merge resume <cr-id>` or `sophia cr merge abort <cr-id>`

## Troubleshooting Shortlist

- No active CR context: `sophia cr switch <id>`
- Metadata drift/missing: `sophia repair`
- Repository integrity issues: `sophia doctor`
- Validation errors: resolve required fields and out-of-scope drift before merge
