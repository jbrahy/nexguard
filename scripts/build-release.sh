#!/usr/bin/env bash
# Build the avtool release archives for every published platform.
#
# Usage: scripts/build-release.sh v1.1.0 [output-dir]     (default dir: dist)
#
# Produces one archive per platform plus checksums.txt, matching the layout
# of the archives already on the releases page: the avtool binary, README.md
# and LICENSE, flat, with no wrapping directory. modernc.org/sqlite is pure
# Go, so CGO_ENABLED=0 cross-compiles all of these from any one machine with
# no C toolchain.
#
# Refuses to build from a dirty tree or from a commit that is not the tag
# being released, so a published archive always corresponds to a commit
# anyone can check out. Set ALLOW_DIRTY=1 for a local test build.
#
# Upload with:
#   gh release create v1.1.0 --title "avtool v1.1.0" --notes-file notes.md dist/*

set -euo pipefail

VERSION="${1:-}"
OUT_DIR="${2:-dist}"

if [ -z "$VERSION" ]; then
  echo "usage: $0 <version> [output-dir]   (e.g. $0 v1.1.0)" >&2
  exit 1
fi

cd "$(git rev-parse --show-toplevel)"

if [ "${ALLOW_DIRTY:-0}" != "1" ]; then
  if [ -n "$(git status --porcelain)" ]; then
    echo "working tree is dirty; commit or stash first (ALLOW_DIRTY=1 to override)" >&2
    exit 1
  fi
  if ! git rev-parse -q --verify "refs/tags/$VERSION" >/dev/null; then
    echo "tag $VERSION does not exist; create it before building the release" >&2
    exit 1
  fi
  if [ "$(git rev-parse HEAD)" != "$(git rev-parse "$VERSION^{}")" ]; then
    echo "HEAD is not $VERSION; check out the tag before building" >&2
    exit 1
  fi
fi

PLATFORMS="darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64"

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

for platform in $PLATFORMS; do
  goos="${platform%/*}"
  goarch="${platform#*/}"
  stage="$OUT_DIR/stage_${goos}_${goarch}"
  binary="avtool"
  [ "$goos" = "windows" ] && binary="avtool.exe"

  mkdir -p "$stage"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-X main.version=$VERSION" -o "$stage/$binary" ./cmd/avtool
  cp README.md LICENSE "$stage/"

  if [ "$goos" = "windows" ]; then
    (cd "$stage" && zip -q "../avtool_${VERSION}_${goos}_${goarch}.zip" "$binary" README.md LICENSE)
  else
    tar -czf "$OUT_DIR/avtool_${VERSION}_${goos}_${goarch}.tar.gz" -C "$stage" "$binary" README.md LICENSE
  fi

  rm -rf "$stage"
  echo "built ${goos}/${goarch}"
done

# shasum is the macOS spelling, sha256sum the Linux one. Both emit the
# "<hash>  <filename>" format that `shasum -c` / `sha256sum -c` reads back.
if command -v shasum >/dev/null 2>&1; then
  (cd "$OUT_DIR" && shasum -a 256 avtool_"${VERSION}"_* | sort -k2 > checksums.txt)
else
  (cd "$OUT_DIR" && sha256sum avtool_"${VERSION}"_* | sort -k2 > checksums.txt)
fi

echo
echo "--- $OUT_DIR ---"
ls -l "$OUT_DIR"
