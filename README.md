# Sophia

Sophia is the no-headache replacement interface to Git for LLM-driven development.

Git remains the source of truth for code and history. Sophia changes what you optimize for: intent, scope, and evidence instead of PR diffs and “WIP commit” archaeology.

Your codebase is going to be written by LLMs. Your bottleneck is going to be review, coordination, and “why did we do this?” archaeology.

Sophia makes intent the primary artifact:
- merge commits are CR-shaped and carry intent summaries
- task checkpoints carry CR and task identity in commit footers
- CRs have an event ledger you can replay (contract updates, checkpoints, edits/redactions)

The primary user of Sophia is an LLM. Verbose contracts are a feature, not a bug.

## What It Feels Like

You don’t “open a PR and hope reviewers read the diff”.

You iterate with an LLM until the contract is crisp, then Sophia turns that intent into a deterministic, auditable merge:
1. Create a CR (the unit of intent)
2. Write the CR contract (why, scope, blast radius, test/rollback)
3. Decompose into scoped tasks and checkpoint progress
4. Run `cr validate` and `cr review`
5. Merge to `main`

## Proof: This Repo Is Built With Sophia

You can see intent in the Git graph without opening a diff:

```bash
git log --oneline --decorate --graph --all -n 25
```

Example (real history from this repo):

```text
*   b6c1137 (main) [CR-46] Maintenance: reduce CLI/service code smell and DRY debt
|\\
| * 1bbb8d7 chore(cr-46/task-15): Deduplicate equivalent CLI list printing helpers
| * 89d4d18 chore(cr-46/task-1): Create shared JSON/plaintext RunE error wrapper
|/
```

The merge commit itself carries intent and metadata (again, in Git):

```bash
git show -s --format=fuller b6c1137
```

Sophia also anchors CR identity in Git refs (so CRs can be resolved even when branches move/delete):

```bash
git for-each-ref refs/sophia/cr --format='%(refname:short) %(objectname:short)' | head
git for-each-ref refs/sophia/cr/uid --format='%(refname:short) %(objectname:short)' | head
```

And the CR itself has a replayable event log that shows exactly what happened and when:

```bash
go run ./cmd/sophia cr history 46
```

Finally, the “intent timeline” is a first-class view:

```bash
go run ./cmd/sophia log
```

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
go run ./cmd/sophia init

# 2) open a CR (does not auto-switch branches)
go run ./cmd/sophia cr add "Add retry policy" --description "Reduce transient failures"

# 3) switch into the CR branch for mutations
go run ./cmd/sophia cr switch <cr-id>

# 4) define contract + tasks
go run ./cmd/sophia cr contract set <cr-id> --why "..." --scope .
go run ./cmd/sophia cr task add <cr-id> "Implement retry behavior"
go run ./cmd/sophia cr task contract set <cr-id> <task-id> --intent "..." --acceptance "..." --scope internal/service

# 5) checkpoint task progress
go run ./cmd/sophia cr task done <cr-id> <task-id> --from-contract

# 6) validate and review
go run ./cmd/sophia cr validate <cr-id>
go run ./cmd/sophia cr review <cr-id>

# 7) merge
go run ./cmd/sophia cr merge <cr-id>
```

## Philosophy

As LLMs increase code creation velocity, diff-based PR review becomes the bottleneck.

Sophia shifts “review” away from PR diffs and toward:
- a detailed CR contract (why, scope, invariants, blast radius, test/rollback plans)
- task contracts (intent, acceptance criteria, explicit scope)
- deterministic validation and trust signals (`cr validate`, `cr review`)

The goal is to merge directly to `main` once the CR is complete and trustworthy, not to argue about line-by-line diffs.

## What Sophia Replaces

This is not “a nicer PR template”. It’s a different default.

| Old world | Sophia |
| --- | --- |
| PR description | CR contract |
| “LGTM” on diffs | `cr validate` + `cr review` (contract compliance + evidence) |
| WIP commits | scoped task checkpoints |
| scattered context | CR history + intent-rich merge commits |

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
Use CLI help for authoritative flags and command syntax:

```bash
go run ./cmd/sophia --help
go run ./cmd/sophia cr --help
go run ./cmd/sophia cr <command> --help
```

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

## Status

Sophia today is local-first and dogfooded in this repository.

“Sophia HQ” (planned) is the collaboration layer: shared CR creation, discussion, and online review surfaces that preserve the same contract-first workflow.

## Community and Governance

- Contributing: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Security policy: [`SECURITY.md`](SECURITY.md)
- Code of conduct: [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)
- License: [`LICENSE`](LICENSE)
