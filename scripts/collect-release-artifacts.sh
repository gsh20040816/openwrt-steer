#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

set -eu

package_root="${1:-bin/packages/x86_64}"
output_dir="${2:-dist}"

[ -d "$package_root" ] || {
	echo "Package directory not found: $package_root" >&2
	exit 1
}

mkdir -p "$output_dir"

for package in geoview steer-geodata steer luci-app-steer luci-i18n-steer-zh-cn; do
	pattern="$package-*.apk"
	[ "$package" != steer ] || pattern='steer-[0-9]*.apk'
	set -- $(find "$package_root" -type f -name "$pattern" | sort)
	if [ "$#" -ne 1 ]; then
		echo "Expected exactly one $package APK, found $#" >&2
		exit 1
	fi
	cp "$1" "$output_dir/"
done

{
	echo "OpenWrt release: ${OPENWRT_RELEASE:-unknown}"
	echo "OpenWrt target: ${OPENWRT_TARGET:-unknown}"
	echo "Source revision: ${SOURCE_REVISION:-unknown}"
	echo "geoview ref: ${GEOVIEW_REF:-unknown}"
} > "$output_dir/BUILD-METADATA.txt"

(
	cd "$output_dir"
	sha256sum ./*.apk BUILD-METADATA.txt > SHA256SUMS
)
