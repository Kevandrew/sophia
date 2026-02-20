#!/usr/bin/env bash

set -euo pipefail

REPO="${SOPHIA_REPO:-Kevandrew/sophia}"
TAG="${SOPHIA_VERSION:-}"
INSTALL_DIR="${SOPHIA_INSTALL_DIR:-/usr/local/bin}"

if ! command -v curl >/dev/null 2>&1; then
  echo "error: curl is required" >&2
  exit 1
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux|darwin) ;;
  *)
    echo "error: unsupported OS: $os" >&2
    exit 1
    ;;
esac

arch_raw="$(uname -m)"
case "$arch_raw" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "error: unsupported architecture: $arch_raw" >&2
    exit 1
    ;;
esac

if [[ -z "$TAG" ]]; then
  TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n1)"
  if [[ -z "$TAG" ]]; then
    echo "error: failed to resolve latest release tag" >&2
    exit 1
  fi
fi

if [[ "$TAG" != v* ]]; then
  TAG="v${TAG}"
fi

version="${TAG#v}"
asset="sophia_${version}_${os}_${arch}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${TAG}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

archive="$tmpdir/$asset"
checksums="$tmpdir/checksums.txt"

echo "Downloading $asset from $TAG..."
curl -fsSL "$base_url/$asset" -o "$archive"
curl -fsSL "$base_url/checksums.txt" -o "$checksums"

expected_sha="$(grep "  ${asset}$" "$checksums" | awk '{print $1}')"
if [[ -z "$expected_sha" ]]; then
  echo "error: checksum not found for $asset" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  echo "${expected_sha}  ${archive}" | sha256sum -c -
elif command -v shasum >/dev/null 2>&1; then
  echo "${expected_sha}  ${archive}" | shasum -a 256 -c -
else
  echo "error: neither sha256sum nor shasum is available" >&2
  exit 1
fi

tar -xzf "$archive" -C "$tmpdir"
if [[ ! -f "$tmpdir/sophia" ]]; then
  echo "error: sophia binary not found in archive" >&2
  exit 1
fi

target_dir="$INSTALL_DIR"
if [[ ! -w "$target_dir" ]]; then
  target_dir="$HOME/.local/bin"
  mkdir -p "$target_dir"
fi

install -m 0755 "$tmpdir/sophia" "$target_dir/sophia"

echo "Installed sophia to $target_dir/sophia"
"$target_dir/sophia" version || true

if [[ ":$PATH:" != *":$target_dir:"* ]]; then
  echo "Add this to your shell profile:"
  echo "  export PATH=\"$target_dir:\$PATH\""
fi
