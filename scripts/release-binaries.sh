#!/usr/bin/env bash
# Builds the release archives: static binaries (UI embedded) for
# linux/darwin × amd64/arm64, one tar.gz each with the README, LICENSE and the user docs, plus
# SHA256SUMS. Used by .github/workflows/release.yml and runnable locally:
#
#   scripts/release-binaries.sh v0.1.0-beta.1 [outdir]   # default outdir: dist
#
# Expects the web UI to be built already (make web, or the workflow's
# web step), so every archive embeds the same build. Go comes from mise
# when it is available, else from PATH (CI activates mise).
set -euo pipefail

tag=${1:?usage: release-binaries.sh <tag, e.g. v0.1.0-beta.1> [outdir]}
out=${2:-dist}
case $tag in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "tag must look like v0.1.0 or v0.1.0-beta.1, got $tag" >&2; exit 1 ;;
esac
version=${tag#v}
cd "$(dirname "$0")/.."

if [ ! -f web/build/spa/index.html ]; then
  echo "web/build/spa is missing: build the UI first (make web)" >&2
  exit 1
fi
go=(go)
if command -v mise >/dev/null 2>&1; then go=(mise exec -- go); fi

rm -rf "$out"
mkdir -p "$out"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  os=${target%/*} arch=${target#*/}
  name="spotussi_${version}_${os}_${arch}"
  mkdir -p "$stage/$name"
  echo "building $name"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch "${go[@]}" build -trimpath \
    -ldflags "-s -w -X main.version=$tag" -o "$stage/$name/spotussi" ./cmd/spotussi
  cp README.md LICENSE "$stage/$name/"
  mkdir -p "$stage/$name/docs"
  cp docs/*.md "$stage/$name/docs/"
  # Reproducible archive metadata: fixed owner and the commit's time.
  mtime=$(git log -1 --format=%cI 2>/dev/null || date -u +%Y-%m-%dT%H:%M:%SZ)
  if tar --version 2>/dev/null | grep -q 'GNU tar'; then
    tar -C "$stage" --owner=0 --group=0 --numeric-owner --mtime="$mtime" --sort=name \
      -cf - "$name" | gzip -n -9 > "$out/$name.tar.gz"
  else # bsdtar (macOS)
    tar -C "$stage" --uid 0 --gid 0 -cf - "$name" | gzip -n -9 > "$out/$name.tar.gz"
  fi
done

(
  cd "$out"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- *.tar.gz > SHA256SUMS
  else
    shasum -a 256 -- *.tar.gz > SHA256SUMS
  fi
)
echo "archives and checksums in $out:"
cat "$out/SHA256SUMS"
