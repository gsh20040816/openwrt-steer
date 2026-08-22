#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

set -eu

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
REPO_DIR="$(CDPATH='' cd -- "$SCRIPT_DIR/../.." && pwd)"
CONTROL_SOURCE="${STEER_OPENWRT_BIN:-$REPO_DIR/bin/steer-openwrt-linux-amd64}"
SING_BOX_BIN="${SING_BOX_BIN:-/usr/bin/sing-box}"
TEST_DIR="$(mktemp -d /tmp/steer-m1-integration.XXXXXX)"
ORIGINAL_CONFIG="$TEST_DIR/original-steer"

cleanup() {
	status="$?"
	trap - EXIT INT TERM
	/etc/init.d/steer stop >/dev/null 2>&1 || true
	uci -q delete firewall.steertest
	uci -q commit firewall
	ip link delete br-steer type bridge >/dev/null 2>&1 || true
	if [ -f "$ORIGINAL_CONFIG" ]; then
		cp "$ORIGINAL_CONFIG" /etc/config/steer
	fi
	rm -f /usr/sbin/steer
	rm -rf /run/steer "$TEST_DIR" /tmp/steer-m1-state
	exit "$status"
}
trap cleanup EXIT INT TERM

[ "$(ubus call system board | jsonfilter -e '@.release.target')" = 'x86/64' ] || {
	echo 'This integration test requires an OpenWrt x86/64 VM.' >&2
	exit 1
}
for executable in "$CONTROL_SOURCE" "$SING_BOX_BIN" /usr/bin/geoview /usr/sbin/nft; do
	[ -x "$executable" ] || { echo "Required executable is missing: $executable" >&2; exit 1; }
done
for seed in release geosite.dat geoip.dat; do
	[ -s "/usr/share/steer/geodata-seed/$seed" ] || { echo "Geo seed is missing: $seed" >&2; exit 1; }
done

[ ! -f /etc/config/steer ] || cp /etc/config/steer "$ORIGINAL_CONFIG"
/etc/init.d/steer stop >/dev/null 2>&1 || true
ln -sf "$CONTROL_SOURCE" /usr/sbin/steer
cp "$REPO_DIR/steer-openwrt/files/etc/init.d/steer" /etc/init.d/steer
chmod 0755 /usr/sbin/steer /etc/init.d/steer

mkdir -p /usr/share/rpcd/ucode
ucode -c "$REPO_DIR/luci-app-steer/root/usr/share/rpcd/ucode/luci.steer" >/dev/null
cp "$REPO_DIR/luci-app-steer/root/usr/share/rpcd/ucode/luci.steer" /usr/share/rpcd/ucode/luci.steer
chmod 0755 /usr/share/rpcd/ucode/luci.steer
/etc/init.d/rpcd restart
for attempt in 1 2 3 4 5; do
	ubus -t 1 list luci.steer >/dev/null 2>&1 && break
	[ "$attempt" -lt 5 ] || { echo 'luci.steer RPC did not become ready.' >&2; exit 1; }
	sleep 1
done
ubus -v list luci.steer > "$TEST_DIR/rpc-methods.txt"
for method in geodata_catalog node_speedtest overview_probe route_speedtest status subscriptions validate; do
	grep -q "\"$method\"" "$TEST_DIR/rpc-methods.txt"
done
for removed in plan rollback; do
	if grep -q "\"$removed\"" "$TEST_DIR/rpc-methods.txt"; then
		echo "Removed RPC method is still public: $removed" >&2
		exit 1
	fi
done

ubus call luci.steer geodata_catalog > "$TEST_DIR/geodata-catalog.json"
[ "$(jsonfilter -q -i "$TEST_DIR/geodata-catalog.json" -e '@.geosite.ok')" = 'true' ]
[ "$(jsonfilter -q -i "$TEST_DIR/geodata-catalog.json" -e '@.geoip.ok')" = 'true' ]

# Representative and detour fixtures remain valid through the public semantic
# validator. Native compilation is exercised by normal Apply below and by the
# shared Go compiler tests; no engineering compiler command is public.
/usr/sbin/steer validate --config "$REPO_DIR/tests/fixtures/m1-representative-valid/steer" > "$TEST_DIR/representative-validation.json"
/usr/sbin/steer validate --config "$REPO_DIR/tests/fixtures/schema7-detour-valid/steer" > "$TEST_DIR/detour-validation.json"

