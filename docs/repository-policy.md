# Repository Policy (`SOPHIA.yaml`)

Sophia reads repository policy from `SOPHIA.yaml` at repository root.

Policy controls what is required for CR/task contracts, how scope is interpreted, and how trust/review checks are evaluated.

## File Location and Behavior

- Path: `SOPHIA.yaml` (fixed)
- Missing file: Sophia falls back to built-in defaults
- Unknown keys: ignored for forward compatibility, with deterministic warnings surfaced in JSON validation surfaces and `sophia doctor`
- Invalid YAML syntax/type mismatches: deterministic failure (`policy_invalid`-style path)

## Contract Requirements

`contract.required_fields` defines CR fields that must be set before merge readiness.

Typical required CR fields:
- `why`
- `scope`
- `non_goals`
- `invariants`
- `blast_radius`
- `test_plan`
- `rollback_plan`

## Task Contract Requirements

`task_contract.required_fields` defines what each task must provide before completion.

Common required task fields:
- `intent`
- `acceptance_criteria`
- `scope`

## Scope Conventions

`scope.allowed_prefixes` constrains accepted scope prefixes for CR/task contracts.

Practical guidance:
- Keep scopes specific enough for meaningful task checkpointing.
- Prefer capability-scoped prefixes over catch-all roots when possible.

## Classification Hints

`classification` helps Sophia identify test/dependency surfaces in impact/review flows.

- `classification.test`
  - filename suffixes and path fragments that imply tests
- `classification.dependency`
  - lockfiles/manifests that imply dependency changes

## Merge Override Policy

`merge.allow_override` controls whether audited override flows are permitted when standard merge gating blocks.

## Merge Mode and PR Gates

`merge.mode` controls merge orchestration:
- `local` (default): existing local merge behavior.
- `pr_gate`: `sophia cr merge` publishes/syncs PR for review instead of performing final merge locally.

`pr_gate` policy controls:
- `merge.required_approvals` (default `1`)
- `merge.require_non_author_approval` (default `true`)
- `merge.require_ready_for_review` (default `true`)
- `merge.require_passing_checks` (default `true`)

Repository CI guidance for `pr_gate`:
- PR checks should run from GitHub Actions `pull_request` events and treat draft PRs as non-ready.
- Use `ready_for_review` in workflow triggers so checks run as soon as a draft PR is promoted.
- For this repository, the `CI` workflow runs `go test ./... -count=1` and `go vet ./...` when `pull_request.draft == false`.

Invalid `merge.mode` values return deterministic `policy_invalid` failures.

## Archive Policy

`archive` controls tracked CR archive artifact behavior.

- `archive.enabled`
  - Enables automatic archive creation during `sophia cr merge` and `sophia cr merge resume`.
  - Default: `true`.
- `archive.path`
  - Repository-relative directory where archive files are written.
  - Default: `.sophia-tracked/cr`.
- `archive.format`
  - Archive file format. Current supported value: `yaml`.
  - Default: `yaml`.
- `archive.include_full_diffs`
  - Parsed for forward compatibility, but full diff embedding is not implemented in v1 archive generation.
  - Default: `false`.

Archive semantics:

- Archives are append-only snapshots named `cr-<id>.vN.yaml`.
- Existing revisions are never rewritten; corrections are new revisions.
- Archives are intended for historical lookback and automation-friendly records.
- In `merge.mode=pr_gate`, `v1` archive artifact is staged/committed to the CR branch before PR sync so final review includes archive output.

## Trust Policy

`trust` controls evidence quality expectations:

- `mode`: trust evaluation mode
- `gate.enabled`: whether trust verdicts enforce merge gates
- `thresholds`: required confidence by risk tier
- `checks`: executable check definitions and freshness policy
- `review_depth`: required sampling depth by risk tier

When checks are not configured, machine-readable check endpoints may report guidance rather than pass/fail evidence.

Example Go infra check definitions:
```yaml
trust:
  checks:
    freshness_hours: 24
    definitions:
      - key: unit_tests
        command: go test ./...
        tiers: [low, medium, high]
        allow_exit_codes: [0]
      - key: go_vet
        command: go vet ./...
        tiers: [low, medium, high]
        allow_exit_codes: [0]
```

## Contributor Expectations

Before requesting merge:

1. CR contract is complete per policy.
2. Task contracts are complete before `task done`.
3. Validation and review are run on the target CR.
4. Test/quality commands in `test_plan` are executed and failures resolved.

## Related Docs

- Workflow lifecycle: [`workflow.md`](workflow.md)
- Command map: [`cli-reference.md`](cli-reference.md)
- Getting started: [`getting-started.md`](getting-started.md)
