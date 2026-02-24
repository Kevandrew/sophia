# Branch Identity

Sophia treats branch names as human-friendly aliases and keeps canonical CR identity in metadata (`id`, `uid`), refs, and commit footers.

## Default Alias Format

- `cr-<slug>-<uid4>`
- optional owner namespace: `<owner>/cr-<slug>-<uid4>`
- collision fallback extends suffix length automatically (`uid6`, then `uid8`)

Examples:

- `cr-branch-identity-redesign-c6be`
- `kevandrew/cr-branch-identity-redesign-c6be`

Legacy branches (`sophia/cr-<id>`) remain supported.

## Alias Controls

Create-time controls:

- `sophia cr add "<title>" --owner-prefix kevandrew`
- `sophia cr add "<title>" --branch-alias kevandrew/cr-branch-identity-redesign-c6be`
- `sophia cr child add "<title>" --owner-prefix kevandrew`

Repository default owner prefix:

- `sophia init --branch-owner-prefix kevandrew`

Plan apply (`sophia cr apply`) supports optional per-CR fields:

- `owner_prefix`
- `branch_alias`

## Branch Utility Commands

- `sophia cr branch show <id>`
- `sophia cr branch resolve [--branch <name>]`
- `sophia cr branch format --id <id> [--title "<title>"] [--owner-prefix <owner>]`
- `sophia cr branch format --uid <cr-uid> --title "<title>" [--owner-prefix <owner>]`
- `sophia cr branch migrate <id> [--dry-run]`
- `sophia cr branch migrate --all [--dry-run] [--json]`

`cr branch format` behavior:

- existing `--id` with no overrides returns the CR's stored branch alias
- non-existing `--id` + `--title` returns a local preview alias (`cr-<slug>-<id-token>`)

## UID and Selector Behavior

Commands that take CR selectors accept:

- numeric CR id (e.g. `42`)
- CR uid (e.g. `cr_c6bec981-b3dc-493d-aa41-897df808126c`)
- exact branch selector match against stored CR branch (e.g. `cr-branch-identity-redesign-c6be`)

## Canonical References and Footers

Sophia maintains:

- `refs/sophia/cr/<id>`
- `refs/sophia/cr/uid/<uid>`

Checkpoint and merge commits include:

- `Sophia-CR`
- `Sophia-CR-UID`
- `Sophia-Branch`
- `Sophia-Branch-Scheme`

## Related Docs

- Docs index: [`index.md`](index.md)
- Workflow lifecycle: [`workflow.md`](workflow.md)
