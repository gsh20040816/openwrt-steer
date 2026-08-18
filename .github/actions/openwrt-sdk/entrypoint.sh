#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Derived from openwrt/gh-action-sdk entrypoint.sh at
# f5813d30eeef3534b58ac7e79c5d8842b6035434.

set -euo pipefail

readonly FEED_NAME=steer
readonly PACKAGES=(geoview steer-geodata steer luci-app-steer)

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

if [ "${STEER_HOST_CACHE_HIT:-false}" = true ]; then
	[ -f staging_dir/hostpkg/stamp/.golang_installed ] || {
		echo 'Host toolchain cache is incomplete.' >&2
		exit 1
	}
	# The cache key pins the SDK and packages feed. Feed checkout gives its
	# Makefiles fresh mtimes on every runner, so refresh only the matching
	# build/install stamps or make would rebuild the cached Go toolchain.
	find build_dir/hostpkg -type f -name '.*' -exec touch {} +
	find staging_dir/hostpkg/stamp -type f -exec touch {} +
fi

# LuCI translation APKs are generated subpackages of luci-app-steer. Selecting
# the language before defconfig keeps the package standard and independently
# installable without inventing a CI-only package.
printf '%s\n' 'CONFIG_LUCI_LANG_zh_Hans=y' >> .config
make defconfig
grep -qx 'CONFIG_LUCI_LANG_zh_Hans=y' .config

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

group 'Build selected packages in parallel'
targets=()
for package in "${PACKAGES[@]}"; do
	targets+=("package/$package/compile")
done
make \
	BUILD_LOG=1 \
	CONFIG_AUTOREMOVE=y \
	V=s \
	-j "$(nproc)" \
	"${targets[@]}"

find bin/packages -type f -name 'luci-i18n-steer-zh-cn-*.apk' -print -quit | grep -q . || {
	echo 'Simplified Chinese LuCI APK was not produced.' >&2
	exit 1
}

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
