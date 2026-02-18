# Workflow Lifecycle

Sophia centers work around intent-bearing CRs rather than ad hoc commits.

## Lifecycle Overview

1. Open CR intent
2. Define CR contract
3. Decompose into tasks
4. Define task contracts
5. Implement and checkpoint task-by-task
6. Validate and review
7. Merge or recover from conflicts

## 1) Open Intent

```bash
sophia cr add "<title>" --description "<why>"
```

`cr add` does not switch by default. Enter CR context explicitly:

```bash
sophia cr switch <cr-id>
```

## 2) Define the CR Contract

Typical required fields (policy-controlled):
- `why`
- `scope`
- `non_goals`
- `invariants`
- `blast_radius`
- `test_plan`
- `rollback_plan`

Set/update contract:

```bash
sophia cr contract set <cr-id> \
  --why "..." \
  --scope . \
  --non-goal "..." \
  --invariant "..." \
  --blast-radius "..." \
  --test-plan "go test ./... && go vet ./..." \
  --rollback-plan "Revert CR merge commit"
```

## 3) Task Decomposition and Contracts

```bash
sophia cr task add <cr-id> "<task title>"
sophia cr task contract set <cr-id> <task-id> \
  --intent "..." \
  --acceptance "..." \
  --scope <prefix>
```

Task contracts must be complete before task completion.

## 4) Checkpointing and Progress

Preferred mode:

```bash
sophia cr task done <cr-id> <task-id> --from-contract
```

Checkpoint commits are generated as task progress artifacts.

## 5) Validate, Review, and Merge

```bash
sophia cr validate <cr-id>
sophia cr review <cr-id>
sophia cr merge <cr-id>
```

Use `sophia cr status <cr-id>` as a compact readiness snapshot.

## 6) Merge Conflict Recovery

If merge enters conflict state:

```bash
sophia cr merge status <cr-id>
# resolve conflicts manually
sophia cr merge resume <cr-id>
# or cancel
sophia cr merge abort <cr-id>
```

During unresolved merge state, many mutating CR commands are intentionally blocked until resume/abort completes.

## 7) Stack and Delegation Patterns

For larger efforts:

- Create child CRs (`cr child add` or `cr add --parent`)
- Use `sophia cr task delegate` for parent task ownership via child CRs
- Use `sophia cr stack` and `sophia cr restack` for topology maintenance

## Related Docs

- Command map: [`cli-reference.md`](cli-reference.md)
- Policy model: [`repository-policy.md`](repository-policy.md)
- Branch identity details: [`branch-identity.md`](branch-identity.md)
