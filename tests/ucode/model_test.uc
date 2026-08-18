#!/usr/bin/ucode
/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

'use strict';

import {
	validate_model, compile_model, collect_source_mac_bindings
} from '../../steer/files/usr/share/steer/model.uc';
import { compile_firewall } from '../../steer/files/usr/share/steer/firewall.uc';

let failures = 0;

function fail(message) {
	warn(`not ok - ${message}\n`);
	failures++;
}

function pass(message) {
	print(`ok - ${message}\n`);
}

function expect(condition, message) {
	if (condition)
		pass(message);
	else
		fail(message);
}

function has_issue(result, code, severity) {
	for (let item in result[severity || 'errors']) {
		if (item.code == code)
			return true;
	}

	return false;
}

function find_by(items, option, value) {
	for (let item in items) {
		if (item[option] == value)
			return item;
	}
	return null;
}

function find_with(items, option) {
	for (let item in items) {
		if (item[option] != null)
			return item;
	}
	return null;
}

function find_group(rule, option) {
	if (rule == null)
		return null;
	if (rule[option] != null)
		return rule;
	for (let child in (rule.rules || [])) {
		const found = find_group(child, option);
		if (found != null)
			return found;
	}
	return null;
}

function find_action_rule(items, option, value) {
	for (let item in items) {
		if (item[option] == value)
			return item;
	}
	return null;
}

function find_by_inbound(items, inbound) {
	for (let item in items) {
		if (item.inbound == inbound)
			return item;
		for (let tag in (item.inbound || [])) {
			if (tag == inbound)
				return item;
		}
	}
	return null;
}

function find_route_by_inbound(items, inbound, outbound) {
	for (let item in items) {
		if (item.outbound != outbound)
			continue;
		if (item.inbound == inbound)
			return item;
		for (let tag in (item.inbound || [])) {
			if (tag == inbound)
				return item;
		}
	}
	return null;
}

function find_with_port(items, port) {
	for (let item in items) {
		const group = find_group(item, 'port');
		for (let value in (group?.port || [])) {
			if (value == port)
				return item;
		}
	}
	return null;
}

function base_model() {
	return {
		main: {
			id: 'main',
			schema_version: 3,
			enabled: false,
			router_proxy: true,
			managed_zone: [ 'lan' ],
			tproxy_port: 1042,
			dns_port: 1053,
			dns_upstream_mark: 256,
			routing_mark: 129,
			tproxy_mark: 128,
			mark_mask: 255,
			route_table: 80,
			rule_priority: 1024,
			log_level: 'warn'
		},
		bootstrap: {
			id: 'bootstrap',
			protocol: 'udp',
			server: '1.1.1.1',
			server_port: 53,
			strategy: 'prefer_ipv4'
		},
		nodes: [],
		outbounds: [
			{ id: 'direct', kind: 'direct', failure: 'block' },
			{ id: 'block', kind: 'block', failure: 'block' }
		],
		dns_servers: [
			{
				id: 'resolver',
				enabled: true,
				protocol: 'udp',
				server: '1.1.1.1',
				server_port: 53
			}
		],
		dns_profiles: [
			{
				id: 'direct_dns',
				enabled: true,
				listen_port: 6053,
				server: [ 'resolver' ],
				response_mode: 'fastest-ip',
				speed_check_mode: 'ping,tcp:443,tcp:80',
				cache_size: 32768,
				cache_persist: true,
				serve_expired: true,
				rr_ttl_min: 600,
				failure: 'block'
			}
		],
		local_proxies: [],
		rules: [
			{
				id: 'default',
				name: 'Default',
				enabled: true,
				default: true,
				domain_match: [],
				ip_match: [],
				source_ip_cidr: [],
				source_mac_address: [],
				network: [],
				protocol: [],
				port: [],
				dns_profile: 'direct_dns',
				outbound: 'direct'
			}
		]
	};
}

