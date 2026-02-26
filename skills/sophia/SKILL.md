---
name: sophia
description: Operate Sophia CLI as an intent-first abstraction over Git for local repositories. Use when the user wants to initialize Sophia, open/manage Change Requests (CRs), define CR and task contracts, checkpoint task progress with explicit scope (including contract-derived scope), inspect impact/validation/review/status/why context, consume JSON outputs, repair metadata from Git history, and merge CRs safely.
---

# Sophia

## Overview
Use Sophia as the primary workflow interface and Git as the execution engine.

Principle:
1. Open intent with a CR.
2. Immediately define CR contract and task contracts.
3. Use explicit CR navigation for mutations (`cr switch`), and ID-based reads for context.
4. Implement using task-scoped checkpoints.
5. Validate impact/risk and stack/delegation health before merge.
6. Merge as one intent-rich CR commit.

## What "Good" Looks Like (CR Authoring Depth)
Sophia only works as well as the intent you write down. Default to **decision-complete** CRs and task contracts so another engineer (or agent) can implement without guessing.

Target qualities:
- **Outcome-first**: contract `why` states the user-visible or system outcome and the decision boundary.
- **Boundaries are explicit**: `scope`, `non_goals`, and `invariants` are concrete and testable.
- **Blast radius is enumerated**: call out CLI surfaces, data surfaces, and failure modes.
- **Tasks are review units**: each task is small enough to checkpoint with explicit scope and verify independently.

Smells of an under-specified CR:
- `why` is “implement X” (implementation step), not “achieve outcome Y under constraints Z”.
- `blast_radius` is empty or vague (“touches a few files”).
- tasks are layer buckets (“update service layer”) rather than behavior slices.
- no explicit rollback or test plan.

## Lifecycle Defaults
Treat this as the default operating sequence unless the user asks for something else.

After `sophia cr add`:
1. Set CR contract details with `sophia cr contract set` (`why`, `scope`, and other relevant fields like risk/test/rollback).
2. Add subtasks with `sophia cr task add`.
3. Set each task contract with `sophia cr task contract set` (`intent`, `acceptance`, `scope`) before implementation/checkpointing.

After implementation is complete:
1. Complete each finished task with an explicit checkpoint scope:
   - chunk-scoped: inspect with `sophia cr task chunk list <id> <task-id>` and checkpoint via `sophia cr task done <id> <task-id> --patch-file <patch-file>`
   - file-scoped: `sophia cr task done <id> <task-id> --path <file> [--path <file>]`
   - contract-scoped fallback: `sophia cr task done <id> <task-id> --from-contract`
   - metadata-only exception: `sophia cr task done <id> <task-id> --no-checkpoint --no-checkpoint-reason "<why no checkpoint>"`
2. Run `sophia cr validate <id>` (use `--record` when an audit validation event is desired).
3. Run `sophia cr review <id>`.
4. Check `sophia cr status <id>` / `--json` and `sophia cr check status <id> --json` as needed, then merge.

## Evidence Chain (Tests, Logs, Attachments)
Treat evidence as part of the SDLC output, not just a “nice to have”. The contract and tasks should define what needs to be proven, and the evidence ledger should contain the proof.

Evidence sources Sophia can use:
- Task checkpoint commits (scope + chunk metadata + commit footers)
- Validation outputs (`sophia cr validate`)
- Review outputs (`sophia cr review`)
- Command outputs (tests, build, lint, reproductions) recorded as evidence entries

When a CR/task contract names specific commands (for example targeted tests), prefer recording those runs as evidence:
1. Run the command(s) exactly as written in the contract or acceptance criteria.
2. Capture raw output to a file (logs are fine to keep under a local docs path, e.g. `_docs/cr-<id>-evidence/...`).
3. Attach the log(s) to the CR evidence ledger as `command_run` entries.

Commands:
```bash
# attach a command run (tests, build, etc.)
sophia cr evidence add <id> \
  --type command_run \
  --summary "Targeted tests: <what/where>" \
  --cmd "<exact command run>" \
  --exit-code 0 \
  --attachment <path-to-log>

# verify evidence entries are present
sophia cr evidence show <id> --json
```

