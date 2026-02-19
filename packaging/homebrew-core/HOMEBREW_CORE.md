# Homebrew-core Maintainer Guide

This directory contains a formula template and release/update checklist for submitting Sophia to Homebrew-core.

## Prerequisites

- A tagged GitHub release exists (`vX.Y.Z`) with artifacts from `.goreleaser.yaml`.
- Release contains `checksums.txt`.
- `sophia version` works locally and in release artifacts.

## Artifact expectations

GoReleaser publishes:

- `sophia_vX.Y.Z_darwin_arm64.tar.gz`
- `sophia_vX.Y.Z_darwin_amd64.tar.gz`
- `sophia_vX.Y.Z_linux_arm64.tar.gz`
- `sophia_vX.Y.Z_linux_amd64.tar.gz`
- `checksums.txt`

## Initial submission flow

1. Copy formula template:
   - `packaging/homebrew-core/sophia.rb`
2. Replace placeholder SHA values using `checksums.txt`.
3. Run local smoke check after install:

```bash
brew install ./sophia.rb
sophia version
```

Expected: output includes version, commit, and build date.

4. Open PR against Homebrew/homebrew-core with:
   - formula file,
   - release tag URL references,
   - checksum values from release artifacts.

## Version bump flow

For each new release:

1. Update `version "X.Y.Z"` in formula.
2. Update each per-arch tarball URL.
3. Replace all SHA256 values from new `checksums.txt`.
4. Run:

```bash
brew uninstall --ignore-dependencies sophia || true
brew install ./sophia.rb
sophia version
```

5. Submit update PR to Homebrew-core.

## Notes

- Keep formula URLs and archive names aligned with `.goreleaser.yaml`.
- Keep `sophia version` as the post-install smoke check in docs and release notes.
