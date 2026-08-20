#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Build orchestration follows openwrt/actions-shared-workflows reusable_build.

set -euo pipefail

readonly FEED_NAME=steer
readonly PACKAGES=(geoview steer-geodata steer luci-app-steer)
readonly BUILD_PACKAGE=luci-app-steer
readonly CCACHE_DIR="${CCACHE_DIR:-/work/openwrt/.ccache}"
readonly CCACHE_CONFIGPATH2=staging_dir/host/etc/ccache.conf
readonly EXTERNAL_TOOLCHAIN_ROOT=/external-toolchain
export CCACHE_DIR CCACHE_CONFIGPATH2

group_open=0
group() {
	if [ "$group_open" = 1 ]; then
		echo '::endgroup::'
	fi
	echo "::group::$1"
	group_open=1
}

endgroup() {
	if [ "$group_open" = 1 ]; then
		echo '::endgroup::'
		group_open=0
	fi
}

trap endgroup EXIT

group 'Configure feeds'
printf '%s\n' \
	'src-git packages https://github.com/openwrt/packages.git^5caa62e0bc9f7fb9b0c12a23267bceb7724214dd' \
	'src-git luci https://github.com/openwrt/luci.git^128a7812f4be233c5dd7f7466f534fd888785caf' \
	"src-link $FEED_NAME /feed/" \
	> feeds.conf
cat feeds.conf

group 'Update feeds'
for attempt in 1 2 3; do
	if ./scripts/feeds update -a; then
		break
	fi
	[ "$attempt" -lt 3 ] || {
		echo 'Feed update failed after three attempts.' >&2
		exit 1
	}
	echo "Feed update attempt $attempt failed; retrying." >&2
	sleep 2
done

group 'Install selected feed packages'
for package in "${PACKAGES[@]}"; do
	./scripts/feeds install -p "$FEED_NAME" -f "$package"
done

group 'Configure official external toolchain'
toolchain_dir="$(find "$EXTERNAL_TOOLCHAIN_ROOT" -mindepth 2 -maxdepth 2 \
	-type d -name 'toolchain-*' -print -quit)"
[ -n "$toolchain_dir" ] || {
	echo 'The pinned OpenWrt external toolchain is missing.' >&2
	exit 1
}
./scripts/ext-toolchain.sh \
	--toolchain "$toolchain_dir" \
	--overwrite-config \
	--config x86/64

# ext-toolchain selects the target's complete default device package set. That
# is appropriate for firmware images, but a single-package build would then
# package unrelated x86 drivers and download the full linux-firmware archive.
# Turn every initial package selection into an explicit unset; the requested
# top-level package below lets Kconfig select only its real dependency closure.
minimal_config="$(mktemp)"
awk '
	/^CONFIG_PACKAGE_.*=[my]$/ {
		symbol = $0
		sub(/=.*/, "", symbol)
		disabled[symbol] = 1
		next
	}
	{ print }
	END {
		for (symbol in disabled)
			print "# " symbol " is not set"
	}
' .config > "$minimal_config"
mv "$minimal_config" .config

# Match OpenWrt's shared CI configuration: reuse compiler outputs through
# ccache, while treating target build and staging directories as disposable.
mkdir -p "$CCACHE_DIR"
touch "$CCACHE_CONFIGPATH2"
printf '%s\n' \
	'compiler_type=gcc' \
	'max_size=8G' \
	'depend_mode=true' \
	'sloppiness=file_macro,locale,time_macros,include_file_ctime,include_file_mtime' \
	'compiler_check=string:openwrt-25.12-x86-64:16fe9150edb39da54b50f5d7e99e9baf0f2c9afb16183c54bfb07bc4e08b3b38' \
	>> "$CCACHE_CONFIGPATH2"

# LuCI translation APKs are generated subpackages of luci-app-steer. Selecting
# the language before defconfig keeps the package standard and independently
# installable without inventing a CI-only package.
printf '%s\n' \
	'CONFIG_PACKAGE_luci-app-steer=m' \
	'CONFIG_CCACHE=y' \
	'CONFIG_AUTOREMOVE=y' \
	'CONFIG_LOCALMIRROR="https://sources.cdn.openwrt.org"' \
	'CONFIG_LUCI_LANG_zh_Hans=y' \
	>> .config
