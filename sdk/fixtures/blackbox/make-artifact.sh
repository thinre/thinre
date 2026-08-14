#!/bin/sh
# Build a blackbox release artifact: blackbox-<version>.tar.gz containing
# VERSION and a payload file, plus its SHA-256 for the release definition.
# Usage: make-artifact.sh <version> [output-dir]
set -eu

VERSION="${1:?usage: make-artifact.sh <version> [output-dir]}"
OUT_DIR="${2:-.}"

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

printf '%s\n' "$VERSION" > "$STAGE/VERSION"
printf 'blackbox payload for version %s\n' "$VERSION" > "$STAGE/payload"

ARTIFACT="$OUT_DIR/blackbox-$VERSION.tar.gz"
tar -czf "$ARTIFACT" -C "$STAGE" VERSION payload

# sha256sum on Linux/git-bash; shasum fallback for macOS.
if command -v sha256sum >/dev/null 2>&1; then
    SHA=$(sha256sum "$ARTIFACT" | cut -d' ' -f1)
else
    SHA=$(shasum -a 256 "$ARTIFACT" | cut -d' ' -f1)
fi

echo "artifact: $ARTIFACT"
echo "sha256:   $SHA"