Guidance:
- Record targeted tests as evidence when new tests are added or acceptance criteria mention specific test selectors.
- Record full-suite runs as evidence when the contract or policy requires them (or when targeted coverage is insufficient to justify risk).

## CR Contract Writing Template (High-Signal Defaults)
Write the CR contract so a reviewer can trust intent and risk without reading raw diffs.

Suggested minimum contract content:
- `why`: 2-6 paragraphs
  - Outcome: what changes for the user/system.
  - Decision boundary: what is in-scope vs out-of-scope (and why).
  - Success criteria: what “done” means in observable terms.
- `scope`: explicit path prefixes; prefer capability-oriented prefixes (avoid `.` unless necessary).
- `non_goals`: 3-8 bullets of “not doing X” that prevents scope creep.
- `invariants`: 2-6 bullets of safety properties that must remain true.
- `blast_radius`: concrete list of affected surfaces plus failure modes.
- `test_plan`: the exact commands expected to run before merge.
- `rollback_plan`: the exact rollback action (typically revert merge commit).
- `risk_*`: include when scope touches high-impact areas or correctness/security surfaces.

High-quality `blast_radius` checklist:
- Interfaces: user-facing surfaces whose behavior/output changes (commands, APIs, UIs, config, job runners).
- Data: CR YAML fields, JSON output fields, commit footers/refs touched (as applicable).
- Workflow: merge gates, validation rules, scope drift behavior, delegated flows.
- Failure modes: what can go wrong; how we detect; how we recover.
- Compatibility: backward compatibility expectations for existing repos/metadata.

Concrete example phrasing (generic placeholders):
- `why`: “Enable collaboration via portable artifacts (bundles + patches) while keeping the core workflow tool network-agnostic.”
- `invariants`: “No silent overwrites; conflicts must be explicit and machine-readable.”
- `non_goals`: “No platform auth/URLs in the core workflow tool; a separate companion tool owns networking.”

## Task Contract Writing Template (Small, Checkpointable Units)
Task contracts should map cleanly to a single checkpoint commit.

Rules of thumb:
- `intent`: one behavior outcome (not “refactor X”).
- `acceptance_criteria`: 2-6 observable conditions.
- `scope`: exact file paths or stable prefixes; keep it narrow to enable `--from-contract` checkpointing.

Common acceptance criteria patterns:
- “Reject invalid inputs deterministically with structured errors.”
- “JSON output is deterministic across runs for unchanged state.”
- “Conflicting edits produce explicit conflict records; no partial writes.”

## CR Decomposition Guidance
Prefer behavior slices over layers. Good task splits:
- “Define canonical doc + fingerprint” (model/service)
- “Extend export payload + mappers” (service/output)
- “Implement patch apply + conflict report” (service/store)
- “Wire new entrypoints” (command/UI/HTTP layer as applicable)
- “Add tests for success/conflict flows” (service/cli)
- “Update docs” (docs)

Avoid:
- “Update service layer”
- “Add tests” as one giant task (split by behavior surface)

## Response Focus
Keep responses intent-first.

- Do not proactively mention constraints/non-goals/invariants in chat output unless they materially affect the current decision, risk, or blocker.
- Avoid repeating the same stable non-goals in every CR update unless they changed or explain a concrete blocker.
- Mention constraints only when:
  - the user explicitly asks for them,
  - they change implementation choices,
  - they explain a validation/merge failure,
  - they are being updated as part of the CR.

## Current CLI Semantics (CR-40+)
Treat these as current UX defaults unless `--help` shows otherwise:

- `sophia cr add` and `sophia cr child add` do **not** switch branches by default.
  - Use `--switch` for immediate checkout.
  - Otherwise run `sophia cr switch <id>` before mutation commands.
- Read/context commands that take explicit CR IDs are branch-agnostic.
  - Example surfaces: `status`, `impact`, `validate`, `review`, `diff`, `range`, `rev-parse`, `pack`, `history`, `task list/diff/rangediff`.
- Mutation commands still require proper CR context and state guardrails.
- `sophia cr refresh <id>` is the canonical sync abstraction over restack/rebase decisions.
- `sophia cr list` and `sophia cr search` both exist; use `search` for query/filter intent.
- `sophia cr list --search` is an alias for `--text`; `--status`/`--risk-tier` reject invalid enum values.
- `sophia cr validate` is read-only by default; add `--record` when an audit event is desired.
- Operational root/CR/task commands now expose `--json` envelope output; still confirm leaf `--help` for exact payload shape.

