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
	rm -f /var/lib/steer/rollback.uci
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
for method in geodata_catalog plan rollback status validate; do grep -q "\"$method\"" "$TEST_DIR/rpc-methods.txt"; done

ubus call luci.steer geodata_catalog > "$TEST_DIR/geodata-catalog.json"
[ "$(jsonfilter -q -i "$TEST_DIR/geodata-catalog.json" -e '@.geosite.ok')" = 'true' ]
[ "$(jsonfilter -q -i "$TEST_DIR/geodata-catalog.json" -e '@.geoip.ok')" = 'true' ]

# The representative fixture covers all three node protocols, all six DNS
# transports, Geo rules, MAC steering and a local proxy. Its example Geo JSON
# is compiled locally so the native sing-box check also opens real SRS files.
mkdir -p /tmp/steer-m1-state/geodata/current/rules
"$SING_BOX_BIN" rule-set compile --output /tmp/steer-m1-state/geodata/current/rules/geosite-category-example.srs \
	"$REPO_DIR/tests/fixtures/m1-geodata/geosite-category-example.json"
"$SING_BOX_BIN" rule-set compile --output /tmp/steer-m1-state/geodata/current/rules/geoip-example.srs \
	"$REPO_DIR/tests/fixtures/m1-geodata/geoip-example.json"
/usr/sbin/steer validate --config "$REPO_DIR/tests/fixtures/m1-representative-valid/steer" > "$TEST_DIR/representative-validation.json"
/usr/sbin/steer compile-sing-box --config "$REPO_DIR/tests/fixtures/m1-representative-valid/steer" --state-dir /tmp/steer-m1-state > "$TEST_DIR/representative-sing-box.json"
"$SING_BOX_BIN" check -c "$TEST_DIR/representative-sing-box.json"
for value in '"type": "vless"' '"type": "hysteria2"' '"type": "trojan"' '"type": "quic"' '"type": "h3"' 'steer-mac-tproxy-0' 'steer-local-developer_proxy'; do
	grep -q "$value" "$TEST_DIR/representative-sing-box.json"
done
if grep -Eq 'fakeip|udp.*443.*reject|smartdns' "$TEST_DIR/representative-sing-box.json"; then
	echo 'Representative plan contains a forbidden hidden behavior.' >&2
	exit 1
fi

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
core_pid="$(jsonfilter -q -i "$TEST_DIR/status.json" -e '@.core_pid')"
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
trigger_digest_before="$(jsonfilter -q -i "$TEST_DIR/status.json" -e '@.intent_digest')"
trigger_session="$(ubus call session login '{"username":"root","password":"","timeout":300}' | jsonfilter -e '@.ubus_rpc_session')"
ubus call uci set "{\"config\":\"steer\",\"section\":\"main\",\"values\":{\"log_level\":\"warn\"},\"ubus_rpc_session\":\"$trigger_session\"}"
ubus call uci commit "{\"config\":\"steer\",\"ubus_rpc_session\":\"$trigger_session\"}"
trigger_digest=''
for wait_attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
	sleep 1
	/usr/sbin/steer status > "$TEST_DIR/trigger-status.json"
	trigger_digest="$(jsonfilter -q -i "$TEST_DIR/trigger-status.json" -e '@.intent_digest' || true)"
	[ "$trigger_digest" = "$trigger_digest_before" ] || break
done
ubus call session destroy "{\"ubus_rpc_session\":\"$trigger_session\"}"
[ -n "$trigger_digest" ] && [ "$trigger_digest" != "$trigger_digest_before" ]
[ "$(jsonfilter -q -i "$TEST_DIR/trigger-status.json" -e '@.healthy')" = 'true' ]

# fw4 must not delete Steer's separately owned table. A service restart must
# rebuild the same current UCI without probes or a boot LKG.
fw4 reload
nft list table inet steer >/dev/null
/etc/init.d/steer restart
/usr/sbin/steer health --timeout 10s
/etc/init.d/steer reload
/usr/sbin/steer health --timeout 10s

