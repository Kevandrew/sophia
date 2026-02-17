# Sophia

**Sophia is the intent layer for AI-native development.**

Git stores code.
Sophia stores *intent*.

In an era where AI generates most code, diff-based workflows break down.
Sophia makes **Change Requests (CRs)** the primary unit of work — not commits, not pull requests.

Git remains the system of record.
Sophia becomes the interface.

---

## Philosophy

Traditional workflow:

```
edit → commit → PR → diff review → merge
```

AI-native workflow:

```
intent → CR → implement → semantic review → merge
```

Sophia does not replace Git.
It abstracts it.

CRs are first-class objects.
Commits are compiled artifacts.

---

## What Sophia CLI Does (v0 Scope)

Sophia CLI is a thin Git wrapper that:

1. Creates and manages Change Requests (CRs)
2. Maps each CR to a dedicated branch
3. Stores structured intent metadata locally in `.sophia/`
4. Tracks task-level progress and checkpoint commits
5. Generates intent-rich CR commits and task checkpoint commits
6. Enforces structured intent contracts and deterministic impact validation
7. Supports auditable metadata amendment/redaction and history inspection
8. Formats review/log output around intent, not diffs

It does **not**:

* Run agents
* Auto-generate code
* Replace GitHub
* Perform heavy semantic reasoning (yet)

---

## Core Concepts

### Change Request (CR)

A CR represents a unit of intent.

Each CR:

* Has a unique ID
* Has a title and description
* Lives in `.sophia/cr/<id>.yaml`
* Maps to a branch `sophia/cr-<id>`

By default (`metadata_mode=local`), Sophia metadata is shared per-repository under the Git common dir (`<git-common-dir>/sophia-local`) so all worktrees use the same CR state.
Tracked mode (`metadata_mode=tracked`) keeps metadata in `.sophia/` inside the worktree.

---

## Repository Structure

In tracked mode, Sophia creates:

```
.sophia/
  config.yaml
  index.yaml
  cr/
    1.yaml
    2.yaml
```

Each CR file contains structured metadata:

```yaml
id: 1
title: Add billing retries
status: in_progress
base_branch: main
branch: sophia/cr-1
notes:
  - Initial retry logic implementation
subtasks: []
```

---

## CLI Commands (Current)

### Initialize Repository

```
sophia init [--base-branch <name>] [--metadata-mode local|tracked]
```

* Creates shared local metadata in `<git-common-dir>/sophia-local` by default
* Creates `.sophia/` when `--metadata-mode tracked` is selected
* Writes config
* Ensures git repo exists
* Sets default base branch
* Defaults to local metadata mode (shared across worktrees)
* Seeds `cr-plan.sample.yaml` in the active metadata root when missing

---

### Create Change Request

```
sophia cr add "Add billing retries"
sophia cr add "Add billing retries" --base release/2026-q1
sophia cr add "Add billing retries" --parent 12
sophia cr child add "Implement parser split" --description "Delegated from active parent CR."
```

Behavior:

* Generate new CR ID
* Create branch `sophia/cr-<id>`
* Supports per-CR base refs via `--base <git-ref>`
* Supports stacked child CR creation via `--parent <cr-id>` (mutually exclusive with `--base`)
* Supports child CR creation from active CR context via `cr child add`
* Write CR YAML file
* Checkout branch

---

### Apply YAML Plan (CR Setup Primitive)

```
sophia cr apply --file plan.yaml
sophia cr apply --file plan.yaml --dry-run --json
sophia cr apply --file plan.yaml --keep-file
```

Behavior:

* Parses a strict versioned YAML schema (`version: v1`) with unknown-field rejection
* Validates CR graph semantics before mutation (parent references, cycles, delegation targets)
* Executes serially: create CR -> set CR contract -> add tasks -> set task contracts -> apply delegations
* Supports dry-run previews with deterministic planned operations and predicted IDs
* Deletes source plan file after successful apply by default; `--keep-file` preserves it
* Restores the starting branch after apply/dry-run when possible

Minimal schema:

