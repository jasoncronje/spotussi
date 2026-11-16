#!/bin/sh
# Regenerates ios/SpotussiKit/Sources/SpotussiAPI/Generated from
# api/openapi.yaml with Swift OpenAPI Generator.
#
# The generator is built once into a cache directory and reused. Override
# the binary with SWIFT_OPENAPI_GENERATOR=/path/to/swift-openapi-generator.
set -eu

VERSION=1.14.4
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/ios/SpotussiKit/Sources/SpotussiAPI/Generated"
gen=${SWIFT_OPENAPI_GENERATOR:-}

if [ -z "$gen" ]; then
	cache="${XDG_CACHE_HOME:-$HOME/Library/Caches}/spotussi-tools/swift-openapi-generator-$VERSION"
	gen="$cache/.build/release/swift-openapi-generator"
	if [ ! -x "$gen" ]; then
		[ -d "$cache" ] || git clone --quiet --depth 1 --branch "$VERSION" \
			https://github.com/apple/swift-openapi-generator "$cache"
		# Four jobs: leaves room for whatever else the machine is doing.
		swift build --package-path "$cache" -c release --product swift-openapi-generator -j 4
	fi
fi

# The spec's "this or null" properties, rewritten into optionals
# (ios/scripts/swiftspec).
spec=$(mktemp -d)
trap 'rm -rf "$spec"' EXIT
(cd "$root" && go run ./ios/scripts/swiftspec <api/openapi.yaml >"$spec/openapi.json")

rm -rf "$out"
mkdir -p "$out"
"$gen" generate "$spec/openapi.json" \
	--config "$root/ios/openapi-generator-config.yaml" \
	--output-directory "$out"