cp "$REPO_DIR/tests/fixtures/m1-openwrt-direct-valid/steer" /etc/config/steer

# Package installation/boot establishes the procd service and its config
# trigger before LuCI can edit an already running configuration.
/usr/sbin/steer apply > "$TEST_DIR/initial-apply.json"
[ "$(jsonfilter -q -i "$TEST_DIR/initial-apply.json" -e '@.ok')" = 'true' ]

# Reproduce the LuCI form lifecycle with a real authenticated UCI session.
# The disk value must remain old until the session-scoped commit completes,
# and the first Steer Apply after that commit must compile the new value.
luci_session="$(ubus call session login '{"username":"root","password":"","timeout":300}' | jsonfilter -e '@.ubus_rpc_session')"
ubus call uci set "{\"config\":\"steer\",\"section\":\"main\",\"values\":{\"log_level\":\"error\"},\"ubus_rpc_session\":\"$luci_session\"}"
[ "$(uci get steer.main.log_level)" = 'warn' ]
luci_apply_sequence_before="$(ubus call luci.steer status | jsonfilter -q -e '@.last_apply.sequence' || true)"
ubus call uci commit "{\"config\":\"steer\",\"ubus_rpc_session\":\"$luci_session\"}"
[ "$(uci get steer.main.log_level)" = 'error' ]
for wait_attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
	sleep 1
	ubus call luci.steer status > "$TEST_DIR/apply-status.json"
	luci_apply_sequence="$(jsonfilter -q -i "$TEST_DIR/apply-status.json" -e '@.last_apply.sequence' || true)"
	[ -z "$luci_apply_sequence" ] || [ "$luci_apply_sequence" = "$luci_apply_sequence_before" ] || break
done
ubus call session destroy "{\"ubus_rpc_session\":\"$luci_session\"}"
[ -n "$luci_apply_sequence" ] && [ "$luci_apply_sequence" != "$luci_apply_sequence_before" ]
[ "$(jsonfilter -q -i "$TEST_DIR/apply-status.json" -e '@.last_apply.result.ok')" = 'true' ]
[ "$(jsonfilter -q -i /run/steer/current/sing-box.json -e '@.log.level')" = 'error' ]
sleep 3
ubus call luci.steer status > "$TEST_DIR/apply-stable-status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/apply-stable-status.json" -e '@.last_apply.sequence')" = "$luci_apply_sequence" ]
/usr/sbin/steer health --timeout 10s
/usr/sbin/steer status > "$TEST_DIR/status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/status.json" -e '@.healthy')" = 'true' ]
core_pid="$(ubus call service list '{"name":"steer"}' | jsonfilter -e '@.steer.instances["sing-box"].pid')"
grep -Eq "^[[:space:]]*100[[:space:]]+$core_pid[[:space:]]" /proc/net/netfilter/nfnetlink_queue

nft list table inet steer > "$TEST_DIR/steer.nft"
grep -q 'dnat ip to 127.0.0.1:1053' "$TEST_DIR/steer.nft"
grep -q 'dnat ip6 to \[::1\]:1053' "$TEST_DIR/steer.nft"
grep -q 'snat ip to 127.0.0.1' "$TEST_DIR/steer.nft"
grep -q 'snat ip6 to ::1' "$TEST_DIR/steer.nft"
grep -q 'ether saddr 02:00:00:00:00:10' "$TEST_DIR/steer.nft"
grep -q 'tproxy to :20000' "$TEST_DIR/steer.nft"
if grep -Eq 'udp dport 443.*(drop|reject)' "$TEST_DIR/steer.nft"; then
	echo 'Steer blocks QUIC instead of routing it.' >&2
	exit 1
fi
ip -4 rule show | grep -Eq '8999:.*fwmark 0x2026.*lookup 2023'
ip -6 rule show | grep -Eq '8999:.*fwmark 0x2026.*lookup 2023'
nslookup openwrt.org 1.1.1.1 > "$TEST_DIR/dns.txt"
grep -q 'Address' "$TEST_DIR/dns.txt"

