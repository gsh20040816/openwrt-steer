#!/usr/bin/env python3
"""Reject package ownership regressions during the M1 refactor."""

from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[1]


def fail(message: str) -> None:
    print(f"check-package-boundaries: {message}", file=sys.stderr)
    raise SystemExit(1)


steer_makefile = (ROOT / "steer/Makefile").read_text()
geodata_makefile = (ROOT / "steer-geodata/Makefile").read_text()
geodata_runtime = (ROOT / "steer/files/usr/libexec/steer/geodata").read_text()
rpc = (ROOT / "luci-app-steer/root/usr/share/rpcd/ucode/luci.steer").read_text()
acl = (ROOT / "luci-app-steer/root/usr/share/rpcd/acl.d/luci-app-steer.json").read_text()

for dependency in ("steer-geodata", "geoview", "ip-tiny", "smartdns"):
    if f"+{dependency}" not in steer_makefile:
        fail(f"steer must declare the current external dependency: {dependency}")

if "+ip-full" in steer_makefile:
    fail("steer only uses policy rule/route operations and must not require ip-full")

extra_depends = [
    line.strip()
    for line in steer_makefile.splitlines()
    if line.strip().startswith("EXTRA_DEPENDS:=")
]
if extra_depends != ["EXTRA_DEPENDS:=curl (>=0), sing-box (>=0)"]:
    fail("only curl and sing-box may bypass Kconfig through EXTRA_DEPENDS")

for dependency in ("curl (>=0)", "sing-box (>=0)"):
    if dependency not in steer_makefile:
        fail(f"steer must declare the cyclic runtime-only dependency: {dependency}")

for forbidden_path in ("usr/bin/sing-box", "usr/sbin/smartdns", "usr/bin/geoview"):
    if forbidden_path in steer_makefile:
        fail(f"steer must not install a third-party binary: {forbidden_path}")

for required in (
    "$(DL_DIR)/$(GEOSITE_FILE)",
    "$(DL_DIR)/$(GEOIP_FILE)",
    "/usr/share/steer/geodata-seed",
):
    if required not in geodata_makefile:
        fail(f"steer-geodata is missing package-owned input: {required}")

for forbidden in (
    "https://",
    "uclient-fetch",
    "API_URL",
    "geodata update",
    "geodata schedule",
):
    if forbidden in geodata_runtime:
        fail(f"runtime geodata helper bypasses package management: {forbidden}")

if (ROOT / "steer/files/etc/init.d/steer-geodata").exists():
    fail("runtime geodata update scheduler must not be installed")

if "geodata_update" in rpc or "geodata_update" in acl:
    fail("LuCI must not expose a router-managed geodata updater")

print("package boundary checks passed")
