# Getting Started

This guide walks through the first successful Sophia workflow from repository setup to CR merge.

## Prerequisites

- Git repository initialized
- Go 1.22+ installed
- Local environment can run `go run ./cmd/sophia`

## 1) Initialize Sophia

```bash
sophia init
```

What this does:
- Validates Git repository context
- Initializes Sophia metadata (local-first by default)
- Seeds policy/template files when missing

## 2) Create a CR

```bash
sophia cr add "Add retry policy" --description "Reduce transient failures"
```

By default, `cr add` does not switch your branch.

Switch explicitly before mutating work:

```bash
sophia cr switch <cr-id>
```

## 3) Define the CR Contract

```bash
sophia cr contract set <cr-id> \
  --why "Reduce retry-related incidents" \
  --scope internal/service \
  --test-plan "go test ./... && go vet ./..." \
  --rollback-plan "Revert CR merge commit"
```

`SOPHIA.yaml` may require additional fields (`non_goal`, `invariant`, `blast_radius`, etc.), so check your local policy.

## 4) Add Tasks and Task Contracts

```bash
sophia cr task add <cr-id> "Implement jittered backoff"
sophia cr task contract set <cr-id> <task-id> \
  --intent "Add bounded jittered retry backoff" \
  --acceptance "Retries follow bounded exponential policy" \
  --scope internal/service
```

## 5) Complete Tasks with Explicit Scope

Preferred checkpoint mode:

```bash
sophia cr task done <cr-id> <task-id> --from-contract
```

Other explicit scope modes:
- `--path <file>` (repeatable)
- `--patch-file <manifest>`
- `--all`

## 6) Validate and Review

```bash
sophia cr validate <cr-id>
sophia cr review <cr-id>
```

Use JSON output when integrating with tools:

```bash
sophia cr status <cr-id> --json
sophia cr validate <cr-id> --json
sophia cr check status <cr-id> --json
```

## 7) Merge

```bash
sophia cr merge <cr-id>
```

If merge conflicts occur:

```bash
sophia cr merge status <cr-id>
# resolve conflicts
sophia cr merge resume <cr-id>
# or abort
sophia cr merge abort <cr-id>
```

## Next Documents

- Command map: [`cli-reference.md`](cli-reference.md)
- Lifecycle details: [`workflow.md`](workflow.md)
- Policy model: [`repository-policy.md`](repository-policy.md)
