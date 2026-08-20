#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

set -eu

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
REPO_DIR="$(CDPATH='' cd -- "$SCRIPT_DIR/../.." && pwd)"
SING_BOX_BIN="${SING_BOX_BIN:-/usr/bin/sing-box}"
SMARTDNS_BIN="${SMARTDNS_BIN:-/usr/sbin/smartdns}"
TEST_DIR="$(mktemp -d /tmp/steer-integration.XXXXXX)"
ORIGINAL_CONFIG="$TEST_DIR/original-steer"
ORIGINAL_FIREWALL="$TEST_DIR/original-firewall"
ORIGINAL_GEODATA="$TEST_DIR/original-geodata"
HAD_GEODATA='0'
SMARTDNS_WAS_ENABLED='0'
SMARTDNS_WAS_RUNNING='0'
SMARTDNS_STATE_CAPTURED='0'

cleanup() {
	status="$?"
	trap - EXIT INT TERM
	/etc/init.d/steer stop >/dev/null 2>&1 || true
	if [ "$SMARTDNS_STATE_CAPTURED" = '1' ]; then
		if [ "$SMARTDNS_WAS_ENABLED" = '1' ]; then
			/etc/init.d/smartdns enable >/dev/null 2>&1 || true
		else
			/etc/init.d/smartdns disable >/dev/null 2>&1 || true
		fi
		if [ "$SMARTDNS_WAS_RUNNING" = '1' ]; then
			/etc/init.d/smartdns start >/dev/null 2>&1 || true
		else
			/etc/init.d/smartdns stop >/dev/null 2>&1 || true
		fi
	fi
	ip -6 address del 2001:db8:ffff::1/64 dev br-lan >/dev/null 2>&1 || true
	ip -6 route del default dev br-lan metric 4096 >/dev/null 2>&1 || true
	rm -rf /var/lib/steer/geodata
	if [ "$HAD_GEODATA" = '1' ]; then
		cp -a "$ORIGINAL_GEODATA" /var/lib/steer/geodata
	fi
	if [ -f "$ORIGINAL_CONFIG" ]; then
		cp "$ORIGINAL_CONFIG" /etc/config/steer
	fi
	if [ -f "$ORIGINAL_FIREWALL" ]; then
		cp "$ORIGINAL_FIREWALL" /etc/config/firewall
		fw4 reload >/dev/null 2>&1 || true
	fi
	rm -rf "$TEST_DIR"
	exit "$status"
}

trap cleanup EXIT INT TERM

if [ "$(ubus call system board | jsonfilter -e '@.release.target')" != 'x86/64' ]; then
	echo 'This integration test requires an OpenWrt x86/64 VM.' >&2
	exit 1
fi

if [ ! -x "$SING_BOX_BIN" ]; then
	echo "sing-box binary not found: $SING_BOX_BIN" >&2
	exit 1
fi

if [ ! -x "$SMARTDNS_BIN" ]; then
	echo "SmartDNS binary not found: $SMARTDNS_BIN" >&2
	exit 1
fi

if /etc/init.d/smartdns enabled >/dev/null 2>&1; then
	SMARTDNS_WAS_ENABLED='1'
fi
if ubus call service list '{"name":"smartdns"}' 2>/dev/null | grep -q '"running": true'; then
	SMARTDNS_WAS_RUNNING='1'
fi
SMARTDNS_STATE_CAPTURED='1'
/etc/init.d/smartdns stop >/dev/null 2>&1 || true
/etc/init.d/smartdns disable >/dev/null 2>&1 || true

if [ -f /etc/config/steer ]; then
	cp /etc/config/steer "$ORIGINAL_CONFIG"
fi
cp /etc/config/firewall "$ORIGINAL_FIREWALL"
if [ -d /var/lib/steer/geodata ]; then
	cp -a /var/lib/steer/geodata "$ORIGINAL_GEODATA"
	HAD_GEODATA='1'
	rm -rf /var/lib/steer/geodata
