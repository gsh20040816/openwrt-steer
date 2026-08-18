/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 *
 * Deterministic nftables renderer for Steer's LAN ingress and router output data plane.
 */

'use strict';

const IANA_REGISTRY_DATE = '2025-10-09';

/*
 * IANA Special-Purpose Address Registry entries whose Globally Reachable
 * value is not True, plus the IP multicast ranges. More-specific globally
 * reachable registry entries are routed around these parent prefixes below.
 */
const NON_GLOBAL_IPV4 = [
	'0.0.0.0/8',
	'10.0.0.0/8',
	'100.64.0.0/10',
	'127.0.0.0/8',
	'169.254.0.0/16',
	'172.16.0.0/12',
	'192.0.0.0/24',
	'192.0.2.0/24',
	'192.88.99.0/24',
	'192.168.0.0/16',
	'198.18.0.0/15',
	'198.51.100.0/24',
	'203.0.113.0/24',
	'224.0.0.0/4',
	'240.0.0.0/4'
];

const GLOBALLY_REACHABLE_SPECIAL_IPV4 = [
	'192.0.0.9/32',
	'192.0.0.10/32'
];

const NON_GLOBAL_IPV6 = [
	'::/128',
	'::1/128',
	'::ffff:0:0/96',
	'64:ff9b:1::/48',
	'100::/64',
	'100:0:0:1::/64',
	'2001::/23',
	'2001:db8::/32',
	'2002::/16',
	'3fff::/20',
	'5f00::/16',
	'fc00::/7',
	'fe80::/10',
	'ff00::/8'
];

const GLOBALLY_REACHABLE_SPECIAL_IPV6 = [
	'2001:1::1/128',
	'2001:1::2/128',
	'2001:1::3/128',
	'2001:3::/32',
	'2001:4:112::/48',
	'2001:20::/28',
	'2001:30::/28'
];

function valid_table_name(value) {
	return type(value) == 'string' && match(value, /^[a-z][a-z0-9_]{0,31}$/) != null;
}

function valid_device(value) {
	return type(value) == 'string' &&
		length(value) >= 1 && length(value) <= 15 &&
		match(value, /^[A-Za-z0-9_.:-]+$/) != null;
}

function unique_devices(devices) {
	let seen = {};
	let result = [];

	for (let device in devices || []) {
		if (!valid_device(device))
			return null;

		if (seen[device] == null) {
			seen[device] = true;
			push(result, device);
		}
	}

	return sort(result);
}

function hex32(value) {
	return sprintf('0x%08x', value & 0xffffffff);
}

function render_set(name, type_name, elements, quote) {
	const rendered = map(elements, (value) => quote ? `"${value}"` : value);
	return join('\n', [
		`\tset ${name} {`,
		`\t\ttype ${type_name}`,
		'\t\tflags interval',
		'\t\tauto-merge',
		`\t\telements = { ${join(', ', rendered)} }`,
		'\t}'
	]);
}

