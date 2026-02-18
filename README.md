# Sophia

Sophia is an intent-first workflow layer over Git.

Git remains the source of truth for code and history. Sophia adds structured intent via Change Requests (CRs), task contracts, and deterministic validation/review workflows.

## Quickstart

Prerequisites:
- Go 1.22+
- Git

Run locally without installing a binary:

```bash
go run ./cmd/sophia --help
```

Typical first workflow:

```bash
# 1) initialize metadata
sophia init

# 2) open a CR (does not auto-switch branches)
sophia cr add "Add retry policy" --description "Reduce transient failures"

# 3) switch into the CR branch for mutations
sophia cr switch <cr-id>

# 4) define contract + tasks
sophia cr contract set <cr-id> --why "..." --scope .
sophia cr task add <cr-id> "Implement retry behavior"
sophia cr task contract set <cr-id> <task-id> --intent "..." --acceptance "..." --scope internal/service

# 5) checkpoint task progress
sophia cr task done <cr-id> <task-id> --from-contract

# 6) validate and review
sophia cr validate <cr-id>
sophia cr review <cr-id>

# 7) merge
sophia cr merge <cr-id>
```

## Install and Distribution

| Channel | Status | Notes |
| --- | --- | --- |
| `go run` from source | Available | `go run ./cmd/sophia <command>` |
| Local binary build | Available | `go build ./cmd/sophia` |
| `go install` | Planned | Versioned module install guidance will be published with release tags. |
| Homebrew | Planned | Formula/repo not published yet. |
| Prebuilt release binaries | Planned | Release artifacts and checksums not published yet. |

## Core Concepts

- **CR (Change Request)**: unit of intent and review.
- **CR Contract**: required rationale, scope, blast radius, and validation plan.
- **Task Contract**: per-task intent, acceptance criteria, and scope.
- **Task Checkpoint**: contract-scoped commit via `sophia cr task done`.
- **Validation + Review**: deterministic quality checks before merge.

## Documentation

Start with the docs index: [`docs/index.md`](docs/index.md)

Key guides:
- Getting started: [`docs/getting-started.md`](docs/getting-started.md)
- CLI reference (curated): [`docs/cli-reference.md`](docs/cli-reference.md)
- Workflow lifecycle: [`docs/workflow.md`](docs/workflow.md)
- Repository policy (`SOPHIA.yaml`): [`docs/repository-policy.md`](docs/repository-policy.md)
- Branch identity model: [`docs/branch-identity.md`](docs/branch-identity.md)

## Development Checks

Minimum pre-commit baseline:

```bash
go test ./...
go vet ./...
```

## Community and Governance

- Contributing: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Security policy: [`SECURITY.md`](SECURITY.md)
- Code of conduct: [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)
- License: [`LICENSE`](LICENSE)