fi

mkdir -p /usr/share/steer /usr/sbin /usr/libexec/steer /etc/init.d /etc/uci-defaults /var/lib/steer/smartdns
cp "$REPO_DIR/steer/files/usr/share/steer/model.uc" /usr/share/steer/model.uc
cp "$REPO_DIR/steer/files/usr/share/steer/firewall.uc" /usr/share/steer/firewall.uc
cp "$REPO_DIR/steer/files/usr/share/steer/firewall.include" /usr/share/steer/firewall.include
cp "$REPO_DIR/steer/files/usr/sbin/steerctl" /usr/sbin/steerctl
cp "$REPO_DIR/steer/files/usr/libexec/steer/runtime" /usr/libexec/steer/runtime
cp "$REPO_DIR/steer/files/usr/libexec/steer/geodata" /usr/libexec/steer/geodata
cp "$REPO_DIR/steer/files/etc/init.d/steer" /etc/init.d/steer
cp "$REPO_DIR/steer/files/etc/uci-defaults/99-steer-firewall" /etc/uci-defaults/99-steer-firewall
chmod 0755 /usr/sbin/steerctl /usr/libexec/steer/runtime /usr/libexec/steer/geodata \
	/etc/init.d/steer \
	/usr/share/steer/firewall.include /etc/uci-defaults/99-steer-firewall
/etc/init.d/steer stop >/dev/null 2>&1 || true
/etc/uci-defaults/99-steer-firewall

mkdir -p /usr/share/rpcd/ucode
ucode -c "$REPO_DIR/luci-app-steer/root/usr/share/rpcd/ucode/luci.steer" >/dev/null
cp "$REPO_DIR/luci-app-steer/root/usr/share/rpcd/ucode/luci.steer" /usr/share/rpcd/ucode/luci.steer
chmod 0755 /usr/share/rpcd/ucode/luci.steer
/etc/init.d/rpcd restart
rpc_ready='0'
for attempt in 1 2 3 4 5; do
	if ubus -t 1 list luci.steer >/dev/null 2>&1; then
		rpc_ready='1'
		break
	fi
	sleep 1
done
if [ "$rpc_ready" != '1' ]; then
	echo 'luci.steer RPC object did not become ready after rpcd restart.' >&2
	exit 1
fi
ubus -v list luci.steer > "$TEST_DIR/luci-steer-methods.txt"
grep -q '"apply"' "$TEST_DIR/luci-steer-methods.txt"
grep -q '"geodata_catalog"' "$TEST_DIR/luci-steer-methods.txt"
if grep -q '"geodata_update"' "$TEST_DIR/luci-steer-methods.txt"; then
	echo 'LuCI still exposes a router-managed geodata updater.' >&2
	exit 1
fi

ubus call luci.steer geodata_catalog > "$TEST_DIR/luci-geodata-catalog.json"
[ "$(jsonfilter -q -i "$TEST_DIR/luci-geodata-catalog.json" -e '@.geosite.ok')" = 'true' ]
[ "$(jsonfilter -q -i "$TEST_DIR/luci-geodata-catalog.json" -e '@.geoip.ok')" = 'true' ]
grep -q '"category-games@cn"' "$TEST_DIR/luci-geodata-catalog.json"
grep -q '"cn"' "$TEST_DIR/luci-geodata-catalog.json"

ucode -S "$REPO_DIR/tests/ucode/model_test.uc"

cp "$REPO_DIR/tests/fixtures/representative-valid/steer" /etc/config/steer
steerctl validate > "$TEST_DIR/valid-result.json"
steerctl compile-sing-box > "$TEST_DIR/sing-box.json"
"$SING_BOX_BIN" check -c "$TEST_DIR/sing-box.json"
grep -q '"tag": "steer-mac-tproxy-0"' "$TEST_DIR/sing-box.json"
grep -q '"tag": "steer-mac-dns-0"' "$TEST_DIR/sing-box.json"
if grep -q '"source_mac_address"' "$TEST_DIR/sing-box.json"; then
	echo 'sing-box 1.13 candidate unexpectedly contains a 1.14-only source_mac_address field.' >&2
	exit 1