let model = base_model();
let result = validate_model(model);
expect(result.ok, '最小直连模型通过校验');

model = base_model();
model.outbounds[0].domain_match = [ 'geosite:geolocation-!cn' ];
model.outbounds[0].dns_profile = 'direct_dns';
result = validate_model(model);
expect(!result.ok && has_issue(result, 'UNKNOWN_OPTION'),
	'规则字段写入 outbound 时拒绝配置，不能静默忽略跨类型 ID 污染');

model = base_model();
model.dns_servers[0].id = 'direct';
result = validate_model(model);
expect(!result.ok && has_issue(result, 'GLOBAL_DUPLICATE_ID'),
	'所有实体共享同一 UCI ID 命名空间');

model = base_model();
model.main.schema_version = 1;
result = validate_model(model);
expect(!result.ok && has_issue(result, 'UNSUPPORTED_SCHEMA'),
	'移除内部 DNS SOCKS 后拒绝旧 schema，不静默忽略旧规则');

model = base_model();
let compiled = compile_model(model);
expect(compiled.ok, '最小直连模型可以编译');
expect(compiled.sing_box.route.final == 'steer-out-direct', 'Default 编译为显式最终出口');
expect(length(compiled.smartdns_instances) == 1, '一个 DNS Profile 编译为一个 SmartDNS 实例');
expect(index(compiled.smartdns_instances[0].config, 'force-qtype-SOA -\n') >= 0,
	'SmartDNS 实例明确清除 HTTPS/SVCB 类型抑制');
expect(index(compiled.smartdns_instances[0].config, '-bootstrap-dns -exclude-default-group') >= 0,
	'SmartDNS 实例使用独立且不参与业务查询的 Bootstrap DNS');
expect(index(compiled.smartdns_instances[0].config,
	'-bootstrap-dns -exclude-default-group -set-mark 129') >= 0,
	'SmartDNS Bootstrap 请求直接发出并携带核心绕行标记');
expect(index(compiled.smartdns_instances[0].config,
	'server 1.1.1.1:53 -set-mark 256') >= 0,
	'SmartDNS 业务上游只携带防回环 mark');
expect(compiled.sing_box.route.default_domain_resolver.server == 'steer-dns-bootstrap',
	'sing-box 节点域名使用显式 Bootstrap DNS');
expect(compiled.sing_box.dns.servers[0].routing_mark == 129,
	'sing-box Bootstrap DNS 携带核心绕行标记');
expect(find_by(compiled.sing_box.inbounds, 'tag', 'steer-dns-upstream') == null &&
	index(compiled.smartdns_instances[0].config, 'proxy-server ') < 0,
	'Steer 不生成内部 DNS SOCKS');

model = base_model();
let geo_default_rule = model.rules[0];
model.rules = [ {
	id: 'geo_rule', name: 'Geo rule', enabled: true, default: false,
	inbound: [],
	domain_match: [
		'full:www.example.com', 'domain:example.com', 'crypto',
		'regexp:^api[0-9]+[.]example[.]com$', 'geosite:category-example'
	],
	ip_match: [ '192.0.2.0/24', 'geoip:example' ],
	source_ip_cidr: [], network: [ 'udp' ], protocol: [], port: [ '443' ],
	dns_profile: 'direct_dns', outbound: 'direct'
}, geo_default_rule ];
result = validate_model(model);
expect(result.ok && !has_issue(result, 'DNS_PROJECTION_EMPTY', 'warnings'),
	'规则中直接填写 GeoSite 构成 DNS 可观察匹配');