```yaml
version: v1
crs:
  - key: parent_refactor
    title: "Decouple large service file"
    description: "Reduce service surface size and isolate responsibilities."
    base: "main"
    contract:
      why: "..."
      scope: ["internal/service"]
      non_goals: ["..."]
      invariants: ["..."]
      blast_radius: "..."
      test_plan: "go test ./... && go vet ./..."
      rollback_plan: "Revert CR merge commit."
    tasks:
      - key: split_service
        title: "Split service responsibilities"
        contract:
          intent: "..."
          acceptance_criteria: ["..."]
          scope: ["internal/service"]
        delegate_to: ["child_cli"]
  - key: child_cli
    title: "Split CLI command wiring"
    description: "Child implementation slice."
    parent_key: "parent_refactor"
    tasks:
      - key: split_cli
        title: "Split CLI command files"
```

---

### Stack Topology

```
sophia cr stack
sophia cr stack 20 --json
```

Behavior:

* Shows root/focus CR IDs and stack node ordering
* Includes per-node merge blockers and delegated task counts
* Provides deterministic JSON fields for machine consumption

---

### Base + Restack Management

```
sophia cr base set <id> --ref <git-ref> [--rebase]
sophia cr restack <id>
```

Behavior:

* `cr base set` retargets a CR onto a new base ref and stores resolved base commit metadata
* `--rebase` performs an immediate Git rebase of the CR branch onto the new base
* `cr restack` rebases a child CR onto its parent effective head (parent branch when open, merged commit when closed)
* Parent-child metadata is preserved for deterministic review/validate diffs

---

### List CRs

```
sophia cr list
```

Shows:

* ID
* Title
* Status
* Branch

---

### Add Note to CR

```
sophia cr note <id> "Refactored payment client"
```

Appends structured note to CR file.

Agents should also be instructed to append notes.

---

### Complete Task (Checkpoint by Default)

```
sophia cr task done <cr-id> <task-id> --from-contract
```

Behavior:

* Requires explicit checkpoint scope mode: `--from-contract`, `--path <file>` (repeatable), `--patch-file <file>`, or `--all`
* Requires task contract completeness before completion (`intent`, `acceptance_criteria`, `scope`)
* `--from-contract` stages changed files that match task contract scope prefixes
* Stages only selected paths by default (or all changes when `--all` is explicitly set)
* Fails fast if staged changes already exist before checkpointing
* Marks task done only if checkpoint commit succeeds
* Records checkpoint metadata on the task (`commit`, `timestamp`, message, `checkpoint_scope`, `checkpoint_chunks`)
* Requires active branch to match the CR branch
* Rejects checkpoint completion for delegated tasks until delegation links are resolved

Optional metadata-only completion:

```
sophia cr task done <cr-id> <task-id> --no-checkpoint
```

Explicit legacy stage-all behavior:

```
sophia cr task done <cr-id> <task-id> --all
```

Explicit file selection behavior:

```
sophia cr task done <cr-id> <task-id> --path internal/service/service.go --path internal/cli/cr.go
```

Patch-manifest (hunk-scoped) behavior:

```
sophia cr task done <cr-id> <task-id> --patch-file /tmp/task.patch
```

Chunk discovery (read-only):

```
sophia cr task chunk list <cr-id> <task-id>
sophia cr task chunk list <cr-id> <task-id> --path internal/service/service.go --json
```

Delegation:

```
sophia cr task delegate <parent-cr-id> <task-id> --child <child-cr-id>
sophia cr task undelegate <parent-cr-id> <task-id> --child <child-cr-id>
```

Behavior:

* Delegation creates a child task and copies parent task contract fields
* Parent task transitions to `delegated` while links remain
* Parent task auto-completes when all delegated child CRs are merged

---

### Task Contract Management

```
sophia cr task contract set <cr-id> <task-id> --intent "..." --acceptance "..." --scope internal/service
sophia cr task contract show <cr-id> <task-id>
```

Behavior:

* Stores task-level intent contract fields for each subtask
* Supports partial updates and records `task_contract_updated` audit events
* Enables task-contract drift warnings in review/validation

---

### Contract Management

```
sophia cr contract set <id> --why "..." --scope internal/service --scope cmd --non-goal "..." --invariant "..." --blast-radius "..." --risk-critical-scope internal/service --risk-tier-hint high --risk-rationale "..." --test-plan "..." --rollback-plan "..."
sophia cr contract show <id>
```

Behavior:

* Stores structured intent contract fields in CR metadata
* Supports partial updates and records `contract_updated` audit events
* Uses contract scope prefixes for drift checks during validation/merge
* Supports contract-authored risk hints (`risk_critical_scopes`, `risk_tier_hint`, `risk_rationale`) for repo-agnostic impact scoring

---

### Impact and Validation

```
sophia cr impact <id>
sophia cr validate <id>
```

Behavior:

* `impact` computes deterministic risk tier/score and blast-radius signals from diff metadata
* `impact` includes contract-driven risk scope signals and optional risk-tier floor hints when configured in CR contract
* `validate` enforces required contract fields and scope-drift policy
* `validate` emits blocking `Errors` and non-blocking `Warnings`
* `validate` includes task chunk metadata warnings (`task_chunk_warnings`) when chunk metadata is malformed/inconsistent
* `validate` records a `cr_validated` audit event
* For merged CRs whose branch was deleted, `validate` derives diff context from the merge commit (with task-checkpoint scope fallback)
* Both commands support machine-readable output via `--json`

---


```
# parent intent
sophia cr add "Parent rollout" --description "Coordinate delegated child work"
sophia cr task add <parent-id> "Implement risky slice"
sophia cr task contract set <parent-id> <task-id> --intent "..." --acceptance "..." --scope internal/service

# child from current parent context
sophia cr child add "Child risky slice" --description "Delegated implementation"
sophia cr task delegate <parent-id> <task-id> --child <child-id>
sophia cr stack <parent-id> --json

# contract-driven risk hints on child
sophia cr contract set <child-id> --risk-critical-scope internal/service --risk-tier-hint high --risk-rationale "Touches parser and merge paths."
sophia cr impact <child-id>
sophia cr validate <child-id>
sophia cr status <child-id> --json

# merge ordering for delegated flow
sophia cr merge <child-id>
sophia cr status <parent-id>
sophia cr merge <parent-id>
```

---

### Why and Merge Readiness

```
sophia cr why <id>
sophia cr status <id>
```

Behavior:

* `why` returns the effective rationale (`contract why` fallback to CR description)
* `status` returns CR identity (`id` + immutable `uid`), per-CR base metadata (`base_ref`, `base_commit`), parent metadata, branch context, workspace dirtiness, task progress (including delegated counters), contract completeness, validation summary, `merge_blocked`, and `merge_blockers`
* Both commands support `--json`

---

### Review CR

```
sophia cr review <id>
```

Displays:

* Title
* Notes
* Files changed (`git diff --name-only`)
* Insertions/deletions
* Test file changes (basic detection)
* Deterministic trust envelope (`trusted | needs_attention | untrusted`) with score, hard-fail reasons, dimension breakdown, and required actions
* Trust is advisory-only in v1 (no merge gating change); thresholds are `trusted >= 85`, `needs_attention 60..84`, `untrusted < 60` or any hard-fail
* Hard-fail is defined as either `validation errors > 0` or missing required CR contract fields; the missing-fields check is intentionally listed separately for explicit reviewer consistency, even though it usually overlaps validation errors
* Evidence signals are derived from scope drift, validation warnings/errors, task checkpoint presence/missing proof commits, tests touched, dependency files touched, and delegated-task blockers/pending state
* For merged CRs whose branch was deleted, review diff context is derived from merge metadata instead of live branch diff
* Supports machine-readable output via `--json`

This is formatting around Git — not replacing it.

---

### Merge CR

```
sophia cr merge <id>
```

Behavior:

* Creates an intent-rich merge commit into base (non-linear Git graph)
* Runs CR validation first and blocks merge on validation errors by default
* Generates a structured commit message:

```
[CR-1] Add billing retries

Intent:
Improve retry behavior in billing client.

Subtasks:
- [x] #1 Add backoff support
- [x] #2 Add tests

Notes:
- Refactored payment client

Metadata:
- actor: Jane <jane@example.com>
- merged_at: 2026-02-17T08:32:10Z

Sophia-CR: 1
Sophia-CR-UID: cr_4fd8bc65-9360-48b5-912d-95f8a03a2d6d
Sophia-Base-Ref: main
Sophia-Base-Commit: 2f4a9f0b6e78d9f2e6fbe2f3f31d42c676f3b1b1
Sophia-Intent: Add billing retries
Sophia-Tasks: 2 completed
```