fi

# LuCI adds a new UCI section at the physical end of the package. Default is a
# semantic terminator, so model loading must still place the new ordinary rule
# before it instead of exposing storage order as policy order.
uci set steer.after_default='rule'
uci set steer.after_default.name='After Default storage test'
uci set steer.after_default.enabled='1'
uci add_list steer.after_default.domain_match='domain:storage-order.example'
uci set steer.after_default.dns_profile='direct_dns'
uci set steer.after_default.outbound='direct'
uci commit steer
steerctl validate > "$TEST_DIR/default-pinned-result.json"
steerctl compile-sing-box > "$TEST_DIR/default-pinned-sing-box.json"
grep -q 'storage-order.example' "$TEST_DIR/default-pinned-sing-box.json"
uci delete steer.after_default
uci commit steer

cp "$REPO_DIR/tests/fixtures/geo-rules-valid/steer" /etc/config/steer
/usr/libexec/steer/geodata ensure
[ "$(cat /var/lib/steer/geodata/current/package.release)" = \
	"$(cat /usr/share/steer/geodata-seed/release)" ]
[ -s /var/lib/steer/geodata/current/rules/geosite-category-example.srs ]
geodata_previous="$(readlink -f /var/lib/steer/geodata/current)"
geodata_staged=/var/lib/steer/geodata/generation.rollback-test
geodata_candidate="$TEST_DIR/geodata-rollback-candidate"
mkdir -p "$geodata_staged" "$geodata_candidate"
printf '%s\n' "$geodata_previous" > "$geodata_candidate/geodata.previous"
printf '%s\n' "$geodata_staged" > "$geodata_candidate/geodata.staged"
ln -s "$geodata_staged" /var/lib/steer/geodata/current.rollback-test
mv -fT /var/lib/steer/geodata/current.rollback-test /var/lib/steer/geodata/current
/usr/libexec/steer/runtime restore-geodata "$geodata_candidate"
[ "$(readlink -f /var/lib/steer/geodata/current)" = "$geodata_previous" ]
rm -rf "$geodata_staged" "$geodata_candidate"
steerctl compile-sing-box > "$TEST_DIR/geo-rules-sing-box.json"
"$SING_BOX_BIN" check -c "$TEST_DIR/geo-rules-sing-box.json"
rm -rf /var/lib/steer/geodata
cp "$REPO_DIR/tests/fixtures/representative-valid/steer" /etc/config/steer

cp "$REPO_DIR/tests/fixtures/local-proxy-valid/steer" /etc/config/steer
steerctl compile-sing-box > "$TEST_DIR/local-proxy-sing-box.json"
"$SING_BOX_BIN" check -c "$TEST_DIR/local-proxy-sing-box.json"
cp "$REPO_DIR/tests/fixtures/representative-valid/steer" /etc/config/steer

for profile in direct_dns proxy_dns; do
	config="$TEST_DIR/smartdns-$profile.conf"
	steerctl compile-smartdns "$profile" > "$config"
	"$SMARTDNS_BIN" -f -x -c "$config" -p - &
	pid="$!"
	sleep 1
	if ! kill -0 "$pid" 2>/dev/null; then
		echo "SmartDNS instance failed to stay running: $profile" >&2
		wait "$pid"
		exit 1
	fi
	kill "$pid"
	# The process was intentionally terminated after proving it stayed up.
	wait "$pid" || true
done

uci set steer.main.enabled='1'
uci commit steer

