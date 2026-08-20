#!/usr/bin/env python3
"""Reject package ownership and retired-runtime regressions after M1 cutover."""

from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[1]


def fail(message: str) -> None:
    print(f"check-package-boundaries: {message}", file=sys.stderr)
    raise SystemExit(1)


makefile = (ROOT / "steer-openwrt/Makefile").read_text()
geodata_makefile = (ROOT / "steer-geodata/Makefile").read_text()
rpc = (ROOT / "luci-app-steer/root/usr/share/rpcd/ucode/luci.steer").read_text()

for dependency in (
    "steer-geodata",
    "geoview",
    "ip",
    "kmod-nft-queue",
    "kmod-nft-tproxy",
    "kmod-tun",
):
    if f"+{dependency}" not in makefile:
        fail(f"steer-openwrt must declare external dependency: {dependency}")

for retired in ("smartdns", "curl", "ucode-mod-uci", "bind-dig"):
    if f"+{retired}" in makefile:
        fail(f"steer-openwrt retained retired dependency: {retired}")

for concrete_ip_provider in ("ip-tiny", "ip-full"):
    if f"+{concrete_ip_provider}" in makefile:
        fail(f"steer-openwrt must use the virtual ip provider, not {concrete_ip_provider}")

extra_depends = [
    line.strip()
    for line in makefile.splitlines()
    if line.strip().startswith("EXTRA_DEPENDS:=")
]
if extra_depends != ["EXTRA_DEPENDS:=sing-box (>=1.13.18)"]:
    fail("only sing-box may bypass Kconfig through EXTRA_DEPENDS")

for metadata in ("PROVIDES:=steer", "CONFLICTS:=steer", "REPLACES:=steer"):
    if metadata not in makefile:
        fail(f"old steer package cutover metadata is missing: {metadata}")

for forbidden_path in ("usr/bin/sing-box", "usr/bin/geoview"):
    if forbidden_path in makefile:
        fail(f"steer-openwrt must not install third-party binary: {forbidden_path}")

if "$(1)/usr/sbin/steer" not in makefile:
    fail("steer-openwrt must install the public CLI as /usr/sbin/steer")
if "$(1)/usr/sbin/steer-openwrt" in makefile or "/usr/sbin/steer-openwrt" in rpc:
    fail("retired steer-openwrt CLI name is still user-visible")

for required in (
    "$(DL_DIR)/$(GEOSITE_FILE)",
    "$(DL_DIR)/$(GEOIP_FILE)",
    "/usr/share/steer/geodata-seed",
):
    if required not in geodata_makefile:
        fail(f"steer-geodata is missing package-owned input: {required}")

if (ROOT / "steer").exists() and any(path.is_file() for path in (ROOT / "steer").rglob("*")):
    fail("retired steer package/runtime still exists")

retired_tokens = (
    "smartdns",
    "last-known-good",
    "/usr/sbin/steerctl",
    "/usr/libexec/steer/runtime",
)
for path in (ROOT / "steer-openwrt").rglob("*"):
    if path.is_file() and not path.name.endswith("_test.go"):
        content = path.read_text(errors="ignore").lower()
        for token in retired_tokens:
            if token.lower() in content:
                fail(f"{path.relative_to(ROOT)} retained retired runtime token: {token}")

for forbidden in ("/bin/sh", "sh -c", "eval("):
    if forbidden in rpc:
        fail(f"LuCI RPC must call fixed controller commands, found: {forbidden}")

print("package boundary checks passed")
