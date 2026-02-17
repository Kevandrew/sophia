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

By default, `.sophia/` is local metadata and ignored in Git.

---

## Repository Structure

When initialized, Sophia creates:

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

* Creates `.sophia/`
* Writes config
* Ensures git repo exists
* Sets default base branch
* Defaults to local metadata mode (`.sophia/` ignored)

---

### Create Change Request

```
sophia cr add "Add billing retries"
```

Behavior:

* Generate new CR ID
* Create branch `sophia/cr-<id>`
* Write CR YAML file
* Checkout branch

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
sophia cr task done <cr-id> <task-id> --path internal/service/service.go --path internal/cli/cr.go
```

Behavior:

* Requires explicit checkpoint scope: `--path <file>` (repeatable) or `--all`
* Requires task contract completeness before completion (`intent`, `acceptance_criteria`, `scope`)
* Stages only selected paths by default (or all changes when `--all` is explicitly set)
* Fails fast if staged changes already exist before checkpointing
* Marks task done only if checkpoint commit succeeds
* Records checkpoint metadata on the task (`commit`, `timestamp`, message, `checkpoint_scope`)
* Requires active branch to match the CR branch

Optional metadata-only completion:

```
sophia cr task done <cr-id> <task-id> --no-checkpoint
```

Explicit legacy stage-all behavior:

```
sophia cr task done <cr-id> <task-id> --all
```

Chunk/hunk scoping is planned for CR-10.

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
sophia cr contract set <id> --why "..." --scope internal/service --scope cmd --non-goal "..." --invariant "..." --blast-radius "..." --test-plan "..." --rollback-plan "..."
sophia cr contract show <id>
```

Behavior:

* Stores structured intent contract fields in CR metadata
* Supports partial updates and records `contract_updated` audit events
* Uses contract scope prefixes for drift checks during validation/merge

---

### Impact and Validation

```
sophia cr impact <id>
sophia cr validate <id>
```

Behavior:

* `impact` computes deterministic risk tier/score and blast-radius signals from diff metadata
* `validate` enforces required contract fields and scope-drift policy
* `validate` emits blocking `Errors` and non-blocking `Warnings`
* `validate` records a `cr_validated` audit event

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
Sophia-Intent: Add billing retries
Sophia-Tasks: 2 completed
```

* Mark CR as merged in local metadata
* Delete branch by default (`--keep-branch` to retain)
* Supports emergency audited bypass:

```
sophia cr merge <id> --override-reason "hotfix required for production outage"
```

---

### Workflow Integrity + Visibility

```
sophia doctor
sophia log
sophia repair
sophia hook install
sophia cr current
sophia cr switch <id>
sophia cr reopen <id>
sophia cr task contract set <cr-id> <task-id> --intent "..."
sophia cr task contract show <cr-id> <task-id>
sophia cr edit <id> --title "..."
sophia cr contract set <id> --why "..."
sophia cr contract show <id>
sophia cr impact <id>
sophia cr validate <id>
sophia cr redact <id> --note-index 1 --reason "..."
sophia cr history <id>
```

* `doctor` flags workflow drift (dirty tree, non-CR branch, stale merged CR branches)
* `log` shows intent-first CR history and can reconstruct merged CRs from Git commit metadata
* `repair` rebuilds missing local CR metadata from Git history and realigns CR IDs
* `hook install` adds a pre-commit guard against direct commits on the base branch
* `current/switch/reopen` supports quick branch context moves
* `task contract` enforces subtask intent + acceptance + scope before completion
* `contract/impact/validate` provide intent integrity and blast-radius review context
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