compiled = compile_model(model);
let geo_route = find_action_rule(compiled.sing_box.route.rules, 'outbound', 'steer-out-direct');
let geo_dns = find_action_rule(compiled.sing_box.dns.rules, 'server', 'steer-dns-direct_dns');
let geo_route_domain = find_group(geo_route, 'domain_suffix');
let geo_route_ip = find_group(geo_route, 'ip_cidr');
let geo_dns_domain = find_group(geo_dns, 'domain_suffix');
let geo_route_site = find_group(geo_route.rules[0], 'rule_set');
let geo_route_geoip = find_group(geo_route.rules[1], 'rule_set');
let geo_dns_site = find_group(geo_dns, 'rule_set');
expect(length(compiled.sing_box.route.rule_set) == 2 &&
	compiled.sing_box.route.rule_set[0].path == '/var/lib/steer/geodata/current/rules/geosite-category-example.srs',
	'规则中的 GeoSite/GeoIP 分类自动编译为受控本地二进制规则集');
expect(geo_route.type == 'logical' && geo_route.mode == 'and' && length(geo_route.rules) == 4 &&
	geo_route.rules[0].type == 'logical' && geo_route.rules[0].mode == 'or' &&
	length(geo_route.rules[0].rules) == 5 &&
	geo_route.rules[1].type == 'logical' && geo_route.rules[1].mode == 'or' &&
	geo_route_domain.domain_suffix[0] == 'example.com' && geo_route_ip.ip_cidr[0] == '192.0.2.0/24' &&
	find_group(geo_route, 'domain').domain[0] == 'www.example.com' &&
	find_group(geo_route, 'domain_keyword').domain_keyword[0] == 'crypto' &&
	find_group(geo_route, 'domain_regex').domain_regex[0] == '^api[0-9]+[.]example[.]com$' &&
	geo_route_site.rule_set[0] == 'steer-geosite-category-example' &&
	geo_route_geoip.rule_set[0] == 'steer-geoip-example' &&
	find_group(geo_route, 'network').network[0] == 'udp' && find_group(geo_route, 'port').port[0] == 443,
	'Route 用显式 logical AND 连接字段组，并保持域名组与目的 IP 组内部 OR');
expect(geo_dns.mode == 'or' && geo_dns_domain != null &&
	geo_dns_site.rule_set[0] == 'steer-geosite-category-example' &&
	geo_dns_site.rule_set[1] == null && find_group(geo_dns, 'ip_cidr') == null,
	'DNS 投影只引用 GeoSite，不把查询前不可观察的 GeoIP 假装成 DNS 条件');

model = base_model();
model.rules[0].default = false;
model.rules[0].domain_match = [ 'geosite:../cn' ];
result = validate_model(model);
expect(has_issue(result, 'INVALID_GEO_CATEGORY'),
	'规则内的 Geo 分类不能通过路径字符逃逸受控生成目录');

model = base_model();
model.rules[0].default = false;
model.rules[0].domain_match = [ 'full:-invalid.example', 'geossite:cn', 'regexp:[' ];
result = validate_model(model);
expect(!result.ok && has_issue(result, 'INVALID_DOMAIN_MATCH'),
	'合并域名字段拒绝非法域名、拼错的保留前缀和错误正则');

model = base_model();
model.rules[0].default = false;
model.rules[0].ip_match = [ 'geoip:../cn' ];
result = validate_model(model);
expect(!result.ok && has_issue(result, 'INVALID_GEO_CATEGORY'),
	'合并目的 IP 字段对 GeoIP 分类执行相同的路径边界校验');

model = base_model();
model.rules = [ {
	id: 'device_rule', name: 'Device rule', enabled: true, default: false,
	inbound: [], domain_match: [], ip_match: [],
	source_ip_cidr: [], source_mac_address: [ '02:00:00:00:00:10' ],
	network: [], protocol: [], port: [],
	dns_profile: 'direct_dns', outbound: 'direct'
}, model.rules[0] ];
result = validate_model(model);
expect(result.ok && !has_issue(result, 'DNS_PROJECTION_EMPTY', 'warnings'),
	'客户端 MAC 是 DNS 与 Route 都可观察的普通规则字段');