export function compile_firewall(main, requested_devices, table_name) {
	table_name ??= 'steer';

	if (!valid_table_name(table_name))
		return { ok: false, code: 'INVALID_TABLE_NAME', message: 'nftables 表名无效' };

	const devices = unique_devices(requested_devices);
	if (devices == null)
		return { ok: false, code: 'INVALID_DEVICE', message: 'firewall zone 解析出了非法设备名' };
	if (length(devices) == 0)
		return { ok: false, code: 'NO_MANAGED_DEVICE', message: '受管 firewall zone 当前没有实际设备' };

	const mask = main.mark_mask;
	const inverse_mask = (~mask) & 0xffffffff;
	let lines = [
		`table inet ${table_name} {`,
		render_set('managed_devices', 'ifname', devices, true),
		'',
		render_set('non_global_ipv4', 'ipv4_addr', NON_GLOBAL_IPV4, false),
		'',
		render_set('globally_reachable_special_ipv4', 'ipv4_addr', GLOBALLY_REACHABLE_SPECIAL_IPV4, false),
		'',
		render_set('non_global_ipv6', 'ipv6_addr', NON_GLOBAL_IPV6, false),
		'',
		render_set('globally_reachable_special_ipv6', 'ipv6_addr', GLOBALLY_REACHABLE_SPECIAL_IPV6, false),
		'',
		'	counter system_bypass {',
		`\t\tcomment "IANA non-global and local destination bypass; registry ${IANA_REGISTRY_DATE}"`,
		'	}',
		'',
		'	counter router_dns {',
		'		comment "Router DNS queries redirected to Steer"',
		'	}',
		'',
		'	counter router_ntp_direct {',
		'		comment "Router NTP startup traffic bypasses Steer"',
		'	}',
		'',
		'	counter smartdns_upstream {',
		'		comment "SmartDNS upstream packets exempted from local DNS redirect"',
		'	}',
		'',
		'	counter router_marked {',
		'		comment "Router packets selected for TPROXY"',
		'	}',
		'',
		'	counter router_tproxied {',
		'		comment "Router packets delivered to TPROXY"',
		'	}',
		'',
		'\tchain dns_prerouting {',
		'\t\ttype nat hook prerouting priority dstnat + 1; policy accept;',
		`\t\tiifname @managed_devices meta l4proto { tcp, udp } th dport 53 counter redirect to :${main.dns_port}`,
		'\t}'
	];

	if (main.router_proxy) {
		push(lines,
			'',
			'\tchain dns_output {',
			'\t\ttype nat hook output priority dstnat + 1; policy accept;',
			`\t\tmeta mark & ${hex32(mask)} == ${hex32(main.routing_mark & mask)} counter return`,
			`\t\tmeta mark & ${hex32(main.dns_upstream_mark)} == ${hex32(main.dns_upstream_mark)} counter name smartdns_upstream return`,
			`\t\tmeta l4proto { tcp, udp } th dport 53 counter name router_dns redirect to :${main.dns_port}`,
			'\t}'
		);
	}

	push(lines,
		'',
		'\tchain tproxy_prerouting {',
		'\t\ttype filter hook prerouting priority mangle; policy accept;'
	);

	if (main.router_proxy)
		push(lines,
			`\t\tiifname "lo" meta mark & ${hex32(mask)} == ${hex32(main.tproxy_mark & mask)} meta l4proto { tcp, udp } counter name router_tproxied tproxy to :${main.tproxy_port} accept`
		);

	push(lines,
		'\t\tiifname @managed_devices meta l4proto { tcp, udp } th dport 53 counter return',
		'\t\tiifname @managed_devices fib daddr type { local, broadcast, anycast, multicast } counter name system_bypass return',
		'\t\tiifname @managed_devices fib daddr oifname @managed_devices counter name system_bypass return',
		'\t\tiifname @managed_devices ct direction reply counter return',
		`\t\tiifname @managed_devices meta mark & ${hex32(mask)} == ${hex32(main.routing_mark & mask)} counter return`,
		'\t\tiifname @managed_devices ip daddr @globally_reachable_special_ipv4 goto tproxy_eligible',
		'\t\tiifname @managed_devices ip6 daddr @globally_reachable_special_ipv6 goto tproxy_eligible',
		'\t\tiifname @managed_devices ip daddr @non_global_ipv4 counter name system_bypass return',
		'\t\tiifname @managed_devices ip6 daddr @non_global_ipv6 counter name system_bypass return',
		'\t\tiifname @managed_devices goto tproxy_eligible',
		'\t}',
		'',
		'\tchain tproxy_eligible {',
		`\t\tmeta l4proto { tcp, udp } meta mark set meta mark & ${hex32(inverse_mask)} | ${hex32(main.tproxy_mark)} tproxy to :${main.tproxy_port} counter accept`,
		'\t}'
	);

	if (main.router_proxy) {
		push(lines,
			'',
			'\tchain tproxy_output {',
			'\t\ttype route hook output priority mangle; policy accept;',
			`\t\tmeta mark & ${hex32(mask)} == ${hex32(main.routing_mark & mask)} counter return`,
			'\t\tct direction reply counter return',
			'\t\tmeta l4proto udp udp dport 123 counter name router_ntp_direct return',
			`\t\tmeta mark & ${hex32(main.dns_upstream_mark)} != ${hex32(main.dns_upstream_mark)} meta l4proto { tcp, udp } th dport 53 counter return`,
			'\t\tfib daddr type { local, broadcast, anycast, multicast } counter name system_bypass return',
			'\t\tip daddr @globally_reachable_special_ipv4 goto mark_output',
			'\t\tip6 daddr @globally_reachable_special_ipv6 goto mark_output',
			'\t\tip daddr @non_global_ipv4 counter name system_bypass return',
			'\t\tip6 daddr @non_global_ipv6 counter name system_bypass return',
			'\t\tgoto mark_output',
			'\t}',
			'',
			'\tchain mark_output {',
			`\t\tmeta l4proto { tcp, udp } meta mark set meta mark & ${hex32(inverse_mask)} | ${hex32(main.tproxy_mark)} counter name router_marked`,
			'\t}'
		);
	}

	push(lines, '}', '');

	return { ok: true, devices, iana_registry_date: IANA_REGISTRY_DATE, config: join('\n', lines) };
};