make defconfig
resolved_ccache_dir="$(make --no-print-directory val.CCACHE_DIR)"
[ "$resolved_ccache_dir" = "$CCACHE_DIR" ] || {
	echo "OpenWrt resolved CCACHE_DIR to $resolved_ccache_dir instead of $CCACHE_DIR." >&2
	exit 1
}
grep -qx '# CONFIG_ALL_NONSHARED is not set' .config
grep -qx '# CONFIG_ALL_KMODS is not set' .config
grep -qx '# CONFIG_ALL is not set' .config
grep -Eq '^CONFIG_PACKAGE_luci-app-steer=[my]$' .config
grep -qx 'CONFIG_CCACHE=y' .config
grep -qx 'CONFIG_AUTOREMOVE=y' .config
grep -qx 'CONFIG_LOCALMIRROR="https://sources.cdn.openwrt.org"' .config
grep -qx 'CONFIG_LUCI_LANG_zh_Hans=y' .config
selected_kmods="$(grep -c '^CONFIG_PACKAGE_kmod-.*=[my]' .config || true)"
[ "$selected_kmods" -le 100 ] || {
	echo "Minimal package config unexpectedly selected $selected_kmods kernel modules." >&2
	exit 1
}
if grep -qE '^CONFIG_PACKAGE_kmod-(aoe|video-gspca-)' .config; then
	echo 'Minimal package config selected unrelated kernel modules.' >&2
	exit 1
fi
if grep -qE '^CONFIG_PACKAGE_.*firmware.*=[my]' .config; then
	echo 'Minimal package config selected target-profile firmware.' >&2
	exit 1
fi
echo "Selected kernel module packages: $selected_kmods"
./scripts/diffconfig.sh
staging_dir/host/bin/ccache --zero-stats
mapfile -t default_kmod_overrides < <(
	awk -F= '/^CONFIG_PACKAGE_kmod-.*=y$/ { print $1 "=" }' .config
)

group 'Install official prebuilt tools and external toolchain wrappers'
# These targets do not rebuild the toolchain. The prebuilt host tools are
# already linked from /prebuilt_tools, while toolchain/install creates the
# wrappers and imports GCC/libc version metadata from /external-toolchain.
make tools/install -j "$(nproc)" BUILD_LOG=1
make toolchain/install -j "$(nproc)" BUILD_LOG=1

group 'Build target kernel for the selected module ABI'
# The external toolchain supplies the compiler, not the kernel build tree.
# Steer's kmod packages require modules.builtin from this exact source/config.
make \
	BUILD_LOG=1 \
	-j "$(nproc)" \
	"${default_kmod_overrides[@]}" \
	target/compile

group 'Check package metadata and downloads'
download_log="$(mktemp)"
for package in "${PACKAGES[@]}"; do
	make "package/$package/download" V=s 2>&1 | tee -a "$download_log"
	make "package/$package/check" V=s
done

if grep -qE 'HASH does not match |HASH uses deprecated hash,|HASH is missing,' "$download_log"; then
	echo 'Package hash validation failed.' >&2
	exit 1
fi

group 'Build selected package dependency closure'
# luci-app-steer depends on steer, which in turn depends on geoview and
# steer-geodata. A single top-level target lets OpenWrt schedule that closure
# once. OpenWrt's disposable target build state is intentionally not cached;
# compiler outputs are reused by ccache instead.
make \
	BUILD_LOG=1 \
	V=s \
	-j "$(nproc)" \
	"${default_kmod_overrides[@]}" \
	"package/$BUILD_PACKAGE/compile"

find bin/packages -type f -name 'luci-i18n-steer-zh-cn-*.apk' -print -quit | grep -q . || {
	echo 'Simplified Chinese LuCI APK was not produced.' >&2
	exit 1
}

group 'Show and trim ccache'
staging_dir/host/bin/ccache -vv --show-stats
staging_dir/host/bin/ccache --evict-older-than 1d
staging_dir/host/bin/ccache --cleanup

group 'Export build outputs'
[ ! -e /artifacts/bin ] || {
	echo '/artifacts/bin already exists.' >&2
	exit 1
}
mv bin /artifacts/
if [ -d logs ]; then
	[ ! -e /artifacts/logs ] || {
		echo '/artifacts/logs already exists.' >&2
		exit 1
	}
	mv logs /artifacts/
fi
chmod -R a+rX /artifacts/bin
[ ! -d /artifacts/logs ] || chmod -R a+rX /artifacts/logs

endgroup
