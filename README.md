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
4. Generates clean, structured commits on merge
5. Formats review output around intent, not diffs

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

## CLI Commands (Initial Implementation Target)

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
sophia cr task done <cr-id> <task-id>
```

Behavior:

* Creates a checkpoint commit for current CR branch changes (`git add -A` + commit)
* Marks task done only if checkpoint commit succeeds
* Records checkpoint metadata on the task (`commit`, `timestamp`, message)

Optional metadata-only completion:

```
sophia cr task done <cr-id> <task-id> --no-checkpoint
```

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

* Squash CR branch to one intent commit
* Fast-forward merge into base branch
* Generate structured commit message:

```
[CR-1] Add billing retries

- Refactored payment client
- Added exponential backoff
```

* Mark CR as merged in local metadata
* Delete branch by default (`--keep-branch` to retain)

---

### Workflow Integrity + Visibility

```
sophia doctor
sophia log
sophia cr current
sophia cr switch <id>
```

* `doctor` flags workflow drift (dirty tree, non-CR branch, stale merged CR branches)
* `log` shows intent-first CR history and can reconstruct merged CRs from Git commit metadata
* `current/switch` supports quick branch context moves
* `repair` rebuilds missing local CR metadata from Git commit history

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

If you stop using `git commit -m "WIP"`,
Sophia is working.
