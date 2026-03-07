# Repository Guidelines

Sophia is a Go CLI for intent-first change-request workflow over Git.

## Start Here

- Repo map: `docs/index.md`
- Agent setup: `docs/agent-quickstart.md`
- First-success walkthrough: `docs/getting-started.md`
- Daily author loop: `docs/workflow.md`
- Recovery and stale-state handling: `docs/troubleshooting.md`
- Policy details: `docs/repository-policy.md`

## Build And Verify

- `go run ./cmd/sophia <command>` runs the CLI locally.
- `go build ./cmd/sophia` builds the entrypoint.
- `go test ./...` runs the test suite.
- `go vet ./...` runs static checks.

## Workflow Invariants

- Use Sophia as the primary workflow interface for CRs, tasks, review, and merge state.
- Work on the active CR branch, usually via `sophia cr switch <id>`.
- Set the CR contract before implementation and set each task contract before `task done`.
- Use explicit checkpoint scope on `sophia cr task done`; prefer `--from-contract` when scope is accurate.
- For agent-created checkpoints, always pass `--commit-type <type>`.
- This repo uses `merge.mode=pr_gate`; do not open/sync PRs or push remote state unless explicitly asked.
- Delegated child CRs may complete before the parent; the parent remains blocked until child work is resolved.

## Codebase Map

- `internal/cli`: Cobra wiring, help text, JSON/text output.
- `internal/service`: workflow logic, status/review/merge behavior.
- `internal/gitx`: Git integration helpers.
- `internal/store`, `internal/model`: persistence and types.

## Coding And Tests

- Format edited Go files with `gofmt`.
- Keep file names capability-based and identifiers descriptive.
- Add focused Go tests near the code you change.
- Minimum local check before handoff: `go test ./... && go vet ./...`.

## Repo Notes

- `.sophia/` is local-first workflow state and is ignored by Git by default.
- `_docs/` is local/internal and ignored via `.gitignore`.
- If metadata drifts from Git history, run `sophia repair`.

## Skills

- Use `$sophia` for Sophia CLI workflow tasks.
- Use other installed skills only when the task clearly matches them; the runtime skill registry is the source of truth.