## Branch Freshness Guard (Avoid Outdated CR Work)
Before implementing, reviewing, or merging an existing CR, verify branch freshness against base and remote base.

Required checks:

1. Confirm CR/base context:
```bash
sophia cr status <id> --json
```
- Read `data.base_ref` and `data.base_commit`.
- Ensure you are on the intended CR branch before mutating.

2. Check CR branch vs local base:
```bash
git rev-list --left-right --count <base_ref>...HEAD
```
- If left count (`<`) is greater than `0`, the CR is behind local base and considered stale.

3. Check local base vs remote base (when remote exists):
```bash
git fetch origin <base_ref>
git rev-list --left-right --count <base_ref>...origin/<base_ref>
```
- If right count (`>`) is greater than `0`, local base is behind remote base.

Recommended action policy:
- If CR is behind local base: suggest `sophia cr refresh <id>` first (preferred abstraction).
- If needed for explicit control: use `sophia cr restack <id>` or `sophia cr base set <id> --ref <base_ref> --rebase`.
- If local base is behind remote: update base first (`git checkout <base_ref> && git pull --ff-only`), then refresh/restack the CR.
- Do not proceed with substantial implementation on a stale CR unless the user explicitly asks to continue as-is.

## Managed Remote Collaboration (HQ)
This is the agent-first collaboration loop for syncing CR *intent metadata* (not code/branches) with a managed remote.

Setup (repo + user):

```bash
# configure repo identity (required)
sophia hq config set --repo-id <org/repo>

# optional (defaults to https://sophiahq.com)
sophia hq config set --base-url https://sophiahq.com

# store token (per-user)
printf '%s' "$SOPHIAHQ_TOKEN" | sophia hq login --token-stdin
```

Daily loop (status-first; deterministic):

```bash
sophia cr status <id|uid> --hq --json
```

Interpret `data.hq_sync.state`:

- `not_configured`: run `sophia hq config set ...` and `sophia hq login --token-stdin`
- `remote_missing`: run `sophia cr push <id|uid>`
- `unlinked`: run `sophia cr pull <id|uid>` (links upstream intent fingerprint)
- `local_ahead`: run `sophia cr push <id|uid>`
- `remote_ahead`: run `sophia cr pull <id|uid>`
- `up_to_date`: no action
- `diverged`: do not guess. Surface conflict details and ask the user whether to:
  - accept remote as canonical: `sophia cr pull <id|uid> --force`
  - overwrite remote as canonical: `sophia cr push <id|uid> --force`
  - reconcile in the remote UI, then pull again

Error playbooks (JSON code -> next action):

- `hq_upstream_moved`: run `sophia cr pull <id|uid> --json` (then retry push if needed).
- `hq_intent_diverged`: surface `details.conflicts` and `details.local_changed_fields/remote_changed_fields`; do not auto-force without user approval.
- `hq_patch_conflict`: pull latest, then retry push; if conflicts persist, reconcile in UI.
- `hq_malformed_response`: stop; the remote is missing required fields or is incompatible.

## Creating Detailed CRs (Recommended Agent Behavior)
When a user asks to “make a CR” or “write a CR plan”, bias toward generating:
1. A crisp CR title and description (why).
2. A full CR contract including `blast_radius`, `test_plan`, `rollback_plan`, and relevant risk hints.
3. 8-14 tasks for cross-cutting changes (interfaces + core logic + storage/integration + tests + docs), each with task contract.
4. Task scopes that make checkpointing obvious (`--from-contract` should be viable).

If the user’s request is ambiguous, do not fill gaps with vibes. Ask for missing decisions that materially affect:
- public interface/schema (CLI flags, JSON fields, file formats),
- conflict/compat behavior,
- scope boundaries and invariants.

## Help-First Discovery Protocol
When command shape is uncertain, navigate help top-down before choosing a write action.

1. `sophia --help` for root workflow path.
2. `sophia cr --help` for CR command grouping.
3. `sophia cr <leaf-command> --help` for exact flags/usage.

Preferred first-run path:

