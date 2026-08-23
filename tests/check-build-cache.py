#!/usr/bin/env python3
"""Keep release builds on the official OpenWrt SDK workflow.

The SDK already contains the target toolchain.  The release workflow must not
reintroduce a second builder, target-state cache, or parallel download helper.
"""

from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = (ROOT / ".github/workflows/release.yml").read_text()


def fail(message: str) -> None:
    print(f"check-build-cache: {message}", file=sys.stderr)
    raise SystemExit(1)


ENTRYPOINT = (ROOT / ".github/actions/openwrt-sdk/entrypoint.sh").read_text()

required_fragments = (
    "name: Build release artifacts",
    "ghcr.io/openwrt/sdk@sha256:c8a248ce2411962a89f227db444bf5cea022829b049e6326c7d1032d9762982a",
    "--volume \"$source_dir:/feed:ro\"",
    "--volume \"$RUNNER_TEMP/steer-sdk-artifacts:/artifacts\"",
    "--volume \"$RUNNER_TEMP/steer-sdk-dl:/builder/dl\"",
    "--volume \"$RUNNER_TEMP/steer-sdk-ccache:/builder/.ccache\"",
    "--volume \"$RUNNER_TEMP/steer-sdk-go-cache:/go-build-cache\"",
    "chmod -R a+rwx",
    "actions/cache/restore@55cc8345863c7cc4c66a329aec7e433d2d1c52a9",
    "actions/cache/save@55cc8345863c7cc4c66a329aec7e433d2d1c52a9",
    "--env PACKAGES=\"geoview steer-geodata steer luci-app-steer\"",
    "cp -R \"$RUNNER_TEMP/steer-sdk-artifacts/bin\" \"$GITHUB_WORKSPACE/bin\"",
    "./scripts/collect-openwrt-artifacts.sh",
    "CGO_ENABLED=0 GOOS=linux GOARCH=\"$goarch\" go build",
    "./scripts/collect-linux-artifacts.sh",
    "name: linux-generic",
    "name: release-bundle",
)
for fragment in required_fragments:
    if fragment not in WORKFLOW:
        fail(f"release workflow is missing: {fragment}")

for forbidden in (
    "docker/build-push-action",
    "docker/setup-buildx-action",
    "aria2c",
    "parallel download",
    "steer-builder",
    ".github/actions/openwrt-builder",
    "GO_BOOTSTRAP_IMAGE",
    "CONFIG_GOLANG_EXTERNAL_BOOTSTRAP_ROOT",
    "external-go",
    "build_dir",
    "staging_dir",
    "hostpkg",
    "makepkg",
    "dpkg-buildpackage",
    "rpmbuild",
):
    if forbidden in WORKFLOW:
        fail(f"release workflow must not contain custom cache or downloader: {forbidden}")

if "paths-ignore:" in WORKFLOW:
    fail("workflow changes must trigger a release build")

if WORKFLOW.count("actions/cache/restore@") != 2:
    fail("release workflow must restore exactly ccache and GOCACHE")
if WORKFLOW.count("actions/cache/save@") != 2:
    fail("release workflow must save exactly ccache and GOCACHE")

cache_paths = {
    line.strip().removeprefix("path: ")
    for line in WORKFLOW.splitlines()
    if line.strip().startswith("path: ${{ runner.temp }}/steer-sdk-")
}
expected_cache_paths = {
    "${{ runner.temp }}/steer-sdk-ccache",
    "${{ runner.temp }}/steer-sdk-go-cache",
}
if cache_paths != expected_cache_paths:
    fail(f"unexpected persistent cache paths: {sorted(cache_paths)}")

required_entrypoint_fragments = (
    "./scripts/feeds update -a",
    "src-git --root=package base https://git.openwrt.org/openwrt/openwrt.git^f0a60eee2fe051741c643ea6118718aae1ef17fb",
    "src-git packages https://github.com/openwrt/packages.git^5caa62e0bc9f7fb9b0c12a23267bceb7724214dd",
    "src-git luci https://github.com/openwrt/luci.git^128a7812f4be233c5dd7f7466f534fd888785caf",
    "CONFIG_ALL_KMODS=n",
    "CONFIG_ALL_NONSHARED=n",
    "CONFIG_CCACHE=y",
    "CCACHE_CONFIGPATH2=staging_dir/host/etc/ccache.conf",
    "compiler_check=string:openwrt-sdk-x86_64-25.12.5:c8a248ce2411962a89f227db444bf5cea022829b049e6326c7d1032d9762982a",
    "make --no-print-directory val.CCACHE_DIR",
    "make defconfig",
    "grep -qx '# CONFIG_ALL_KMODS is not set' .config",
    "grep -qx '# CONFIG_ALL_NONSHARED is not set' .config",
    "kmod-r8169|kmod-video",
    "kmod_overrides=",
    "CONFIG_AUTOREMOVE=y $kmod_overrides",
    "staging_dir/host/bin/ccache --zero-stats",
    'make "package/$package/download" V=s',
    'make package/luci-app-steer/compile V="${V:-s}" -j "$(nproc)"',
    "make package/index",
    "'steer-[0-9]*.apk'",
    "'luci-i18n-steer-zh-cn-*.apk'",
    "staging_dir/host/bin/ccache -vv --show-stats",
)
for fragment in required_entrypoint_fragments:
    if fragment not in ENTRYPOINT:
        fail(f"official SDK entrypoint is missing: {fragment}")

if ENTRYPOINT.index("make \"package/$package/download\" V=s") > ENTRYPOINT.index(
    'make package/luci-app-steer/compile'
):
    fail("package downloads must complete before compilation")

if ENTRYPOINT.count("make package/luci-app-steer/compile") != 1:
    fail("the SDK build must have one top-level compile target")

if ENTRYPOINT.count('-j "$(nproc)"') != 1:
    fail("only the compile target may use parallel make jobs")

print("OpenWrt official SDK workflow checks passed")
