#!/usr/bin/env python3
"""Static contracts for native macOS DMG packaging and tag publication."""

from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = (ROOT / ".github/workflows/release.yml").read_text(encoding="utf-8")
BUNDLER = (ROOT / "macos/scripts/build-app-bundle.sh").read_text(encoding="utf-8")
INSTALLER = (ROOT / "macos/scripts/install-embedded-payload.sh").read_text(encoding="utf-8")
UNINSTALLER = (ROOT / "macos/scripts/uninstall-embedded-payload.sh").read_text(encoding="utf-8")


def fail(message: str) -> None:
    print(f"check-macos-packaging: {message}", file=sys.stderr)
    raise SystemExit(1)


for path in (
    ROOT / "macos/scripts/build-app-bundle.sh",
    ROOT / "macos/scripts/install-embedded-payload.sh",
    ROOT / "macos/scripts/uninstall-embedded-payload.sh",
    ROOT / "macos/launchd/com.steer.steer.control.plist",
    ROOT / "macos/launchd/com.steer.steer.subscription.plist",
    ROOT / "macos/SteerAppIcon.png",
):
    if not path.exists():
        fail(f"missing required file: {path.relative_to(ROOT)}")

for fragment in (
    "runner: macos-26",
    "arch: arm64",
    "runner: macos-26-intel",
    "arch: x86_64",
    "DEVELOPER_DIR: /Applications/Xcode_26.6.app/Contents/Developer",
    "xcodebuild -version | grep -Fqx 'Xcode 26.6'",
    "xcrun vtool -show-build",
    "grep -E '^(macOS SDK|Xcode|Swift|Go): '",
    "CGO_ENABLED=0 GOOS=darwin GOARCH=${{ matrix.goarch }} go build",
    "swift build -c release --disable-sandbox",
    "sing-box-$SING_BOX_VERSION-darwin-${{ matrix.upstream_arch }}.tar.gz",
    "0c57457917ad529da4af939a3da5e0ad1cfa639c140dd3de7b6248aef2170bcd",
    "b0c45037c369616e744b8276bfc3be74f246d889531b73ca592a67c0e06bb432",
    "macos/scripts/build-app-bundle.sh",
    "steer-macos-arm64.dmg",
    "steer-macos-x86_64.dmg",
    "codesign --verify --deep --strict",
    "plutil -extract CFBundleExecutable raw",
    "name: macos-${{ matrix.arch }}",
    "actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6",
):
    if fragment not in WORKFLOW:
        fail(f"tag workflow is missing macOS contract: {fragment}")

for forbidden in (
    "GOOS=darwin GOARCH=amd64 swift",
    "notarytool",
    "altool",
    "Developer ID Application",
    "steer-macos-arm64.zip",
    "steer-macos-x86_64.zip",
):
    if forbidden in WORKFLOW or forbidden in BUNDLER:
        fail(f"unsupported macOS release behavior present: {forbidden}")

for fragment in (
    'app="$work_directory/Steer.app"',
    '"$app/Contents/MacOS/SteerApp"',
    '"$app/Contents/Resources/Installer"',
    '"$app/Contents/Resources/geodata-seed"',
    '"$app/Contents/Resources/LICENSES"',
    'app_icon="$repository_root/macos/SteerAppIcon.png"',
    'iconutil -c icns "$iconset" -o "$resources_directory/SteerAppIcon.icns"',
    "<key>CFBundleIconFile</key>",
    "<string>SteerAppIcon</string>",
    "<string>SteerApp</string>",
    "<string>com.steer.steer</string>",
    "<string>13.0</string>",
    "STEER_BUILD_NUMBER must contain digits only",
    "PAYLOAD-SHA256SUMS",
    "uninstall-embedded-payload.sh",
    "verify-geodata --directory",
    "validate --config",
    "parse-nodes --input",
    "com.steer.steer.subscription.plist",
    "lipo -archs",
    "xcrun vtool -show-build",
    "SteerApp must be linked against the macOS 26 SDK",
    "codesign --force --sign - --timestamp=none",
    "codesign --verify --deep --strict",
    "hdiutil create",
    "hdiutil verify",
    "ln -s /Applications",
    "Signing: ad-hoc",
    "macOS SDK: $app_sdk",
    "Notarization: none",
    "BUILD-METADATA.txt",
    "SHA256SUMS",
):
    if fragment not in BUNDLER:
        fail(f"bundle script is missing: {fragment}")