```bash
sophia init
sophia cr add "<title>" --description "<why>"     # stays on current branch by default
sophia cr switch <id>                              # explicit navigation for mutation flow
sophia cr contract set <id> --why "<intent>" --scope <prefix>
sophia cr task add <id> "<task>"
sophia cr task contract set <id> <task-id> --intent "<intent>" --acceptance "<criterion>" --scope <prefix>
sophia cr task chunk list <id> <task-id>
sophia cr task done <id> <task-id> --path <file>            # or --patch-file <patch-file>, or --from-contract
sophia cr validate <id> --record
sophia cr review <id>
sophia cr status <id>
sophia cr merge <id>
```

## Contract Quality
Write contracts so reviewers can trust intent/risk context without reading raw diffs.

- `why` should explain outcome and decision boundary, not implementation steps.
- `blast_radius` should be specific and concrete:
  - list affected command surfaces (commands/flags/output fields),
  - list affected data surfaces (CR/task YAML fields, commit footers, status JSON),
  - list merge/review behavior changes (new blockers, ordering rules, fallback behavior),
  - list main failure modes and rollback scope.
- Keep `non_goals` short and non-redundant; include only constraints that clarify what is intentionally excluded for this CR.

## Plan YAML Authoring (When Using `sophia cr apply`)
When authoring a YAML plan:
- Keep CR `key` stable (used for delegation references).
- Use explicit `parent_key` for stacked flows and keep delegations only to direct children.
- Put CR-level contract in the plan when possible so created CRs are immediately merge-reviewable.
- Give each task a `key` and keep it stable; use contracts to enable `--from-contract`.
- Avoid broad scopes (`.`) unless the CR truly touches many unrelated paths.

Plan quality checklist:
- Each task has a contract (`intent`, `acceptance_criteria`, `scope`).
- Scopes are normalized and repo-relative (no globs, no `../`, no absolute paths).
- Delegation edges are intentional and topologically consistent.

## Task Decomposition Standards
Treat tasking as a review artifact, not a checkbox.

- Prefer behavior slices over layer buckets. Avoid umbrella tasks like "implement service changes".
- Size tasks so each one can be checkpointed and reviewed independently.
- For medium CRs, target roughly `6-10` tasks.
- For large cross-layer CRs (for example CLI + service + gitx + store + migration + tests/docs), target roughly `10-18` tasks.
- If implementation scope expands after opening a CR, add tasks immediately instead of overloading existing ones.
- Each task contract should be specific:
  - `intent`: one behavior outcome.
  - `acceptance`: observable pass condition.
  - `scope`: exact files or stable path prefixes.
- Keep a direct mapping between task contracts and checkpoint commits. One checkpoint should correspond to one completed task whenever possible.

## CR Split Policy (Stacked Intent)
For very large efforts, do not keep expanding one CR forever. Split into stacked CRs with explicit parent/child intent.

- Use one anchor CR for shared primitives or migration scaffolding.
- Put each independently reviewable behavior change in its own child CR.
- Prefer multi-CR stacks when any of these is true:
  - the work spans multiple independent user-visible outcomes,
  - projected tasks exceed roughly `12-15` with weak coupling,
  - expected touched files exceed roughly `20` across unrelated areas,
  - review quality drops because one CR mixes infra, behavior, and follow-up refactors.
- Keep each CR mergeable on its own contract and tests.
- Use delegation when parent tasks are fulfilled by child CRs.

Recommended split flow:
1. Open anchor CR with shared groundwork only.
2. Create child CRs using `--parent` (or `cr child add`) per intent slice.
3. Delegate parent tasks to child CRs when ownership crosses CR boundaries.
4. Merge children first when delegated rules allow, then merge parent.

Recommended task taxonomy for larger CRs:
1. Repository/repo-context discovery and path resolution.
2. Data model/schema/store changes.
3. Git integration/parsing/execution behavior.
4. Service orchestration/decision logic.
5. CLI wiring, flags, user-facing errors/warnings.
6. Migration/backward compatibility behavior.
7. Unit tests by layer.
8. Integration/e2e workflow tests.
9. Docs and operator guidance.

When using `sophia cr apply --file <plan.yaml>`:
- Define task-level chunks up front, with explicit files per task.
- Avoid "catch-all" tasks that own unrelated files.
- If a file is shared across tasks, split by hunk and keep chunk ownership explicit.
- Before completion, verify scope with:
```bash
sophia cr task chunk list <cr-id> <task-id>
```