* Mark CR as merged in local metadata
* Delete branch by default (`--keep-branch` to retain)
* Non-delegated stacks keep parent-first merge gating
* Delegated children may merge before parent when explicitly linked from parent task delegation
* Parent CR merge blocks while delegated tasks still point to unmerged child CRs
* Supports emergency audited bypass:

```
sophia cr merge <id> --override-reason "hotfix required for production outage"
```

---

### Workflow Integrity + Visibility

```
sophia doctor
sophia log
sophia blame <path> [--rev <git-ref>] [-L start,end] [--json]
sophia repair
sophia hook install
sophia cr why <id>
sophia cr status <id>
sophia cr current
sophia cr switch <id>
sophia cr reopen <id>
sophia cr base set <id> --ref <git-ref>
sophia cr restack <id>
sophia cr task contract set <cr-id> <task-id> --intent "..."
sophia cr task contract show <cr-id> <task-id>
sophia cr task chunk list <cr-id> <task-id> [--path <file>] [--json]
sophia cr task done <cr-id> <task-id> --patch-file <patch-file>
sophia cr task delegate <parent-cr-id> <task-id> --child <child-cr-id>
sophia cr task undelegate <parent-cr-id> <task-id> --child <child-cr-id>
sophia cr child add "<title>" --description "..."
sophia cr stack [<id>] [--json]
sophia cr edit <id> --title "..."
sophia cr contract set <id> --why "..."
sophia cr contract show <id>
sophia cr impact <id>
sophia cr validate <id>
sophia cr review <id> --json
sophia cr redact <id> --note-index 1 --reason "..."
sophia cr history <id>
```

* `doctor` flags workflow drift (dirty tree, non-CR branch, stale merged CR branches)
* `log` shows intent-first CR history and can reconstruct merged CRs from Git commit metadata
* `blame` shows Sophia-enriched per-line attribution (`CR`, intent) with fallback to commit summary when CR metadata is unavailable
* Worktree-safe local metadata keeps CR IDs/state shared across worktrees and avoids branch checkout collisions during init/add/merge flows
* `repair` rebuilds missing local CR metadata from Git history and realigns CR IDs
* `hook install` adds a pre-commit guard against direct commits on the base branch
* `current/switch/reopen/base/restack` supports deterministic branch and stack context moves
* `task contract` enforces subtask intent + acceptance + scope before completion
* `task chunk list` provides deterministic hunk discovery from current working-tree diff
* `task done --patch-file` checkpoints selected hunks from a patch manifest
* `contract/impact/validate` provide intent integrity and blast-radius review context
* `--json` on read/check commands provides stable machine-readable envelopes for agents
* JSON read/check outputs include immutable CR uid fields and per-CR base/parent metadata for stacked workflows
* `edit/redact/history` supports retroactive metadata hygiene with audit-safe events

---

## Implementation Notes (For Coding Agent)

Language: Go

Use:

* cobra for CLI
* os/exec for Git integration (simpler than reimplementing git)
* yaml.v3 for CR files
* clean error handling
* deterministic ID generation (incremental integer via index.yaml)

Keep v0 minimal.

Do not:

* Add LLM integration
* Add cloud sync
* Add complex branching logic

Focus on:

* Correct Git plumbing
* Clean file structure
* Predictable state transitions
* Simple UX

---

## Design Constraints

Sophia must:

* Be local-first
* Be Git-compatible
* Add zero friction
* Feel lighter than GitHub
* Compile intent into commits

If it feels heavier than plain Git, it has failed.

---

## Long-Term Vision (Not for v0)

Future capabilities may include:

* Hosted CR coordination
* Duplicate detection
* Semantic summarization
* Multi-agent coordination
* Intent clustering
* Cross-repo CR tracking

These are **not part of the CLI v0 implementation**.

---

## Why Sophia Exists

AI increases change velocity.

Git optimizes for human-scale diffs.

Sophia optimizes for intent-scale coordination.

If AI is the dominant producer of code,
intent becomes the primary artifact.

Sophia is that artifact.

---

## Next Step

Build the minimal CLI.
Use Sophia to build Sophia.

The tool must justify its own existence inside this repository.

If you stop thinking in `git commit -m "WIP"`,
Sophia is working.
