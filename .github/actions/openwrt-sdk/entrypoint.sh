#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Derived from openwrt/gh-action-sdk entrypoint.sh at
# f5813d30eeef3534b58ac7e79c5d8842b6035434.

set -euo pipefail

readonly FEED_NAME=steer
readonly PACKAGES=(geoview steer-geodata steer luci-app-steer)
readonly BUILD_PACKAGE=luci-app-steer
readonly TARGET_BUILD_DIR=build_dir/target-x86_64_musl
readonly TARGET_STAGING_DIR=staging_dir/target-x86_64_musl
readonly CACHE_COMPLETE_MARKER="$TARGET_STAGING_DIR/stamp/.steer-dependency-cache-complete"

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
grep -E '^src-git( --root=package)? (base|packages|luci) ' feeds.conf.default | sed \
	-e 's,https://git.openwrt.org/feed/,https://github.com/openwrt/,' \
	-e 's,https://git.openwrt.org/openwrt/,https://github.com/openwrt/,' \
	-e 's,https://git.openwrt.org/project/,https://github.com/openwrt/,' \
	> feeds.conf
echo "src-link $FEED_NAME /feed/" >> feeds.conf
[ "$(grep -c '^src-git' feeds.conf)" -eq 3 ]
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
make defconfig

group 'Install selected feed packages'
for package in "${PACKAGES[@]}"; do
	./scripts/feeds install -p "$FEED_NAME" -f "$package"
done

# LuCI translation APKs are generated subpackages of luci-app-steer. Selecting
# the language before defconfig keeps the package standard and independently
# installable without inventing a CI-only package.
printf '%s\n' 'CONFIG_LUCI_LANG_zh_Hans=y' >> .config
make defconfig
grep -qx 'CONFIG_LUCI_LANG_zh_Hans=y' .config

if [ "${STEER_BUILD_CACHE_HIT:-false}" = true ]; then
	[ -f staging_dir/hostpkg/stamp/.golang_installed ] || {
		echo 'Host toolchain cache is incomplete.' >&2
		exit 1
	}
	[ -f "$CACHE_COMPLETE_MARKER" ] || {
		echo 'Target dependency cache is incomplete.' >&2
		exit 1
	}
	# A target cache also contains the previous run's Steer build directories
	# and staging records. Remove them through OpenWrt's package clean targets;
	# only third-party dependencies may be reused across source revisions.
	for package in "${PACKAGES[@]}"; do
		make "package/$package/clean"
	done
	# Feed checkout and the final defconfig both create fresh inputs on every
	# runner. Refresh dependency stamps only after both are complete; otherwise
	# defconfig immediately makes the restored dependency state stale again.
	find build_dir/hostpkg -type f -name '.*' -exec touch {} +
	find staging_dir/hostpkg/stamp -type f -exec touch {} +
	find "$TARGET_BUILD_DIR" -type f -name '.*' -exec touch {} +
	find "$TARGET_STAGING_DIR/stamp" -type f -exec touch {} +
fi

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
# once. CONFIG_AUTOREMOVE must stay disabled: it deletes completed build
# directories and turns a restored build cache back into a cold build.
make \
	BUILD_LOG=1 \
	V=s \
	-j "$(nproc)" \
	"package/$BUILD_PACKAGE/compile"

find bin/packages -type f -name 'luci-i18n-steer-zh-cn-*.apk' -print -quit | grep -q . || {
	echo 'Simplified Chinese LuCI APK was not produced.' >&2
	exit 1
}

# actions/cache only saves after the whole job succeeds. This marker lets a
# later exact-key restore fail closed if a partial target cache ever appears.
mkdir -p "$(dirname "$CACHE_COMPLETE_MARKER")"
touch "$CACHE_COMPLETE_MARKER"

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
