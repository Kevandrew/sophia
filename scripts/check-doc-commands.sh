#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

bin="$tmpdir/sophia"
go build -o "$bin" ./cmd/sophia

declare -a command_surfaces=(
  ""
  "version"
  "init"
  "doctor"
  "repair"
  "cr"
  "cr add"
  "cr switch"
  "cr status"
  "cr validate"
  "cr review"
  "cr merge"
  "cr merge status"
  "cr merge resume"
  "cr merge abort"
  "cr task add"
  "cr task contract set"
  "cr task done"
  "cr task chunk list"
  "cr task chunk show"
  "cr task chunk export"
  "cr evidence add"
  "cr evidence show"
  "cr export"
  "cr import"
  "cr patch preview"
  "cr patch apply"
  "cr reconcile"
  "cr check status"
)

missing=0
for surface in "${command_surfaces[@]}"; do
  if [[ -z "$surface" ]]; then
    args=(--help)
    label="sophia"
  else
    read -r -a parts <<<"$surface"
    args=("${parts[@]}" "--help")
    label="sophia $surface"
  fi

  if "$bin" "${args[@]}" >/dev/null 2>&1; then
    printf 'ok   %s\n' "$label"
  else
    printf 'fail %s\n' "$label"
    missing=$((missing + 1))
  fi
done

if [[ "$missing" -gt 0 ]]; then
  printf '\n%d documented command surface(s) failed help checks.\n' "$missing" >&2
  exit 1
fi

printf '\nAll documented command surfaces are available.\n'
