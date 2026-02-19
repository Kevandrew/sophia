# Repository Policy (`SOPHIA.yaml`)

Sophia reads repository policy from `SOPHIA.yaml` at repository root.

Policy controls what is required for CR/task contracts, how scope is interpreted, and how trust/review checks are evaluated.

## File Location and Behavior

- Path: `SOPHIA.yaml` (fixed)
- Missing file: Sophia falls back to built-in defaults
- Invalid YAML or unknown keys: deterministic failure (`policy_invalid`-style path)

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

## Trust Policy

`trust` controls evidence quality expectations:

- `mode`: trust evaluation mode
- `gate.enabled`: whether trust verdicts enforce merge gates
- `thresholds`: required confidence by risk tier
- `checks`: executable check definitions and freshness policy
- `review_depth`: required sampling depth by risk tier

When checks are not configured, machine-readable check endpoints may report guidance rather than pass/fail evidence.

## Contributor Expectations

Before requesting merge:

1. CR contract is complete per policy.
2. Task contracts are complete before `task done`.
3. Validation and review are run on the target CR.
4. Test/quality commands in `test_plan` are executed and failures resolved.

Before release/documentation updates, run:

```bash
scripts/check-doc-commands.sh
```

This checks that documented `sophia ... --help` command surfaces still exist.

## Related Docs

- Workflow lifecycle: [`workflow.md`](workflow.md)
- Command map: [`cli-reference.md`](cli-reference.md)
- Getting started: [`getting-started.md`](getting-started.md)