## Review Score Interpretation
Do not treat `sophia cr review` score alone as merge readiness.

- A `100/100` score can still hide weak task decomposition or oversized checkpoints.
- Always cross-check:
  - changed surface area (`git diff --stat`),
  - number and granularity of tasks (`sophia cr task list <id>`),
  - test coverage breadth (`go test ./...` plus targeted integration tests),
  - validation outputs (`sophia cr validate <id>`).
- Interpret trust outputs explicitly:
  - `required_actions`: deterministic blockers that must be resolved to satisfy trust requirements.
  - `attention_actions`: non-blocking next steps when verdict is `needs_attention`.
  - `cr check status/run` with `check_mode=none`: no required checks are configured yet; follow guidance to add policy/task check requirements.
- If files changed materially exceed task granularity, add/refine tasks before merge.

## Core Workflow
Run from repository root.

1. Initialize:
```bash
sophia init
# optional
sophia init --base-branch <name> --metadata-mode local|tracked
# seeds .sophia/cr-plan.sample.yaml when missing
```

2. Open and structure work:
```bash
sophia cr apply --file <plan.yaml> [--dry-run] [--json] [--keep-file]
sophia cr add "<title>" --description "<why>"              # default: no switch
sophia cr add "<title>" --description "<why>" --switch     # opt-in immediate switch
sophia cr add "<title>" --description "<why>" --base <git-ref>
sophia cr add "<title>" --description "<why>" --parent <cr-id>
sophia cr child add "<title>" --description "<why>" # child from current CR context
sophia cr child add "<title>" --description "<why>" --switch
sophia cr switch <cr-id> # explicit navigation into CR branch context
sophia cr list
sophia cr list [--status <in_progress|merged>] [--risk-tier <low|medium|high>] [--scope <prefix>] [--text <text>] [--search <text>] [--json]
sophia cr search [query] [--status <in_progress|merged>] [--risk-tier <low|medium|high>] [--scope <prefix>] [--text <text>] [--search <text>] [--json]
sophia cr current
sophia cr stack [<id>] [--json]
sophia cr base set <cr-id> --ref <git-ref> [--rebase]
sophia cr restack <cr-id>
sophia cr refresh <cr-id> [--dry-run] [--strategy auto|restack|rebase]
sophia cr contract set <cr-id> \
  --why "<intent>" \
  --scope <prefix> --scope <prefix> \
  --non-goal "No unrelated refactors." \
  --invariant "Existing behavior remains compatible." \
  --risk-critical-scope <prefix> --risk-critical-scope <prefix> \
  --risk-tier-hint <low|medium|high> \
  --risk-rationale "<why elevated floor is justified>" \
  --blast-radius "<affected area>" \
  --test-plan "go test ./... && go vet ./..." \
  --rollback-plan "Revert the CR merge commit."

sophia cr task add <cr-id> "<subtask>"
sophia cr task contract set <cr-id> <task-id> \
  --intent "<task intent>" \
  --acceptance "<acceptance criterion>" \
  --scope <prefix> --scope <prefix>
sophia cr task delegate <parent-cr-id> <task-id> --child <child-cr-id>
sophia cr task undelegate <parent-cr-id> <task-id> --child <child-cr-id>

sophia cr note <cr-id> "<decision/progress note>"
sophia cr evidence add <cr-id> --type manual_note --summary "<evidence>"
sophia cr evidence sample add <cr-id> --scope <prefix> --summary "<review sample>"
sophia cr evidence sample list <cr-id> [--json]
sophia cr evidence show <cr-id> [--json]
```

3. Execute task checkpoints:
```bash
# preferred: task-contract scoped checkpoint
sophia cr task done <cr-id> <task-id> --from-contract

# explicit file-scoped checkpoint
sophia cr task done <cr-id> <task-id> --path <file> --path <file>

# explicit stage-all fallback
sophia cr task done <cr-id> <task-id> --all

# patch-manifest checkpoint (hunk-scoped)
sophia cr task done <cr-id> <task-id> --patch-file <patch-file>

# metadata-only completion
sophia cr task done <cr-id> <task-id> --no-checkpoint --no-checkpoint-reason "<why no checkpoint>"

# reopen completed task when needed
sophia cr task reopen <cr-id> <task-id> [--clear-checkpoint] [--json]
```

