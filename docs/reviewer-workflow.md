# Reviewer Workflow

Review Sophia changes by intent and evidence first, then inspect diffs where needed.

## Reviewer checklist

1. Confirm contract quality and task completion state.

```bash
sophia cr status [<cr-id>|<cr-uid>]
```

Optional: open a read-only browser report for the same CR context.

```bash
sophia cr show [<cr-id>|<cr-uid>]
```

2. Verify policy/contract validity.

```bash
sophia cr validate [<cr-id>|<cr-uid>]
```

3. Evaluate trust verdict and required actions.

```bash
sophia cr review [<cr-id>|<cr-uid>]
```

4. Inspect evidence coverage.

```bash
sophia cr evidence show <cr-id>
```

## How to interpret review results

- `required_actions`: blocking items. Treat as must-fix before merge.
- `attention_actions`: advisory improvements. Non-blocking unless policy says otherwise.
- Trust verdict: use as deterministic confidence signal, not as a replacement for judgment.

## When to request more evidence

Request additional evidence when:

- contract acceptance criteria reference commands but no matching `command_run` entries exist,
- scope is broad and risk is non-trivial,
- validation warnings indicate possible drift,
- high-impact files changed with limited test signal.

## Suggested evidence policy for reviews

- For targeted behavior changes: require targeted test logs and command metadata.
- For wide or risky changes: require full suite evidence and validation/review outputs.
- For release and packaging changes: require artifact generation logs and checksum outputs.

## Merge readiness quick gate

Merge when all are true:

1. `sophia cr validate [<cr-id>|<cr-uid>]` has no errors.
2. `sophia cr review [<cr-id>|<cr-uid>]` has no unresolved required actions.
3. evidence exists for contract-required checks.
4. `sophia cr status [<cr-id>|<cr-uid>]` shows all tasks complete and no merge blockers.
