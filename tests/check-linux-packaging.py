#!/usr/bin/env python3
"""Keep generic Linux delivery platform-neutral and install-name consistent."""

from pathlib import Path
import re
import sys


ROOT = Path(__file__).resolve().parents[1]


def fail(message: str) -> None:
    print(f"check-linux-packaging: {message}", file=sys.stderr)
    raise SystemExit(1)


for path in (
    ROOT / "linux/config.example.json",
    ROOT / "linux/web.example.json",
    ROOT / "scripts/collect-linux-artifacts.sh",
    ROOT / "tests/integration/linux-system.Dockerfile",
    ROOT / "tests/integration/run-linux-system.sh",
    ROOT / "go/cmd/steer-openwrt/main.go",
    ROOT / "go/cmd/steer-linux/commands.go",
    ROOT / "go/cmd/steer-linux/command_apply.go",
    ROOT / "go/cmd/steer-linux/command_probe.go",
    ROOT / "go/cmd/steer-linux/command_subscription.go",
    ROOT / "go/cmd/steer-linux/operation_lock.go",
    ROOT / "go/cmd/steer-linux/web_server.go",
    ROOT / "go/cmd/steer-linux/web_api.go",
    ROOT / "go/cmd/steer-linux/web_auth.go",
    ROOT / "go/cmd/steer-linux/web_config.go",
):
    if not path.is_file():
        fail(f"required structure is missing: {path.relative_to(ROOT)}")

for retired in (
    ROOT / "linux/config.json",
    ROOT / "linux/platform.json",
    ROOT / "linux/platform.example.json",
    ROOT / "go/cmd/steer/main.go",
    ROOT / "scripts/collect-release-artifacts.sh",
    ROOT / "packaging/archlinux/geoview",
):
    if retired.exists():
        fail(f"retired path still exists: {retired.relative_to(ROOT)}")

main_lines = (ROOT / "go/cmd/steer-linux/main.go").read_text().splitlines()
if len(main_lines) >= 100:
    fail("Linux main.go must remain a small process entry point")

units = "\n".join(
    path.read_text() for path in sorted((ROOT / "linux/systemd").glob("steer*.service"))
)
if "/usr/bin/steer-linux" in units:
    fail("systemd exposes the source target name steer-linux")
for command in (
    "/usr/bin/steer _run",
    "/usr/bin/steer cleanup",
    "/usr/bin/steer web",
    "/usr/bin/steer subscription update",
):
    if command not in units:
        fail(f"systemd is missing public executable command: {command}")
if "PartOf=nftables.service" not in units:
    fail("steer.service must restart with nftables.service")

collector = (ROOT / "scripts/collect-linux-artifacts.sh").read_text()
for required in (
    "for target in x86_64 aarch64",
    'package_name="steer-linux-$target"',
    "config.example.json",
    "web.example.json",
    "geodata-seed",
    "linux/systemd/*.service",
    "linux/systemd/*.timer",
    "CGO_ENABLED=0",
    "SOURCE_DATE_EPOCH",
):
    if required not in collector:
        fail(f"Linux collector is missing: {required}")
for bundled in ("sing-box", "geoview", "geosite.dat", "geoip.dat"):
    if f'install -m 0755 "$project_root/{bundled}' in collector:
        fail(f"Linux collector bundles external resource: {bundled}")

workflow = (ROOT / ".github/workflows/release.yml").read_text()
publish = (ROOT / ".github/workflows/publish.yml").read_text()
for required in (
    "needs: verify",
    "Generic Linux x86_64 and aarch64",
    'CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build',
    "./scripts/collect-linux-artifacts.sh",
    "name: linux-generic",
    "name: release-bundle",
    "name: Linux system integration",
    "/workspace/tests/integration/run-linux-system.sh",
):
    if required not in workflow:
        fail(f"release workflow is missing: {required}")
for archive in ("steer-linux-x86_64.tar.zst", "steer-linux-aarch64.tar.zst"):
    if archive not in workflow or archive not in publish:
        fail(f"release pipeline does not enforce archive: {archive}")
for distro_tool in ("makepkg", "dpkg-buildpackage", "rpmbuild"):
    if distro_tool in workflow:
        fail(f"upstream CI must not build distribution package with {distro_tool}")

arch_pkgbuild = (ROOT / "packaging/archlinux/steer/PKGBUILD").read_text()
arch_srcinfo = (ROOT / "packaging/archlinux/steer/.SRCINFO").read_text()
version_match = re.search(r"^pkgver=(\S+)$", arch_pkgbuild, re.MULTILINE)
if version_match is None:
    fail("Arch steer PKGBUILD has no scalar pkgver")
arch_version = version_match.group(1)
commit_match = re.search(r"^_commit=([0-9a-f]{40})$", arch_pkgbuild, re.MULTILINE)
if commit_match is None:
    fail("Arch steer PKGBUILD must pin a full source commit")
arch_commit = commit_match.group(1)
for required in (
    "  'git'\n",
    f"pkgver = {arch_version}\n",
    "\tmakedepends = git\n",
    f"\tsource = steer::git+https://github.com/gsh20040816/steer.git#commit={arch_commit}\n",
):
    source = arch_pkgbuild if required == "  'git'\n" else arch_srcinfo
    if required not in source:
        fail(f"Arch steer metadata is missing or stale: {required.strip()}")

for required in (
    "steer-geodata.tar.zst::https://gsh20040816.github.io/steer/geodata/latest/steer-geodata.tar.zst",
    "go run ./cmd/steer-geodata-build verify",
    "install=steer.install",
):
    if required not in arch_pkgbuild:
        fail(f"Arch steer package is missing Geo seed or migration contract: {required}")
if "  'sing-box'\n" not in arch_pkgbuild or "sing-box>=1.14" in arch_pkgbuild or "sing-box<1.15" in arch_pkgbuild:
    fail("Arch steer must depend only on the virtual sing-box provider; native config check decides compatibility")
if "\tdepends = sing-box\n" not in arch_srcinfo:
    fail("Arch steer .SRCINFO is missing the virtual sing-box dependency")

print("generic Linux packaging checks passed")
