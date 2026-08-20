#!/usr/bin/env python3
"""Keep release caching aligned with OpenWrt's tools/toolchain + ccache boundary."""

from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = (ROOT / ".github/workflows/release.yml").read_text()
ENTRYPOINT = (ROOT / ".github/actions/openwrt-builder/entrypoint.sh").read_text()
DOCKERFILE = (ROOT / ".github/actions/openwrt-builder/Dockerfile").read_text()


def fail(message: str) -> None:
    print(f"check-build-cache: {message}", file=sys.stderr)
    raise SystemExit(1)


required_workflow_fragments = (
    "actions: write",
    "branches:\n      - master",
    "group: openwrt-package-build",
    "cancel-in-progress: false",
    "Build cached OpenWrt source and toolchain image",
    "GO_BOOTSTRAP_IMAGE=docker.io/library/golang:1.26.4-bookworm@sha256:b305420a68d0f229d91eb3b3ed9e519fcf2cf5461da4bef997bf927e8c0bfd2b",
    "id: openwrt-ccache",
    "actions/cache/restore@55cc8345863c7cc4c66a329aec7e433d2d1c52a9",
    "openwrt-ccache-packages-v2-${{ runner.os }}-25.12.5-x86_64-f0a60eee-16fe9150",
    "${{ runner.temp }}/steer-openwrt-ccache",
    "--env CCACHE_DIR=/work/openwrt/.ccache",
    '--volume "$ccache_dir:/work/openwrt/.ccache"',
    "Delete previously restored OpenWrt ccache",
    "octokit/request-action@b91aabaa861c777dcdb14e2387e30eddf04619ae",
    "DELETE /repos/{repository}/actions/caches?key={key}&ref={ref}",
    "INPUT_REF: ${{ github.ref }}",
    "steps.openwrt-ccache.outputs.cache-primary-key",
    "actions/cache/save@55cc8345863c7cc4c66a329aec7e433d2d1c52a9",
)
for fragment in required_workflow_fragments:
    if fragment not in WORKFLOW:
        fail(f"release workflow is missing: {fragment}")

default_branch_guard = (
    "github.ref == format('refs/heads/{0}', "
    "github.event.repository.default_branch)"
)
if WORKFLOW.count(default_branch_guard) != 3:
    fail("only the default branch may write or rotate shared caches")

cache_to_line = next(
    (line.strip() for line in WORKFLOW.splitlines() if line.strip().startswith("cache-to:")),
    "",
)
if default_branch_guard not in cache_to_line:
    fail("Buildx cache export must be restricted to the default branch")

delete_block = WORKFLOW.split("- name: Delete previously restored OpenWrt ccache", 1)[1]
delete_block = delete_block.split("- name: Save OpenWrt package ccache", 1)[0]
if default_branch_guard not in delete_block:
    fail("shared ccache rotation must be restricted to the default branch")

save_block = WORKFLOW.split("- name: Save OpenWrt package ccache", 1)[1]
save_block = save_block.split("- name: Collect exact release assets", 1)[0]
if default_branch_guard not in save_block:
    fail("shared ccache save must be restricted to the default branch")

for forbidden in (
    "require_build_cache_hit",
    "build_dir-target",
    "staging_dir-target",
    "STEER_BUILD_CACHE_HIT",
    "STEER_RETAINED_BUILD_CACHE_HIT",
    "Restore previous v5 cache as seed",
    "steer-dependency-cache-complete",
    "targetdeps-v",
    "ghcr.io/openwrt/sdk:",
    "CCACHE_DIR=/builder/.ccache",
    '"$ccache_dir:/builder/.ccache"',
):
    if forbidden in WORKFLOW:
        fail(f"release workflow still caches OpenWrt target state: {forbidden}")