# A separately managed system SmartDNS would compete with Steer's per-profile
# instances. The failure must be explicit, persisted for LuCI and happen before
# Steer takes over nftables or starts its core.
/etc/init.d/smartdns enable
if steerctl apply > "$TEST_DIR/smartdns-conflict.log" 2>&1; then
	echo 'Steer unexpectedly started while the system SmartDNS was enabled.' >&2
	exit 1
fi
grep -q '系统 SmartDNS 已启用' "$TEST_DIR/smartdns-conflict.log"
ubus call luci.steer status > "$TEST_DIR/smartdns-conflict-status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/smartdns-conflict-status.json" -e '@.runtime_state')" = 'failed' ]
[ "$(jsonfilter -q -i "$TEST_DIR/smartdns-conflict-status.json" -e '@.conflicts[0].name')" = 'smartdns' ]
if nft list table inet steer >/dev/null 2>&1; then
	echo 'Steer loaded nftables despite the system SmartDNS conflict.' >&2
	exit 1
fi
/etc/init.d/smartdns disable

ubus call luci.steer apply > "$TEST_DIR/luci-apply.json"
if [ "$(jsonfilter -q -i "$TEST_DIR/luci-apply.json" -e '@.ok')" != 'true' ]; then
	echo 'LuCI Apply failed:' >&2
	cat "$TEST_DIR/luci-apply.json" >&2
	exit 1
fi
ubus call luci.steer status > "$TEST_DIR/luci-status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/luci-status.json" -e '@.core_running')" = 'true' ]
[ "$(jsonfilter -q -i "$TEST_DIR/luci-status.json" -e '@.network_loaded')" = 'true' ]
[ "$(jsonfilter -q -i "$TEST_DIR/luci-status.json" -e '@.runtime_state')" = 'active' ]
[ "$(jsonfilter -q -i "$TEST_DIR/luci-status.json" -e '@.desired_enabled')" = 'true' ]
[ "$(jsonfilter -q -i "$TEST_DIR/luci-status.json" -e '@.dns_running')" = \
		"$(jsonfilter -q -i "$TEST_DIR/luci-status.json" -e '@.dns_total')" ]
/etc/init.d/steer enabled
/usr/libexec/steer/runtime health-check

/etc/init.d/smartdns enable
if steerctl apply > "$TEST_DIR/active-smartdns-conflict.log" 2>&1; then
	echo 'Steer stayed active while the system SmartDNS became enabled.' >&2
	exit 1
fi
grep -q 'Steer 已停止' "$TEST_DIR/active-smartdns-conflict.log"
if nft list table inet steer >/dev/null 2>&1; then
	echo 'Steer kept network interception after detecting an active conflict.' >&2
	exit 1
fi
ubus call luci.steer status > "$TEST_DIR/active-smartdns-conflict-status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/active-smartdns-conflict-status.json" -e '@.runtime_state')" = 'failed' ]
[ "$(jsonfilter -q -i "$TEST_DIR/active-smartdns-conflict-status.json" -e '@.core_running')" = 'false' ]
/etc/init.d/smartdns disable
ubus call luci.steer apply > "$TEST_DIR/restart-after-conflict.json"
[ "$(jsonfilter -q -i "$TEST_DIR/restart-after-conflict.json" -e '@.ok')" = 'true' ]
/usr/libexec/steer/runtime health-check

nft list table inet steer > "$TEST_DIR/steer-router-enabled.nft"
grep -q 'type route hook output priority mangle' "$TEST_DIR/steer-router-enabled.nft"
grep -q 'type nat hook output priority dstnat + 1' "$TEST_DIR/steer-router-enabled.nft"
grep -q 'iifname "lo"' "$TEST_DIR/steer-router-enabled.nft"
grep -q 'ether saddr 02:00:00:00:00:10' "$TEST_DIR/steer-router-enabled.nft"
grep -q 'redirect to :49153' "$TEST_DIR/steer-router-enabled.nft"
grep -q 'tproxy to :49152' "$TEST_DIR/steer-router-enabled.nft"