let mac_bindings = collect_source_mac_bindings(model);
expect(length(mac_bindings) == 1 && mac_bindings[0].address == '02:00:00:00:00:10' &&
	mac_bindings[0].tproxy_port == 49152 && mac_bindings[0].dns_port == 49153,
	'MAC 分类入口按规范化地址稳定分配独立端口');
compiled = compile_model(model);
let mac_route = find_route_by_inbound(
	compiled.sing_box.route.rules, mac_bindings[0].tproxy_tag, 'steer-out-direct');
let mac_dns = find_by_inbound(compiled.sing_box.dns.rules, mac_bindings[0].dns_tag);
expect(mac_route != null && find_group(mac_route, 'source_mac_address') == null,
	'sing-box 1.13 后端把 source_mac_address 降级为隐藏 TPROXY inbound');
expect(mac_dns != null && find_group(mac_dns, 'inbound').inbound[0] == mac_bindings[0].tproxy_tag &&
	find_group(mac_dns, 'inbound').inbound[1] == mac_bindings[0].dns_tag,
	'MAC 规则的 DNS 投影同时保留连接解析和 53 端口查询上下文');
expect(find_by(compiled.sing_box.inbounds, 'tag', mac_bindings[0].tproxy_tag).listen == '::' &&
	find_by(compiled.sing_box.inbounds, 'tag', mac_bindings[0].dns_tag).listen == '::',
	'MAC 分类的隐藏入口同时监听 IPv4 与 IPv6');
let native_mac_compiled = compile_model(model, { source_mac_backend: 'sing-box-1.14-native' });
let native_mac_route = find_action_rule(
	native_mac_compiled.sing_box.route.rules, 'outbound', 'steer-out-direct');
expect(native_mac_compiled.ok && length(native_mac_compiled.source_mac_bindings) == 0 &&
	find_group(native_mac_route, 'source_mac_address').source_mac_address[0] ==
		'02:00:00:00:00:10',
	'预留的 sing-box 1.14 后端直接生成同名原生 source_mac_address 字段');

let mac_firewall = compile_firewall(model.main, [ 'br-lan' ], 'steer', mac_bindings);
expect(mac_firewall.ok && index(mac_firewall.config,
	'ether saddr 02:00:00:00:00:10 meta l4proto { tcp, udp } th dport 53 counter redirect to :49153') >= 0 &&
	index(mac_firewall.config,
	'ether saddr 02:00:00:00:00:10 meta l4proto { tcp, udp } meta mark set') >= 0 &&
	index(mac_firewall.config, 'tproxy to :49152') >= 0,
	'nftables 在协议族判断前用同一 ether saddr 规则分类双栈 DNS 与业务流量');

model = base_model();
model.rules = [ {
	id: 'bad_mac', enabled: true, default: false,
	inbound: [], domain_match: [], ip_match: [], source_ip_cidr: [],
	source_mac_address: [ '02:00:00:00:00' ], network: [], protocol: [], port: [],
	dns_profile: 'direct_dns', outbound: 'direct'
}, model.rules[0] ];
result = validate_model(model);
expect(!result.ok && has_issue(result, 'INVALID_SOURCE_MAC_ADDRESS'),
	'非法客户端 MAC 地址阻止应用');

model = base_model();
push(model.local_proxies, {
	id: 'local_entry', enabled: true, protocol: 'mixed',
	listen: '127.0.0.1', listen_port: 1090
});
model.rules = [ {
	id: 'conflicting_sources', enabled: true, default: false,
	inbound: [ 'local_entry' ], domain_match: [], ip_match: [], source_ip_cidr: [],
	source_mac_address: [ '02:00:00:00:00:10' ], network: [], protocol: [], port: [],
	dns_profile: 'direct_dns', outbound: 'direct'
}, model.rules[0] ];
result = validate_model(model);
expect(!result.ok && has_issue(result, 'INCOMPATIBLE_SOURCE_SELECTORS'),
	'客户端 MAC 与本地代理 inbound 的不可能交集会被明确拒绝');