required_entrypoint_fragments = (
    'readonly CCACHE_DIR="${CCACHE_DIR:-/work/openwrt/.ccache}"',
    "readonly CCACHE_CONFIGPATH2=staging_dir/host/etc/ccache.conf",
    'make --no-print-directory val.CCACHE_DIR',
    "OpenWrt resolved CCACHE_DIR to",
    "'compiler_type=gcc'",
    "'max_size=8G'",
    "'depend_mode=true'",
    "compiler_check=string:openwrt-25.12-x86-64:16fe9150",
    "./scripts/ext-toolchain.sh",
    "--config x86/64",
    "'CONFIG_PACKAGE_luci-app-steer=m'",
    "'CONFIG_CCACHE=y'",
    "'CONFIG_AUTOREMOVE=y'",
    "'CONFIG_LOCALMIRROR=\"https://sources.cdn.openwrt.org\"'",
    "'CONFIG_GOLANG_EXTERNAL_BOOTSTRAP_ROOT=\"/external-go\"'",
    "'# CONFIG_GOLANG_BUILD_BOOTSTRAP is not set'",
    "Unexpected external Go bootstrap:",
    "staging_dir/host/bin/ccache --zero-stats",
    'make tools/install -j "$(nproc)" BUILD_LOG=1',
    'make toolchain/install -j "$(nproc)" BUILD_LOG=1',
    "Build target kernel for the selected module ABI",
    "target/compile",
    "staging_dir/host/bin/ccache -vv --show-stats",
    "staging_dir/host/bin/ccache --evict-older-than 1d",
    "staging_dir/host/bin/ccache --cleanup",
    "grep -qx '# CONFIG_ALL_KMODS is not set' .config",
    "Minimal package config unexpectedly selected",
    "Minimal package config selected unrelated kernel modules.",
    "Minimal package config selected target-profile firmware.",
    'minimal_config="$(mktemp)"',
    "/^CONFIG_PACKAGE_.*=[my]$/",
    "default_kmod_overrides",
    "/^CONFIG_PACKAGE_kmod-.*=y$/",
    "readonly BUILD_PACKAGE=luci-app-steer",
    '"package/$BUILD_PACKAGE/compile"',
)
for fragment in required_entrypoint_fragments:
    if fragment not in ENTRYPOINT:
        fail(f"OpenWrt builder entrypoint is missing: {fragment}")

for forbidden in (
    "CACHE_COMPLETE_MARKER",
    "retained_dependency_tree_exists",
    "STEER_BUILD_CACHE_HIT",
    "package/$package/clean",
    "CONFIG_AUTOREMOVE=n",
    "find build_dir/hostpkg",
    "find staging_dir/hostpkg/stamp",
):
    if forbidden in ENTRYPOINT:
        fail(f"OpenWrt builder entrypoint still maintains cached target state: {forbidden}")

if 'targets+=("package/$package/compile")' in ENTRYPOINT:
    fail("build the single luci-app-steer dependency closure, not parallel top-level targets")

ccache_config_position = ENTRYPOINT.index("'CONFIG_CCACHE=y'")
final_defconfig_position = ENTRYPOINT.index("grep -qx 'CONFIG_CCACHE=y' .config")
if ccache_config_position > final_defconfig_position:
    fail("CONFIG_CCACHE must be selected before the final defconfig")

required_dockerfile_fragments = (
    "ARG GO_BOOTSTRAP_IMAGE",
    "ARG TOOLCHAIN_IMAGE",
    "ARG OPENWRT_REF",
    "FROM ${GO_BOOTSTRAP_IMAGE} AS go-bootstrap",
    "COPY --from=go-bootstrap /usr/local/go /external-go",
    "git -C /work/openwrt fetch --depth=1 origin",
    "ln -s /prebuilt_tools/staging_dir/host staging_dir/host",
    "ln -s /prebuilt_tools/build_dir/host build_dir/host",
    "./scripts/ext-tools.sh --refresh",
    "USER buildbot",
)
for fragment in required_dockerfile_fragments:
    if fragment not in DOCKERFILE:
        fail(f"OpenWrt builder Dockerfile is missing: {fragment}")

if "ARG SDK_IMAGE" in DOCKERFILE:
    fail("release builder must use the official external toolchain, not the all-package SDK")

print("OpenWrt official cache-boundary checks passed")
