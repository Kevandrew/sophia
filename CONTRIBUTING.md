# Contributing to Sophia

Thanks for contributing.

This repository uses Sophia as the primary workflow interface over Git. Contributions should preserve intent-first traceability through CR contracts and task contracts.

## Prerequisites

- Go 1.22+
- Git
- Ability to run `go run ./cmd/sophia`

## Local Setup

```bash
go run ./cmd/sophia init
```

If metadata is missing or out of sync, run:

```bash
go run ./cmd/sophia repair
```

## Contribution Workflow

1. Create a CR:

```bash
go run ./cmd/sophia cr add "<title>" --description "<why>"
```

2. Switch to the CR branch:

```bash
go run ./cmd/sophia cr switch <cr-id>
```

3. Set CR contract fields required by `SOPHIA.yaml`.
4. Add tasks and set task contracts (`intent`, `acceptance`, `scope`).
5. Implement changes.
6. Complete tasks with explicit checkpoint scope (prefer `--from-contract`).

```bash
go run ./cmd/sophia cr task done <cr-id> <task-id> --from-contract
```

7. Run validation and review:

```bash
go run ./cmd/sophia cr validate <cr-id>
go run ./cmd/sophia cr review <cr-id>
```

8. Merge when ready:

```bash
go run ./cmd/sophia cr merge <cr-id>
```

## Quality Gates

Minimum checks before merge:

```bash
go test ./...
go vet ./...
```

If your CR contract `test_plan` declares additional checks, run those as well.

## Scope Discipline

- Keep task scopes precise and contract-aligned.
- Avoid unrelated refactors in feature/maintenance CRs.
- Prefer small, reviewable task checkpoints.

## Documentation Changes

- Keep `README.md` onboarding-focused.
- Put deeper operational guidance under `docs/`.
- Keep `_docs/` internal/private unless explicitly promoted.

## Reporting Problems

For security issues, follow [`SECURITY.md`](SECURITY.md) and avoid public disclosure.

## Code of Conduct

By participating, you agree to follow [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).