model = base_model();
model.rules = [ {
	id: 'ipv6_validation', enabled: true, default: false,
	inbound: [], domain_match: [], ip_match: [],
	source_ip_cidr: [ '2001:db8::10/128', '::::/128' ], source_mac_address: [],
	network: [], protocol: [], port: [], dns_profile: 'direct_dns', outbound: 'direct'
}, model.rules[0] ];
result = validate_model(model);
expect(!result.ok && has_issue(result, 'INVALID_IP_CIDR'),
	'压缩 IPv6 可用但非法冒号序列不会再通过字面量校验');

let firewall = compile_firewall(model.main, [ 'br-lan', 'br-guest', 'br-lan' ]);
expect(firewall.ok && length(firewall.devices) == 2,
	'firewall 编译器去重受管 zone 解析出的实际设备');
expect(index(firewall.config, 'iifname @managed_devices meta l4proto { tcp, udp } th dport 53') >= 0,
	'DNS 接管按 ingress 设备匹配并同时覆盖 TCP 与 UDP');
expect(index(firewall.config, 'ip6 saddr') < 0,
	'透明接管不依赖 IPv6 源地址范围判断');
expect(index(firewall.config, 'tproxy to :1042') >= 0,
	'TCP 与 UDP 使用同一双栈 TPROXY 入站');
expect(index(firewall.config, 'type route hook output priority mangle') >= 0 &&
	index(firewall.config, 'iifname "lo" meta mark & 0x000000ff == 0x00000080') >= 0,
	'启用本机代理时 output 流量经策略路由回送同一 TPROXY');
expect(index(firewall.config,
	'meta l4proto udp udp dport 123 counter name router_ntp_direct return') >= 0 &&
	index(firewall.config, 'router_ntp_direct return') < index(firewall.config, 'goto mark_output'),
	'路由器本机 UDP/123 在普通规则标记前固定直连');
expect(index(firewall.config, 'ct direction reply counter return') >= 0 &&
	index(firewall.config, 'ct direction reply counter return') < index(firewall.config, 'goto mark_output'),
	'路由器本机服务的 conntrack reply 不会被误当作新连接代理');
expect(index(firewall.config, 'type nat hook output priority dstnat + 1') >= 0 &&
	index(firewall.config, 'th dport 53 counter name router_dns redirect to :1053') >= 0,
	'本机传统 DNS 在保留地址旁路前进入 Steer DNS');
expect(index(firewall.config,
	'meta mark & 0x00000100 == 0x00000100 counter name smartdns_upstream return') >= 0 &&
	index(firewall.config,
	'meta mark & 0x00000100 != 0x00000100 meta l4proto { tcp, udp } th dport 53 counter return') >= 0,
	'SmartDNS 上游 mark 只跳过本机 53 劫持，不跳过 output TPROXY');
expect(index(firewall.config, '192.0.0.9/32') >= 0 &&
	index(firewall.config, '2001:1::1/128') >= 0,
	'IANA 特殊用途范围中的全局可达例外仍进入普通 Rules');
model.main.router_proxy = false;
firewall = compile_firewall(model.main, [ 'br-lan' ]);
expect(index(firewall.config, 'hook output') < 0 && index(firewall.config, 'iifname "lo"') < 0,
	'关闭本机代理时同时移除本机业务和 DNS 接管');
expect(!compile_firewall(model.main, []).ok,
	'受管 zone 没有实际设备时拒绝生成空接管规则');

model = base_model();
model.main.routing_mark = 128;
result = validate_model(model);
expect(!result.ok && has_issue(result, 'MARK_COLLISION'),
	'sing-box 出站绕行标记与 TPROXY 标记冲突时拒绝应用');

model = base_model();
model.main.dns_upstream_mark = 128;
result = validate_model(model);
expect(!result.ok && has_issue(result, 'DNS_UPSTREAM_MARK_COLLISION'),
	'SmartDNS 上游防回环 mark 不能落入 TPROXY mark mask');

