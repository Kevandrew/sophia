# Branch Identity

Sophia treats branch names as human-friendly aliases and keeps canonical CR identity in metadata (`id`, `uid`), refs, and commit footers.

## Default Alias Format

- `cr-<id>-<slug>`
- optional owner namespace: `<owner>/cr-<id>-<slug>`

Examples:

- `cr-42-branch-identity-redesign`
- `kevandrew/cr-42-branch-identity-redesign`

Legacy branches (`sophia/cr-<id>`) remain supported.

## Alias Controls

Create-time controls:

- `sophia cr add "<title>" --owner-prefix kevandrew`
- `sophia cr add "<title>" --branch-alias kevandrew/cr-42-branch-identity-redesign`
- `sophia cr child add "<title>" --owner-prefix kevandrew`

Repository default owner prefix:

- `sophia init --branch-owner-prefix kevandrew`

Plan apply (`sophia cr apply`) supports optional per-CR fields:

- `owner_prefix`
- `branch_alias`

## Branch Utility Commands

- `sophia cr branch show <id>`
- `sophia cr branch resolve [--branch <name>]`
- `sophia cr branch format --id <id> --title "<title>" [--owner-prefix <owner>]`
- `sophia cr branch migrate <id> [--dry-run]`
- `sophia cr branch migrate --all [--dry-run] [--json]`

## UID and Selector Behavior

Commands that take CR selectors accept:

- numeric CR id (e.g. `42`)
- CR uid (e.g. `cr_c6bec981-b3dc-493d-aa41-897df808126c`)
- branch-style selector when parseable (e.g. `cr-42-branch-identity-redesign`)

## Canonical References and Footers

Sophia maintains:

- `refs/sophia/cr/<id>`
- `refs/sophia/cr/uid/<uid>`

Checkpoint and merge commits include:

- `Sophia-CR`
- `Sophia-CR-UID`
- `Sophia-Branch`
- `Sophia-Branch-Scheme`