uci set steer.main.router_proxy='0'
uci commit steer
steerctl apply > "$TEST_DIR/router-proxy-disabled.log"
nft list table inet steer > "$TEST_DIR/steer-router-disabled.nft"
if grep -q 'hook output' "$TEST_DIR/steer-router-disabled.nft"; then
	echo 'Router output hooks remained after router proxying was disabled.' >&2
	exit 1
fi
if grep -q 'iifname "lo"' "$TEST_DIR/steer-router-disabled.nft"; then
	echo 'Router TPROXY delivery remained after router proxying was disabled.' >&2
	exit 1
fi

uci set steer.main.router_proxy='1'
uci commit steer
steerctl apply > "$TEST_DIR/router-proxy-enabled.log"
/usr/libexec/steer/runtime health-check

uci set steer.default.outbound='direct'
uci set steer.default.dns_profile='direct_dns'
uci set steer.resolver_a.protocol='udp'
uci set steer.resolver_a.server_port='53'
uci -q delete steer.resolver_a.path
uci -q delete steer.resolver_a.tls_server_name
uci -q delete steer.resolver_a_direct.port
uci add_list steer.resolver_a_direct.port='53'
uci commit steer
steerctl apply > "$TEST_DIR/router-data-path-apply.log"

ntp_before="$(nft list counter inet steer router_ntp_direct | awk '{ for (i = 1; i <= NF; i++) if ($i == "packets") { print $(i + 1); exit } }')"
/etc/init.d/sysntpd restart
sleep 3
ntp_after="$(nft list counter inet steer router_ntp_direct | awk '{ for (i = 1; i <= NF; i++) if ($i == "packets") { print $(i + 1); exit } }')"
if [ "$ntp_after" -le "$ntp_before" ]; then
	echo 'Router-originated UDP/123 did not hit the fixed NTP direct rule.' >&2
	exit 1
fi

nslookup openwrt.org 127.0.0.1 > "$TEST_DIR/router-dns-query.log"
grep -q 'Address:' "$TEST_DIR/router-dns-query.log"
nslookup "steer-integration-$$.openwrt.org" 127.0.0.1 \
	> "$TEST_DIR/router-dns-upstream-probe.log" 2>&1 || true
wget -T 5 -O /dev/null http://1.1.1.1/ > "$TEST_DIR/router-tcp.log" 2>&1 || true
traceroute -n -m 1 -q 1 -w 1 -p 443 1.1.1.1 > "$TEST_DIR/router-udp.log" 2>&1 || true
nft list counter inet steer router_dns > "$TEST_DIR/router-dns-counter.nft"
nft list counter inet steer router_ntp_direct > "$TEST_DIR/router-ntp-direct-counter.nft"
nft list counter inet steer smartdns_upstream > "$TEST_DIR/smartdns-upstream-counter.nft"
nft list counter inet steer router_marked > "$TEST_DIR/router-marked-counter.nft"
nft list counter inet steer router_tproxied > "$TEST_DIR/router-tproxied-counter.nft"
grep -Eq 'packets [1-9][0-9]*' "$TEST_DIR/router-dns-counter.nft"
grep -Eq 'packets [1-9][0-9]*' "$TEST_DIR/router-ntp-direct-counter.nft"
grep -Eq 'packets [1-9][0-9]*' "$TEST_DIR/smartdns-upstream-counter.nft"
grep -Eq 'packets [1-9][0-9]*' "$TEST_DIR/router-marked-counter.nft"
grep -Eq 'packets [1-9][0-9]*' "$TEST_DIR/router-tproxied-counter.nft"
ubus call luci.steer status > "$TEST_DIR/router-status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/router-status.json" -e '@.iana_registry_date')" = '2025-10-09' ]
[ "$(jsonfilter -q -i "$TEST_DIR/router-status.json" -e '@.router_ntp_direct_packets')" -ge '1' ]