model = base_model();
model.main.log_level = 'warn\nserver 127.0.0.1';
result = validate_model(model);
expect(!result.ok && has_issue(result, 'INVALID_LOG_LEVEL'),
	'可能注入 SmartDNS 指令的日志级别会被拒绝');

model = base_model();
model.dns_servers[0].protocol = 'tls';
model.dns_servers[0].tls_server_name = 'dns.example\nserver 127.0.0.1';
result = validate_model(model);
expect(!result.ok && has_issue(result, 'INVALID_TLS_SERVER_NAME'),
	'可能注入 SmartDNS 指令的 TLS server name 会被拒绝');

model = base_model();
push(model.rules, {
	id: 'dangling_reference',
	name: 'Dangling reference',
	enabled: true,
	default: false,
	domain_match: [ 'domain:service.example' ],
	ip_match: [],
	source_ip_cidr: [],
	network: [],
	protocol: [],
	port: [],
	dns_profile: 'direct_dns',
	outbound: 'missing_subscription_node'
});
result = validate_model(model);
expect(!result.ok && has_issue(result, 'DANGLING_OUTBOUND'), '启用规则的悬空出口阻止应用');

model = base_model();
model.rules = [
	{
		id: 'disabled_example',
		name: 'Disabled example',
		enabled: false,
		default: false,
		domain_match: [ 'domain:ads.example' ],
		ip_match: [],
		source_ip_cidr: [],
		network: [],
		protocol: [],
		port: [],
		dns_profile: 'missing_dns',
		outbound: 'missing_outbound'
	},
	model.rules[0]
];
result = validate_model(model);
expect(result.ok && has_issue(result, 'DANGLING_OUTBOUND', 'warnings'),
	'禁用规则保留悬空引用但只产生警告');
compiled = compile_model(model);
expect(length(compiled.sing_box.route.rules) == 2, '禁用规则不进入 sing-box 路由规则');

model = base_model();
model.main.enabled = true;
model.main.managed_zone = [];
result = validate_model(model);
expect(!result.ok && has_issue(result, 'NO_MANAGED_ZONE'), '启用时没有受管 zone 会被拒绝');

model = base_model();
model.main.managed_zone = [ 'lan zone' ];
result = validate_model(model);
expect(!result.ok && has_issue(result, 'INVALID_MANAGED_ZONE'),
	'不能安全解析为 firewall zone 的名称会被拒绝');

model = base_model();
const default_rule = model.rules[0];
model.rules = [
	default_rule,
	{
		id: 'late',
		name: 'Late rule',
		enabled: true,
		default: false,
		domain_match: [ 'full:example.com' ],
		ip_match: [],
		source_ip_cidr: [],
		network: [],
		protocol: [],
		port: [],
		dns_profile: 'direct_dns',
		outbound: 'direct'
	}
];
result = validate_model(model);
expect(!result.ok && has_issue(result, 'RULE_AFTER_DEFAULT'), 'Default 后不能出现其他启用规则');

model = base_model();
model.rules = [
	{
		id: 'port_only',
		name: 'Port only',
		enabled: true,
		default: false,
		domain_match: [],
		ip_match: [],
		source_ip_cidr: [],
		network: [],
		protocol: [],
		port: [ '443' ],
		dns_profile: 'direct_dns',
		outbound: 'direct'
	},
	model.rules[0]
];
result = validate_model(model);
expect(result.ok && has_issue(result, 'DNS_PROJECTION_EMPTY', 'warnings'),
	'只有连接阶段条件的规则明确提示 DNS Profile 不参与匹配');
compiled = compile_model(model);
expect(find_with_port(compiled.sing_box.route.rules, 443) != null,
	'只有连接阶段条件的规则仍进入路由投影');