helper_sign = BUNDLER.index('codesign --force --sign - --timestamp=none "$installer_directory/steer-macos"')
payload_hash = BUNDLER.index("\t\t> PAYLOAD-SHA256SUMS")
app_sign = BUNDLER.index('codesign --force --sign - --timestamp=none "$app"')
if not helper_sign < payload_hash < app_sign:
    fail("embedded executables must be signed before payload checksums and the App must be signed last")
if 'codesign --force --deep --sign - --timestamp=none "$app"' in BUNDLER:
    fail("final App signing must not recursively mutate already-hashed embedded executables")

for fragment in (
    'helper_payload="$script_dir/steer-macos"',
    'sing_box_payload="$script_dir/sing-box"',
    'control_plist_payload="$script_dir/com.steer.steer.control.plist"',
    'subscription_plist_payload="$script_dir/com.steer.steer.subscription.plist"',
    'geodata_payload="$resources_dir/geodata-seed"',
    "[ ! -L \"$1\" ]",
    "/usr/bin/shasum -a 256 -c PAYLOAD-SHA256SUMS",
    "/usr/bin/codesign --verify --deep --strict \"$app_bundle\"",
    "/usr/bin/file \"$binary\"",
    "verify-geodata --directory",
    "/usr/local/libexec/steer",
    "/Library/LaunchDaemons/com.steer.steer.control.plist",
    "/Library/LaunchDaemons/com.steer.steer.subscription.plist",
    "/var/run/steer",
    "-o root -g wheel -m 0755 \"$socket_directory\"",
    "-o root -g wheel -m 0755",
    "-o root -g wheel -m 0644",
    "if [ -f \"$support_directory/config/config.json\" ]",
    "launchctl bootout system/com.steer.steer.control",
    "launchctl bootstrap system \"$control_plist_path\"",
    "launchctl bootstrap system \"$subscription_plist_path\"",
    '"$script_dir/uninstall-embedded-payload.sh"',
):
    if fragment not in INSTALLER:
        fail(f"embedded installer is missing: {fragment}")

if "command -v" in INSTALLER or "go build" in INSTALLER:
    fail("embedded installer must not depend on PATH discovery or source compilation")

for fragment in (
    '"--remove-user-data"',
    'helper_directory="/usr/local/libexec/steer"',
    'runtime_plist_path="/Library/LaunchDaemons/com.steer.steer.plist"',
    'control_plist_path="/Library/LaunchDaemons/com.steer.steer.control.plist"',
    'subscription_plist_path="/Library/LaunchDaemons/com.steer.steer.subscription.plist"',
    'support_directory="/Library/Application Support/Steer"',
    'logs_directory="/Library/Logs/Steer"',
    'socket_directory="/var/run/steer"',
    'Refusing to uninstall through a symbolic-link directory',
    "/usr/bin/shasum -a 256 -c PAYLOAD-SHA256SUMS",
    '/usr/bin/codesign --verify --deep --strict "$app_bundle"',
    'launchctl bootout "system/$label"',
    '/bin/rm -rf "$helper_directory"',
    '/bin/rm -f "$socket_directory/control.sock"',
    '/bin/rm -rf "$support_directory/run" "$support_directory/geodata-seed"',
    'Preserved configuration, state, and logs.',
):
    if fragment not in UNINSTALLER:
        fail(f"embedded uninstaller is missing: {fragment}")

for forbidden in ("eval ", "sh -c", "command -v", "go build"):
    if forbidden in UNINSTALLER:
        fail(f"embedded uninstaller accepts an unsafe execution path: {forbidden}")

print("macOS native DMG packaging checks passed")