# Default procd respawn is the only runtime recovery. Repeated process death
# must produce a fresh PID without a controller watchdog or configuration roll back.
for attempt in 1 2 3; do
	pid="$(ubus call service list '{"name":"steer"}' | jsonfilter -e '@.steer.instances["sing-box"].pid')"
	kill -9 "$pid"
	new_pid=''
	for wait_attempt in 1 2 3 4 5 6 7 8 9 10; do
		sleep 1
		new_pid="$(ubus call service list '{"name":"steer"}' | jsonfilter -q -e '@.steer.instances["sing-box"].pid' || true)"
		[ -z "$new_pid" ] || break
	done
	[ -n "$new_pid" ] && [ "$new_pid" != "$pid" ]
done
/usr/sbin/steer health --timeout 10s

# HTTPS probes are explicit diagnostics. They never reject an otherwise
# healthy Apply; the single previous healthy UCI remains available for an
# explicit CLI/RPC rollback.
/usr/sbin/steer status > "$TEST_DIR/pre-probe-status.json"
digest_before="$(jsonfilter -q -i "$TEST_DIR/pre-probe-status.json" -e '@.intent_digest')"
uci -q delete steer.main.probe_direct
uci add_list steer.main.probe_direct='https://127.0.0.1:1/'
uci commit steer
/usr/sbin/steer apply > "$TEST_DIR/probe-independent-apply.json"
[ "$(jsonfilter -q -i "$TEST_DIR/probe-independent-apply.json" -e '@.ok')" = 'true' ]
if /usr/sbin/steer probe > "$TEST_DIR/probe-failure.json"; then
	echo 'Unreachable manual HTTPS probe was reported healthy.' >&2
	exit 1
fi
[ "$(jsonfilter -q -i "$TEST_DIR/probe-failure.json" -e '@.ok')" = 'false' ]
ubus call luci.steer status > "$TEST_DIR/rollback-ready-status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/rollback-ready-status.json" -e '@.rollback_available')" = 'true' ]
rollback_started="$(date +%s)"
ubus call luci.steer rollback > "$TEST_DIR/manual-rollback.json"
rollback_elapsed="$(( $(date +%s) - rollback_started ))"
[ "$rollback_elapsed" -lt 20 ] || { echo "Rollback took ${rollback_elapsed}s." >&2; exit 1; }
if [ "$(jsonfilter -q -i "$TEST_DIR/manual-rollback.json" -e '@.ok')" != 'true' ]; then
	echo 'Manual rollback result:' >&2
	cat "$TEST_DIR/manual-rollback.json" >&2
	ubus call service list '{"name":"steer"}' >&2 || true
	logread | tail -n 40 >&2
	exit 1
fi
uci show steer.main.probe_direct | grep -q 'https://openwrt.org/'
/usr/sbin/steer status > "$TEST_DIR/rolled-back-status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/rolled-back-status.json" -e '@.intent_digest')" = "$digest_before" ]
[ "$(jsonfilter -q -i "$TEST_DIR/rolled-back-status.json" -e '@.healthy')" = 'true' ]
[ "$(jsonfilter -q -i "$TEST_DIR/rolled-back-status.json" -e '@.rollback_available')" = 'false' ]

# Invalid schema and unknown legacy fields fail before the running generation
# is stopped or replaced.
uci set steer.main.router_proxy='1'
uci commit steer
if /usr/sbin/steer apply > "$TEST_DIR/rejected-schema.json"; then
	echo 'Legacy schema field was accepted.' >&2
	exit 1
fi
/usr/sbin/steer status > "$TEST_DIR/invalid-status.json"
[ "$(jsonfilter -q -i "$TEST_DIR/invalid-status.json" -e '@.core_running')" = 'true' ]
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

echo 'OpenWrt M1 VM integration tests passed.'