expect(compiled.sing_box.route.rules[2].port[0] == 443,
	'连接阶段规则按普通 Rules 顺序编译');
expect(length(compiled.sing_box.dns.rules || []) == 0,
	'连接阶段规则不生成空 DNS 规则');

model = base_model();
model.rules = [
	{
		id: 'mixed',
		name: 'Mixed rule',
		enabled: true,
		default: false,
		domain_match: [ 'domain:example.com' ],
		ip_match: [],
		source_ip_cidr: [],
		network: [ 'udp' ],
		protocol: [],
		port: [ '443' ],
		dns_profile: 'direct_dns',
		outbound: 'direct'
	},
	model.rules[0]
];
result = validate_model(model);
expect(result.ok, '域名与连接条件可以共存于一条混合规则');
compiled = compile_model(model);
let mixed_route_rule = find_action_rule(compiled.sing_box.route.rules, 'outbound', 'steer-out-direct');
expect(mixed_route_rule.type == 'logical' && mixed_route_rule.mode == 'and' &&
	find_group(mixed_route_rule, 'domain_suffix').domain_suffix[0] == 'example.com' &&
	find_group(mixed_route_rule, 'network').network[0] == 'udp' &&
	find_group(mixed_route_rule, 'port').port[0] == 443,
	'路由投影用 AND 保留混合规则的全部字段条件');
let mixed_dns_rule = find_action_rule(compiled.sing_box.dns.rules, 'server', 'steer-dns-direct_dns');
expect(mixed_dns_rule.domain_suffix[0] == 'example.com' &&
	mixed_dns_rule.network == null && mixed_dns_rule.port == null && mixed_dns_rule.type == null,
	'DNS 投影只保留查询阶段可观察的条件');

model = base_model();
model.rules = [ {
	id: 'bad_port', enabled: true, default: false,
	inbound: [], domain_match: [], ip_match: [],
	source_ip_cidr: [], network: [], protocol: [], port: [ '443:8443' ],
	dns_profile: 'direct_dns', outbound: 'direct'
}, model.rules[0] ];
result = validate_model(model);
expect(!result.ok && has_issue(result, 'INVALID_RULE_PORT'),
	'端口范围不能被静默截断为单个端口');

model = base_model();
push(model.local_proxies, {
	id: 'developer_proxy',
	name: 'Developer proxy',
	enabled: true,
	protocol: 'mixed',
	listen: '127.0.0.1',
	listen_port: 1090
});
model.rules = [
	{
		id: 'developer_proxy_traffic',
		name: 'Developer proxy',
		enabled: true,
		default: false,
		inbound: [ 'developer_proxy' ],
		domain_match: [], ip_match: [],
		source_ip_cidr: [], network: [], protocol: [], port: [],
		dns_profile: 'direct_dns',
		outbound: 'direct'
	},
	model.rules[0]
];
result = validate_model(model);
expect(result.ok && !has_issue(result, 'DNS_PROJECTION_EMPTY', 'warnings'),
	'本地代理入口是 DNS 与 Route 都可观察的规则维度');
compiled = compile_model(model);
expect(find_by(compiled.sing_box.inbounds, 'tag', 'steer-local-developer_proxy') != null,
	'具名 mixed 本地代理入口进入 sing-box 配置');
expect(find_by_inbound(compiled.sing_box.dns.rules, 'steer-local-developer_proxy').server == 'steer-dns-direct_dns' &&
	find_route_by_inbound(compiled.sing_box.route.rules,
		'steer-local-developer_proxy', 'steer-out-direct') != null,
	'规则用同一个入口标签选择 DNS Profile 和逻辑出口');

model = base_model();
push(model.local_proxies, {
	id: 'lan_proxy', enabled: true, protocol: 'socks',
	listen: '0.0.0.0', listen_port: 1090
});
result = validate_model(model);
expect(!result.ok && has_issue(result, 'LOCAL_PROXY_AUTH_REQUIRED'),
	'非回环本地代理入口没有认证时被拒绝');

