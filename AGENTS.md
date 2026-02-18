# Repository Guidelines

## Build, Test, and Development Commands
- `go run ./cmd/sophia <command>`: run the CLI locally without building a binary.
- `go build ./cmd/sophia`: compile the Sophia CLI entrypoint.
- `go test ./...`: run all unit and integration tests.
- `go vet ./...`: run static checks before committing.

Recommended daily flow (intent-first):
0. optional declarative setup for LLMs/humans:
   `sophia cr apply --file <plan.yaml> [--dry-run] [--json] [--keep-file]`
   Template seed is available at `.sophia/cr-plan.sample.yaml` after `sophia init`.
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
5. implement on the active CR branch (typically via `sophia cr switch <id>`)
6. `sophia cr task done <id> <task-id> --from-contract` (preferred checkpoint from task contract scope)
   Optional hunk flow:
   `sophia cr task chunk list <id> <task-id> [--path <file>] [--json]`
   `sophia cr task done <id> <task-id> --patch-file <patch-file>`
7. `sophia cr validate <id>` (read-only by default; add `--record` to append validation audit event)
8. `sophia cr review <id>` (use Trust verdict/required_actions as deterministic confidence signal; treat `attention_actions` as non-blocking improvement guidance; hard-fail means `validation errors > 0` or missing required CR contract fields, and evidence signals come from scope drift, validation warnings/errors, task checkpoints, tests/dependencies touched, and delegated blockers)
9. optional machine-readable checks: `sophia cr status <id> --json`, `sophia cr validate <id> --json`, `sophia cr check status <id> --json`, `sophia doctor --json`
10. `sophia cr merge <id>`
11. if merge conflicts occur:
   `sophia cr merge status <id>`
   resolve conflicts + `sophia cr merge resume <id>` or `sophia cr merge abort <id>`
12. stacked flows when needed: `sophia cr restack <id>` or `sophia cr base set <id> --ref <git-ref> [--rebase]`
13. optional delegated stacking:
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
- Use `--no-checkpoint --no-checkpoint-reason "<why>"` for metadata-only completion; use `--all` only when full-stage behavior is intended.
- Pre-staged index changes are rejected before checkpointing to prevent accidental scope drift.
- Merge is validation-gated; use `--override-reason` only for audited emergency bypasses.
- During unresolved merge state, mutating CR commands are blocked until `cr merge abort` or `cr merge resume` completes.
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
- Current milestone: CR-29 (merge conflict recovery primitive with deterministic status/abort/resume).
