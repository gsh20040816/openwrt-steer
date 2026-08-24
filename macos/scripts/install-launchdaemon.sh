#!/bin/sh
set -eu

if [ "$(uname -s)" != "Darwin" ]; then
	printf '%s\n' 'Steer macOS LaunchDaemon must be installed on macOS.' >&2
	exit 1
fi
if [ "$(id -u)" -ne 0 ]; then
	printf '%s\n' 'Run this installer with sudo.' >&2
	exit 1
fi

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
repository_root="$(CDPATH= cd -- "$script_dir/../.." && pwd)"
helper_directory="/usr/local/libexec/steer"
plist_path="/Library/LaunchDaemons/com.gsh20040816.steer.plist"
sing_box_path="$(command -v sing-box || true)"

[ -n "$sing_box_path" ] || {
	printf '%s\n' 'sing-box is required; install it with Homebrew first.' >&2
	exit 1
}

install -d -m 0755 "$helper_directory"
install -d -m 0750 "/Library/Application Support/Steer/config" \
	"/Library/Application Support/Steer/run" \
	"/Library/Application Support/Steer/state" \
	"/Library/Application Support/Steer/geodata-seed" \
	"/Library/Logs/Steer"

(cd "$repository_root/go" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$helper_directory/steer-macos" ./cmd/steer-macos)
install -m 0644 "$repository_root/macos/launchd/com.gsh20040816.steer.plist" "$plist_path"
plutil -replace ProgramArguments.11 -string "$sing_box_path" "$plist_path"
chown root:wheel "$helper_directory/steer-macos" "$plist_path"
chmod 0755 "$helper_directory/steer-macos"
chmod 0644 "$plist_path"

launchctl bootout system/com.gsh20040816.steer 2>/dev/null || true
launchctl bootstrap system "$plist_path"
printf '%s\n' "Installed Steer LaunchDaemon with sing-box at $sing_box_path"