4. Validate and inspect:
```bash
sophia cr contract show <cr-id>
sophia cr why <cr-id>
sophia cr status <cr-id>
sophia cr diff <cr-id> [--task <task-id>] [--critical] [--json]
sophia cr task diff <cr-id> <task-id> [--chunks] [--json]
sophia cr rangediff <cr-id> [--from <ref>] [--to <ref>] [--since-last-checkpoint] [--json]
sophia cr task rangediff <cr-id> <task-id> [--from <ref>] [--to <ref>] [--since-last-checkpoint] [--json]
sophia cr task chunk list <cr-id> <task-id> [--path <file>] [--json]
sophia cr task chunk diff <cr-id> <task-id> <chunk-id> [--json]
sophia cr range <cr-id>
sophia cr rev-parse <cr-id> --kind <base|head|merge-base>
sophia cr pack <cr-id> --json
sophia cr task contract show <cr-id> <task-id>
sophia cr impact <cr-id>
sophia cr impact <cr-id> --json
sophia cr validate <cr-id> [--record]
sophia cr check status <cr-id> [--json]
sophia cr check run <cr-id> [--json]
sophia cr review <cr-id>
sophia cr status <cr-id> --json
sophia cr validate <cr-id> --json
sophia cr review <cr-id> --json
sophia cr task list <cr-id> [--json]
```

5. Metadata hygiene and recovery:
```bash
sophia cr edit <cr-id> --title "..." --description "..."
sophia cr redact <cr-id> --note-index <n> --reason "..."
sophia cr redact <cr-id> --event-index <n> --reason "..."
sophia cr history <cr-id> [--show-redacted]
sophia cr doctor <cr-id> [--json]
sophia cr reconcile <cr-id> [--regenerate] [--json]
sophia cr export <cr-id> [--format json] [--include diffs] [--out <file>] [--json]
sophia cr reopen <cr-id> [--json]
sophia doctor [--limit <n>] [--json]
sophia log [--json]
sophia repair [--base-branch <name>] [--refresh] [--json]
```

6. Merge:
```bash
# default (validation must pass)
sophia cr merge <cr-id>

# optional keep branch
sophia cr merge <cr-id> --keep-branch

# emergency audited override
sophia cr merge <cr-id> --override-reason "<why bypass is necessary>"

# merge conflict recovery flow
sophia cr merge status <cr-id> [--json]
sophia cr merge resume <cr-id> [--keep-branch] [--override-reason "..."] [--json]
sophia cr merge abort <cr-id> [--json]
```

## Guardrails
- Do not edit `.sophia/*.yaml` manually unless explicitly requested.
- Do not run mutating Sophia commands in parallel (for example, two `cr contract set` calls at once); run metadata writes serially.
- Keep one CR focused on one intent outcome.
- If one intent cannot stay reviewable, split into stacked CRs instead of inflating task count in a single CR.
- Keep rationale in CR `description` and contract `why`.
- Task count should match scope. Large, multi-surface CRs should not be compressed into a few broad tasks.
- Keep constraint language minimal by default; include it in user-facing narration only when decision-relevant.
- Add or split tasks as soon as new surfaces appear; do not defer task correction until merge time.
- Set task contracts before `task done`; completion is blocked when task contract is incomplete.
- In checkpoint mode, always provide explicit scope mode (`--from-contract`, `--path`, `--patch-file`, or `--all`).
- Prefer `--from-contract` when task contract scope is defined.
- Pre-staged index changes should be cleared before scoped checkpoints.
- Always run `cr validate` before `cr merge`; override only for explicit emergencies.
- In delegated workflows, use `cr task delegate`/`undelegate` to represent cross-CR task ownership; delegated tasks are not normal checkpoint targets until delegation is resolved.
- When branch context is ambiguous, prefer explicit CR IDs for reads and `cr switch <id>` before writes.
- For agent workflows, prefer machine output (`--json`) where available and treat text mode as operator-facing.
- For agent workflows, prefer setting `SOPHIA_OUTPUT=json` so `--json`-capable commands emit JSON by default; use explicit `--json=false` only when text output is required.