model = base_model();
model.rules = [
	{
		id: 'removed_proxy', enabled: true, default: false,
		inbound: [ 'removed_proxy' ], domain_match: [], ip_match: [],
		source_ip_cidr: [], network: [], protocol: [], port: [],
		dns_profile: 'direct_dns', outbound: 'direct'
	},
	model.rules[0]
];
result = validate_model(model);
expect(!result.ok && has_issue(result, 'DANGLING_LOCAL_PROXY'),
	'规则引用不存在的本地代理入口时被拒绝');

model = base_model();
push(model.nodes, {
	id: 'proxy_node',
	enabled: true,
	type: 'trojan',
	server: 'proxy.example.com',
	server_port: 443,
		password: 'fixture-password',
	tls_server_name: 'proxy.example.com',
	insecure: false
});
push(model.outbounds, {
	id: 'proxy',
	kind: 'single',
	node: 'proxy_node',
	failure: 'block'
});
push(model.dns_servers, {
		id: 'resolver_b',
	enabled: true,
	protocol: 'https',
	server: '1.1.1.1',
	server_port: 443,
	path: '/dns-query',
		tls_server_name: 'one.one.one.one',
	insecure: false
});
push(model.dns_profiles, {
	id: 'proxy_dns',
	enabled: true,
	listen_port: 6553,
		server: [ 'resolver_b' ],
	response_mode: 'fastest-response',
	speed_check_mode: 'none',
	cache_size: 32768,
	cache_persist: true,
	serve_expired: true,
	rr_ttl_min: 600,
	failure: 'block'
});
model.rules = [ {
	id: 'resolver_proxy', enabled: true, default: false,
	inbound: [], domain_match: [], ip_match: [ '1.1.1.1/32' ],
	source_ip_cidr: [], network: [], protocol: [], port: [ '443' ],
	dns_profile: 'direct_dns', outbound: 'proxy'
}, model.rules[0] ];
result = validate_model(model);
expect(result.ok, 'SmartDNS 上游目标可以由普通规则选择代理出口');
compiled = compile_model(model);
expect(length(compiled.smartdns_instances) == 2, '两个 DNS Profile 编译为两个 SmartDNS 实例');
expect(index(compiled.smartdns_instances[1].config, '-set-mark 256') >= 0 &&
	index(compiled.smartdns_instances[1].config, '-proxy ') < 0,
	'多个 DNS Profile 共用防回环 mark 而不生成代理入口');
let resolver_route = find_with_port(compiled.sing_box.route.rules, 443);
expect(resolver_route != null && resolver_route.outbound == 'steer-out-proxy',
	'SmartDNS 上游连接与其他本机流量共用普通 Rules');

model = base_model();
push(model.nodes, {
	id: 'hy2_hopping',
	enabled: true,
	type: 'hysteria2',
	server: 'hy2.example.com',
	server_port: 443,
		server_ports: [ '12000:12010', '13000' ],
		password: 'fixture-password',
	tls_server_name: 'hy2.example.com',
	insecure: false
});
push(model.outbounds, {
	id: 'hy2_route',
	kind: 'single',
	node: 'hy2_hopping',
	failure: 'block'
});
result = validate_model(model);
expect(result.ok, 'Hysteria2 端口跳跃节点通过校验');
compiled = compile_model(model);
let hy2_outbound = find_by(compiled.sing_box.outbounds, 'tag', 'steer-node-hy2_hopping');
expect(hy2_outbound != null && hy2_outbound.server_port == null &&
		length(hy2_outbound.server_ports) == 2 && hy2_outbound.server_ports[0] == '12000:12010',
	'Hysteria2 server_ports 启用时不生成冲突的 server_port');

if (failures > 0) {
	warn(`${failures} test(s) failed\n`);
	exit(1);
}

print('all model tests passed\n');