# A second authenticated UCI commit must still route through the same
# transactional reload_service path without requiring a second Apply.
trigger_sequence_before="$(jsonfilter -q -i "$TEST_DIR/status.json" -e '@.last_apply.sequence')"
trigger_session="$(ubus call session login '{"username":"root","password":"","timeout":300}' | jsonfilter -e '@.ubus_rpc_session')"
ubus call uci set "{\"config\":\"steer\",\"section\":\"main\",\"values\":{\"log_level\":\"warn\"},\"ubus_rpc_session\":\"$trigger_session\"}"
ubus call uci commit "{\"config\":\"steer\",\"ubus_rpc_session\":\"$trigger_session\"}"
trigger_sequence=''
for wait_attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
	sleep 1
	/usr/sbin/steer status > "$TEST_DIR/trigger-status.json"
	trigger_sequence="$(jsonfilter -q -i "$TEST_DIR/trigger-status.json" -e '@.last_apply.sequence' || true)"
	[ "$trigger_sequence" = "$trigger_sequence_before" ] || break
done
ubus call session destroy "{\"ubus_rpc_session\":\"$trigger_session\"}"
[ -n "$trigger_sequence" ] && [ "$trigger_sequence" != "$trigger_sequence_before" ]
[ "$(jsonfilter -q -i "$TEST_DIR/trigger-status.json" -e '@.healthy')" = 'true' ]
[ "$(jsonfilter -q -i /run/steer/current/sing-box.json -e '@.log.level')" = 'warn' ]

# fw4 must not delete Steer's separately owned table. A service restart must
# rebuild the current UCI through the private init hook.
fw4 reload
nft list table inet steer >/dev/null
/etc/init.d/steer restart
/usr/sbin/steer health --timeout 10s
/etc/init.d/steer reload
/usr/sbin/steer health --timeout 10s

# Invalid schema and unknown legacy fields fail before the running generation
# is stopped or replaced.
uci set steer.main.router_proxy='1'
uci commit steer
if /usr/sbin/steer apply > "$TEST_DIR/rejected-schema.json"; then
	echo 'Legacy schema field was accepted.' >&2
	exit 1
fi
/usr/sbin/steer status > "$TEST_DIR/invalid-status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/invalid-status.json" -e '@.healthy')" = 'true' ]
uci -q delete steer.main.router_proxy
uci commit steer

uci set steer.main.enabled='0'
uci commit steer
/usr/sbin/steer apply > "$TEST_DIR/disabled.json"
[ "$(jsonfilter -q -i "$TEST_DIR/disabled.json" -e '@.ok')" = 'true' ]
if nft list table inet steer >/dev/null 2>&1; then
	echo 'Disabled Apply retained the Steer nftables table.' >&2
	exit 1
fi
[ ! -e /run/steer/current ]

# Disabling removes runtime resources but must not unregister the config
# trigger needed for the next LuCI commit to enable Steer again.
ubus call luci.steer status > "$TEST_DIR/disabled-status.json"
disabled_sequence="$(jsonfilter -q -i "$TEST_DIR/disabled-status.json" -e '@.last_apply.sequence')"
reenable_session="$(ubus call session login '{"username":"root","password":"","timeout":300}' | jsonfilter -e '@.ubus_rpc_session')"
ubus call uci set "{\"config\":\"steer\",\"section\":\"main\",\"values\":{\"enabled\":\"1\"},\"ubus_rpc_session\":\"$reenable_session\"}"
ubus call uci commit "{\"config\":\"steer\",\"ubus_rpc_session\":\"$reenable_session\"}"
for wait_attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
	sleep 1
	ubus call luci.steer status > "$TEST_DIR/reenabled-status.json"
	reenabled_sequence="$(jsonfilter -q -i "$TEST_DIR/reenabled-status.json" -e '@.last_apply.sequence' || true)"
	[ -z "$reenabled_sequence" ] || [ "$reenabled_sequence" = "$disabled_sequence" ] || break
done
ubus call session destroy "{\"ubus_rpc_session\":\"$reenable_session\"}"
[ "$(jsonfilter -q -i "$TEST_DIR/reenabled-status.json" -e '@.last_apply.result.ok')" = 'true' ]
[ "$(jsonfilter -q -i "$TEST_DIR/reenabled-status.json" -e '@.healthy')" = 'true' ]

echo 'OpenWrt cross-platform-preparation VM integration tests passed.'
