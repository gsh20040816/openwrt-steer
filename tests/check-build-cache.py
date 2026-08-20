#!/usr/bin/env python3
"""Reject unsafe or unverifiable OpenWrt dependency cache changes."""

from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = (ROOT / ".github/workflows/release.yml").read_text()
ENTRYPOINT = (ROOT / ".github/actions/openwrt-sdk/entrypoint.sh").read_text()


def fail(message: str) -> None:
    print(f"check-build-cache: {message}", file=sys.stderr)
    raise SystemExit(1)


required_workflow_fragments = (
    "require_build_cache_hit:",
    "id: openwrt-build-cache",
    "build_dir-target",
    "staging_dir-packages",
    "staging_dir-target",
    "openwrt-build-v2-${{ runner.os }}-25.12.5-x86_64-c8a248ce-",
    "f0a60eee-5caa62e0-128a7812-targetdeps-v1",
    "steps.openwrt-build-cache.outputs.cache-hit != 'true'",
    "STEER_BUILD_CACHE_HIT=${{ steps.openwrt-build-cache.outputs.cache-hit }}",
    "/builder/build_dir/target-x86_64_musl",
    "/builder/staging_dir/packages",
    "/builder/staging_dir/target-x86_64_musl",
)
for fragment in required_workflow_fragments:
    if fragment not in WORKFLOW:
        fail(f"release workflow is missing: {fragment}")

if "restore-keys:" in WORKFLOW:
    fail("target dependency state must use an exact cache key")

required_entrypoint_fragments = (
    "readonly CACHE_COMPLETE_MARKER=",
    'if [ "${STEER_BUILD_CACHE_HIT:-false}" = true ]; then',
    'make "package/$package/clean"',
    'find "$TARGET_BUILD_DIR" -type f -name \'.*\' -exec touch {} +',
    'find "$TARGET_STAGING_DIR/stamp" -type f -exec touch {} +',
    'touch "$CACHE_COMPLETE_MARKER"',
)
for fragment in required_entrypoint_fragments:
    if fragment not in ENTRYPOINT:
        fail(f"SDK entrypoint is missing: {fragment}")

if "STEER_HOST_CACHE_HIT" in ENTRYPOINT:
    fail("entrypoint still uses the obsolete host-only cache contract")

clean_position = ENTRYPOINT.index('make "package/$package/clean"')
target_touch_position = ENTRYPOINT.index(
    'find "$TARGET_BUILD_DIR" -type f -name \'.*\' -exec touch {} +'
)
if clean_position > target_touch_position:
    fail("Steer's own packages must be cleaned before dependency stamps are refreshed")

print("OpenWrt build cache checks passed")
