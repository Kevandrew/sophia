# Repository Guidelines

## Build, Test, and Development Commands
- `go run ./cmd/sophia <command>`: run the CLI locally without building a binary.
- `go build ./cmd/sophia`: compile the Sophia CLI entrypoint.
- `go test ./...`: run all unit and integration tests.
- `go vet ./...`: run static checks before committing.

Recommended daily flow (intent-first):
1. `sophia cr add "<title>" --description "<why>"`
2. `sophia cr task add <id> "<subtask>"`
3. implement on `sophia/cr-<id>`
4. `sophia cr task done <id> <task-id>` (default checkpoint commit; stages all current changes)
5. `sophia cr review <id>`
6. `sophia cr merge <id>`

## Coding Style & Naming Conventions
- Language: Go (module `sophia`).
- Format all edited Go files with `gofmt`.
- Keep packages focused by layer:
  - `internal/cli`: Cobra command wiring and output formatting
  - `internal/service`: workflow and business logic
  - `internal/gitx`: Git command integration
  - `internal/store` and `internal/model`: persistence and types
- Prefer capability-based file names (avoid milestone names like `plan2`).
- Keep identifiers descriptive and stable; avoid abbreviations unless obvious.

## Testing Guidelines
- Test framework: Go `testing` package.
- Test files use `*_test.go` naming and live near the code they verify.
- Add service-level tests for workflow behavior and git edge cases (temp repos).
- For new commands/features, add positive and failure-path tests.
- Minimum pre-commit check: `go test ./... && go vet ./...`.

## Commit & Pull Request Guidelines
- Follow existing commit style:
  - CR merge commits: `[CR-<id>] <intent title>`
  - Maintenance commits: `chore: ...`
  - Feature commits: `feat: ...`
- Use Sophia as the primary workflow interface (`cr`, `task`, `note`, `review`, `merge`, `repair`).
- Task completion can create checkpoint commits automatically via `sophia cr task done`; use `--no-checkpoint` only when needed.
- Checkpoint commits are repo-wide (`git add -A`) in CR-6; no hunk/line-scoped checkpointing yet.
- PRs should include:
  - intent summary (what/why)
  - key command outputs (`go test`, `go vet`, `sophia cr review`)
  - notable behavior changes and edge cases covered

## Repository-Specific Notes
- `.sophia/` is local-first workflow state and is ignored in Git by default.
- If local metadata is missing/out-of-sync, run `sophia repair`.
- `_docs/` is local/internal and ignored via `.gitignore`.
- Current milestone: CR-6 (task-driven checkpoint commits).