# The isolated VM has no delegated IPv6 prefix. Give the test interface a
# documentation-only source address so the kernel actually emits the probe;
# otherwise traceroute exits before the nftables output hook is reached.
ip -6 address add 2001:db8:ffff::1/64 dev br-lan
sleep 2
ip -6 route add default dev br-lan metric 4096
/usr/libexec/steer/runtime firewall-reload
traceroute -6 -n -m 1 -q 1 -w 1 -p 443 2606:4700:4700::1111 \
	> "$TEST_DIR/router-ipv6-udp.log" 2>&1 || true
nft list counter inet steer router_marked > "$TEST_DIR/router-ipv6-marked-counter.nft"
nft list counter inet steer router_tproxied > "$TEST_DIR/router-ipv6-tproxied-counter.nft"
ip -6 route del default dev br-lan metric 4096
ip -6 address del 2001:db8:ffff::1/64 dev br-lan
if ! grep -Eq 'packets [1-9][0-9]*' "$TEST_DIR/router-ipv6-marked-counter.nft"; then
	echo 'Router-originated IPv6 UDP did not reach the Steer marking rule.' >&2
	cat "$TEST_DIR/router-ipv6-udp.log" >&2
	cat "$TEST_DIR/router-ipv6-marked-counter.nft" >&2
	exit 1
fi
if ! grep -Eq 'packets [1-9][0-9]*' "$TEST_DIR/router-ipv6-tproxied-counter.nft"; then
	echo 'Router-originated IPv6 UDP was marked but did not reach TPROXY.' >&2
	cat "$TEST_DIR/router-ipv6-tproxied-counter.nft" >&2
	exit 1
fi

active_before="$(/usr/libexec/steer/runtime active-directory)"
[ -d /etc/steer/last-known-good ]
nft list table inet steer > "$TEST_DIR/steer.nft"
grep -q 'br-lan' "$TEST_DIR/steer.nft"
grep -q 'tproxy to :1042' "$TEST_DIR/steer.nft"
ip -4 rule show | grep -q 'fwmark 0x80/0xff lookup 80'
ip -6 rule show | grep -q 'fwmark 0x80/0xff lookup 80'

fw4 reload
nft list table inet steer >/dev/null

uci set steer.main.routing_mark='128'
uci commit steer
if steerctl apply > "$TEST_DIR/rejected-apply.log" 2>&1; then
	echo 'A mark collision unexpectedly replaced the running generation.' >&2
	exit 1
fi
[ "$(/usr/libexec/steer/runtime active-directory)" = "$active_before" ]
/usr/libexec/steer/runtime health-check
uci set steer.main.routing_mark='129'
uci commit steer

uci set steer.main.enabled='0'
uci commit steer
steerctl apply > "$TEST_DIR/disabled-apply.log"
if nft list table inet steer >/dev/null 2>&1; then
	echo 'Steer nftables table remained after applying the disabled state.' >&2
	exit 1
fi
if /etc/init.d/steer enabled; then
	echo 'Steer boot entries remained enabled after applying the disabled state.' >&2
	exit 1
fi
ubus call luci.steer status > "$TEST_DIR/disabled-status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/disabled-status.json" -e '@.runtime_state')" = 'disabled' ]
[ "$(jsonfilter -q -i "$TEST_DIR/disabled-status.json" -e '@.desired_enabled')" = 'false' ]

cp "$REPO_DIR/tests/fixtures/dangling-reference-invalid/steer" /etc/config/steer
if steerctl validate > "$TEST_DIR/invalid-result.json"; then
	echo 'Dangling reference unexpectedly passed validation.' >&2
	exit 1
fi

if ! grep -q 'DANGLING_OUTBOUND' "$TEST_DIR/invalid-result.json"; then
	echo 'Dangling reference did not report DANGLING_OUTBOUND.' >&2
	exit 1
fi

echo 'OpenWrt VM integration tests passed.'
