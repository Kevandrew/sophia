# Repository Guidelines

## Build, Test, and Development Commands
- `go run ./cmd/sophia <command>`: run the CLI locally without building a binary.
- `go build ./cmd/sophia`: compile the Sophia CLI entrypoint.
- `go test ./...`: run all unit and integration tests.
- `go vet ./...`: run static checks before committing.

Recommended daily flow (intent-first):
1. `sophia cr add "<title>" --description "<why>"`
   Optional stacked/multi-base forms:
   `sophia cr add "<title>" --base <git-ref>`
   `sophia cr add "<title>" --parent <cr-id>`
   `sophia cr child add "<title>" --description "<why>"`
2. `sophia cr contract set <id> --why "..." --scope <prefix> ...`
   Optional risk hints:
   `--risk-critical-scope <prefix> --risk-tier-hint <low|medium|high> --risk-rationale "..."`
3. `sophia cr task add <id> "<subtask>"`
4. `sophia cr task contract set <id> <task-id> --intent "..." --acceptance "..." --scope <prefix>`
5. implement on `sophia/cr-<id>`
6. `sophia cr task done <id> <task-id> --from-contract` (preferred checkpoint from task contract scope)
   Optional hunk flow:
   `sophia cr task chunk list <id> <task-id> [--path <file>] [--json]`
   `sophia cr task done <id> <task-id> --patch-file <patch-file>`
7. `sophia cr validate <id>`
8. `sophia cr review <id>`
9. optional machine-readable checks: `sophia cr status <id> --json`, `sophia cr validate <id> --json`
10. `sophia cr merge <id>`
11. stacked flows when needed: `sophia cr restack <id>` or `sophia cr base set <id> --ref <git-ref> [--rebase]`
12. optional delegated stacking:
   `sophia cr task delegate <parent-cr-id> <task-id> --child <child-cr-id>`
   `sophia cr stack [<id>] [--json]`

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
- Use contract and risk commands (`cr contract`, `cr impact`, `cr validate`) before merge.
- Set task contracts (`cr task contract`) before task completion; `task done` is blocked if missing.
- Task completion creates checkpoint commits via `sophia cr task done` and requires explicit scope mode (`--from-contract`, `--path`, `--patch-file`, or `--all`).
- Prefer `--from-contract` to keep staging aligned with task scope declarations.
- Use `--no-checkpoint` for metadata-only completion; use `--all` only when full-stage behavior is intended.
- Pre-staged index changes are rejected before checkpointing to prevent accidental scope drift.
- Merge is validation-gated; use `--override-reason` only for audited emergency bypasses.
- For non-delegated stacked CRs, merge parents before children unless an audited override is explicitly required.
- Delegated child CRs may merge before parent when explicitly linked via `cr task delegate`; parent merge remains blocked until delegated children are merged.
- PRs should include:
  - intent summary (what/why)
  - key command outputs (`go test`, `go vet`, `sophia cr review`)
  - notable behavior changes and edge cases covered

## Repository-Specific Notes
- `.sophia/` is local-first workflow state and is ignored in Git by default.
- If local metadata is missing/out-of-sync, run `sophia repair`.
- `_docs/` is local/internal and ignored via `.gitignore`.
- Current milestone: CR-20 (CR-native child delegation, stack topology visibility, and delegated merge blockers).
