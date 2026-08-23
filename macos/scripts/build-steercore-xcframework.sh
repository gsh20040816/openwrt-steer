#!/bin/sh
set -eu

if [ "$(uname -s)" != "Darwin" ]; then
    echo "SteerCore XCFramework must be built on macOS" >&2
    exit 1
fi
command -v go >/dev/null 2>&1 || { echo "go is required" >&2; exit 1; }
command -v xcodebuild >/dev/null 2>&1 || { echo "xcodebuild is required" >&2; exit 1; }

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
repository_root="$(CDPATH= cd -- "$script_dir/../.." && pwd)"
bridge_dir="$repository_root/macos/bridge"
output_dir="${1:-$repository_root/macos/build}"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT INT TERM

mkdir -p "$output_dir"

for arch in arm64 amd64; do
    slice_dir="$temporary_dir/$arch"
    headers_dir="$slice_dir/Headers"
    mkdir -p "$headers_dir"
    (
        cd "$bridge_dir"
        CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" go build \
            -trimpath \
            -buildmode=c-archive \
            -o "$slice_dir/libSteerCore.a" \
            .
    )
    cp "$slice_dir/libSteerCore.h" "$headers_dir/SteerCore.h"
done

xcodebuild -create-xcframework \
    -library "$temporary_dir/arm64/libSteerCore.a" \
    -headers "$temporary_dir/arm64/Headers" \
    -library "$temporary_dir/amd64/libSteerCore.a" \
    -headers "$temporary_dir/amd64/Headers" \
    -output "$output_dir/SteerCore.xcframework"

{
    echo "Source revision: $(git -C "$repository_root" rev-parse HEAD)"
    echo "Go version: $(go version)"
    echo "sing-box: v1.13.19"
    echo "architectures: arm64 amd64"
} > "$output_dir/SteerCore-BUILD-METADATA.txt"

(
    cd "$output_dir"
    find SteerCore.xcframework -type f -print0 | sort -z | xargs -0 shasum -a 256 > SteerCore-SHA256SUMS
)
