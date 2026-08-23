#!/usr/bin/env python3
"""Reject package ownership and retired-runtime regressions after M1 cutover."""

from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[1]


def fail(message: str) -> None:
    print(f"check-package-boundaries: {message}", file=sys.stderr)
    raise SystemExit(1)


makefile = (ROOT / "steer/Makefile").read_text()
luci_makefile = (ROOT / "luci-app-steer/Makefile").read_text()
geodata_makefile = (ROOT / "steer-geodata/Makefile").read_text()
geoview_makefile = (ROOT / "geoview/Makefile").read_text()
rpc = (ROOT / "luci-app-steer/root/usr/share/rpcd/ucode/luci.steer").read_text()
acl = (ROOT / "luci-app-steer/root/usr/share/rpcd/acl.d/luci-app-steer.json").read_text()

for dependency in (
    "steer-geodata",
    "geoview",
    "ip",
    "kmod-nft-queue",
    "kmod-nft-tproxy",
    "kmod-tun",
):
    if f"+{dependency}" not in makefile:
        fail(f"steer must declare external dependency: {dependency}")

for retired in ("smartdns", "curl", "ucode-mod-uci", "bind-dig"):
    if f"+{retired}" in makefile:
        fail(f"steer retained retired dependency: {retired}")

for concrete_ip_provider in ("ip-tiny", "ip-full"):
    if f"+{concrete_ip_provider}" in makefile:
        fail(f"steer must use the virtual ip provider, not {concrete_ip_provider}")

extra_depends = [
    line.strip()
    for line in makefile.splitlines()
    if line.strip().startswith("EXTRA_DEPENDS:=")
]
if extra_depends != ["EXTRA_DEPENDS:=sing-box (>=1.13.18), sing-box (<1.14.0)"]:
    fail("only sing-box may bypass Kconfig through EXTRA_DEPENDS")

for metadata in (
    "PROVIDES:=steer-openwrt",
    "CONFLICTS:=steer-openwrt",
    "REPLACES:=steer-openwrt",
):
    if metadata not in makefile:
        fail(f"steer-openwrt package replacement metadata is missing: {metadata}")

for forbidden_path in ("usr/bin/sing-box", "usr/bin/geoview"):
    if forbidden_path in makefile:
        fail(f"steer must not install third-party binary: {forbidden_path}")

if "$(1)/usr/sbin/steer" not in makefile:
    fail("steer must install the public CLI as /usr/sbin/steer")
if "$(PKG_INSTALL_DIR)/usr/bin/steer-openwrt $(1)/usr/sbin/steer" not in makefile:
    fail("OpenWrt source target must be renamed to the public steer executable during packaging")
if "$(1)/usr/sbin/steer-openwrt" in makefile or "/usr/sbin/steer-openwrt" in rpc:
    fail("retired steer-openwrt CLI name is still user-visible")

if '[ "$$schema" = 7 ]' not in makefile or "schema 7 is required" not in makefile:
    fail("steer package must reject configurations outside schema 7")
for retired_migration in (
    "for option in probe_direct probe_proxy speedtest_proxy",
    "uci set steer.main.schema_version='7'",
    "_connect_speedtest _download_speedtest",
    "/var/lib/steer/logs/speedtests",
):
    if retired_migration in makefile:
        fail(f"package retained expired alpha migration: {retired_migration}")
if "PKG_NAME:=steer" not in makefile or "define Package/steer" not in makefile:
    fail("the OpenWrt controller package must be named steer")
if "PKG_VERSION:=0.6.4\n" not in makefile or "PKG_RELEASE:=1\n" not in makefile:
    fail("steer package version must be the 0.6.4-r1 stable release")
if "PKG_VERSION:=0.6.4\n" not in luci_makefile or "PKG_RELEASE:=1\n" not in luci_makefile:
    fail("LuCI packages must use the 0.6.4-r1 stable release")
if "PKG_RELEASE:=3" not in geoview_makefile:
    fail("geoview package release must increase when removing its downstream patch")
patches = ROOT / "geoview/patches"
if patches.exists() and any(path.is_file() for path in patches.rglob("*")):
    fail("geoview must be built from upstream without downstream patches")
if "github.com/gsh20040816/steer/go" not in makefile or "$(CURDIR)/../go/." not in makefile:
    fail("steer must build the repository-level Go module")
for stale_repair in ("repaired_subscription_network", "uci -q delete steer.$$section.network"):
    if stale_repair in makefile:
        fail(f"package retained the expired subscription network repair: {stale_repair}")
if "*/15 * * * * /usr/sbin/steer subscription update" not in makefile:
    fail("subscription cron dispatcher is not package-managed")
if "[ -x /etc/init.d/cron ]" not in makefile:
    fail("subscription dispatcher must fail fast when BusyBox crond is unavailable")
if "PKG_UPGRADE=0 /usr/sbin/steer apply" not in makefile:
    fail("post-upgrade must switch the schema 7 intent through verified Apply")
if "PKG_UPGRADE=0 /etc/init.d/steer start" in makefile:
    fail("post-upgrade must not leave an already-running sing-box instance unchanged")
if "subscription_update" not in rpc or "subscription_update" not in acl:
    fail("LuCI subscription update must be implemented and authorized explicitly")
for removed_contract in ("steer rollback", "steer plan", "method: 'rollback'", "method: 'plan'"):
    if removed_contract in rpc:
        fail(f"LuCI RPC retained removed public contract: {removed_contract}")
if "rm -f /var/lib/steer/rollback.uci" not in makefile:
    fail("the removed rollback state file must be cleaned during upgrade")

for required in (
    "$(DL_DIR)/$(GEOSITE_FILE)",
    "$(DL_DIR)/$(GEOIP_FILE)",
    "/usr/share/steer/geodata-seed",
):
    if required not in geodata_makefile:
        fail(f"steer-geodata is missing package-owned input: {required}")
if "PKG_RELEASE:=2" not in geodata_makefile:
    fail("steer-geodata release must increase when removing the release marker")
if "files/release" in geodata_makefile or "/geodata-seed/release" in geodata_makefile:
    fail("steer-geodata still installs the retired release marker")
if (ROOT / "steer-geodata/files/release").exists():
    fail("retired steer-geodata release marker still exists")

if (ROOT / "steer-openwrt").exists() and any(
    path.is_file() for path in (ROOT / "steer-openwrt").rglob("*")
):
    fail("retired steer-openwrt package directory still exists")

retired_tokens = (
    "smartdns",
    "last-known-good",
    "/usr/sbin/steerctl",
    "/usr/libexec/steer/runtime",
)
for path in (ROOT / "steer").rglob("*"):
    if path.is_file() and not path.name.endswith("_test.go"):
        content = path.read_text(errors="ignore").lower()
        for token in retired_tokens:
            if token.lower() in content:
                fail(f"{path.relative_to(ROOT)} retained retired runtime token: {token}")

for forbidden in ("/bin/sh", "sh -c", "eval("):
    if forbidden in rpc:
        fail(f"LuCI RPC must call fixed controller commands, found: {forbidden}")

print("package boundary checks passed")
