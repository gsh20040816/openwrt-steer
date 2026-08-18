/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 *
 * Steer semantic model, validation and deterministic sing-box compilation.
 */

'use strict';

import { cursor } from 'uci';

const SCHEMA_VERSION = 3;

const SECTION_OPTIONS = {
	steer: [
		'schema_version', 'enabled', 'router_proxy', 'managed_zone',
		'tproxy_port', 'dns_port', 'dns_upstream_mark', 'routing_mark',
		'tproxy_mark', 'mark_mask', 'route_table', 'rule_priority', 'log_level'
	],
	bootstrap: [ 'protocol', 'server', 'server_port', 'strategy' ],
	node: [
		'enabled', 'name', 'type', 'server', 'server_port', 'uuid', 'flow',
		'packet_encoding', 'password', 'server_ports', 'hop_interval',
		'obfs_type', 'obfs_password', 'up_mbps', 'down_mbps',
		'tls_server_name', 'insecure', 'reality_public_key',
		'reality_short_id', 'utls_fingerprint'
	],
	outbound: [ 'name', 'kind', 'node', 'failure' ],
	dns_server: [
		'enabled', 'name', 'protocol', 'server', 'server_port',
		'tls_server_name', 'path', 'insecure'
	],
	dns_profile: [
		'enabled', 'name', 'listen_port', 'server', 'response_mode',
		'speed_check_mode', 'failure', 'cache_size', 'cache_persist',
		'serve_expired', 'rr_ttl_min'
	],
	local_proxy: [
		'enabled', 'name', 'protocol', 'listen', 'listen_port', 'username', 'password'
	],
	rule: [
		'enabled', 'default', 'name', 'dns_profile', 'outbound', 'inbound',
		'domain_match', 'ip_match', 'source_ip_cidr', 'network', 'protocol', 'port'
	]
};

function as_array(value) {
	if (value == null)
		return [];

	return type(value) == 'array' ? value : [ value ];
}

function as_bool(value) {
	return value === true || value == '1';
}

function as_int(value) {
	if (value == null || value == '')
		return null;

	return int(value);
}

function has_text(value) {
	return type(value) == 'string' && length(value) > 0;
}

function copy_section(section) {
	let result = {};

	for (let key in keys(section)) {
		if (substr(key, 0, 1) != '.')
			result[key] = section[key];
	}

	result.id = section['.name'];
	return result;
}

function normalize_section(section, section_type) {
	let result = copy_section(section);

	if (section_type == 'steer') {
		result.schema_version = as_int(result.schema_version);
		result.enabled = as_bool(result.enabled);
		result.router_proxy = result.router_proxy == null ? true : as_bool(result.router_proxy);
		result.managed_zone = as_array(result.managed_zone);
		result.tproxy_port = as_int(result.tproxy_port);
		result.dns_port = as_int(result.dns_port);
		result.dns_upstream_mark = as_int(result.dns_upstream_mark);
		result.routing_mark = as_int(result.routing_mark);
		result.tproxy_mark = as_int(result.tproxy_mark);
		result.mark_mask = as_int(result.mark_mask);
		result.route_table = as_int(result.route_table);
		result.rule_priority = as_int(result.rule_priority);
	}
	else if (section_type == 'node') {
		result.enabled = result.enabled == null ? true : as_bool(result.enabled);
		result.server_port = as_int(result.server_port);
		result.up_mbps = as_int(result.up_mbps);
		result.down_mbps = as_int(result.down_mbps);
		result.insecure = as_bool(result.insecure);
		result.server_ports = as_array(result.server_ports);
	}
	else if (section_type == 'bootstrap') {
		result.server_port = as_int(result.server_port);
	}
	else if (section_type == 'dns_server') {
		result.enabled = result.enabled == null ? true : as_bool(result.enabled);
		result.server_port = as_int(result.server_port);
		result.insecure = as_bool(result.insecure);
	}
	else if (section_type == 'dns_profile') {
		result.enabled = result.enabled == null ? true : as_bool(result.enabled);
		result.listen_port = as_int(result.listen_port);
		result.server = as_array(result.server);
		result.cache_size = as_int(result.cache_size);
		result.cache_persist = as_bool(result.cache_persist);
		result.serve_expired = as_bool(result.serve_expired);
		result.rr_ttl_min = as_int(result.rr_ttl_min);
	}
	else if (section_type == 'local_proxy') {
		result.enabled = result.enabled == null ? true : as_bool(result.enabled);
		result.listen_port = as_int(result.listen_port);
	}
	else if (section_type == 'rule') {
		result.enabled = result.enabled == null ? true : as_bool(result.enabled);
		result.default = as_bool(result.default);
		for (let option in [
			'inbound', 'domain_match', 'ip_match', 'source_ip_cidr',
			'network', 'protocol', 'port'
		])
			result[option] = as_array(result[option]);
	}

	return result;
}

export function load_uci_model(package_name) {
	package_name ??= 'steer';

	const uci = cursor();
	uci.load(package_name);

	let model = {
		main: null,
		bootstrap: null,
		nodes: [],
		outbounds: [],
		dns_servers: [],
		dns_profiles: [],
		local_proxies: [],
		rules: []
	};

	uci.foreach(package_name, 'steer', (section) => {
		if (model.main == null)
			model.main = normalize_section(section, 'steer');
		else
			push(model.rules, { id: section['.name'], __duplicate_main: true });
	});
	uci.foreach(package_name, 'bootstrap', (section) => {
		if (model.bootstrap == null)
			model.bootstrap = normalize_section(section, 'bootstrap');
		else
			push(model.rules, { id: section['.name'], __duplicate_bootstrap: true });
	});
	uci.foreach(package_name, 'node', (section) =>
		push(model.nodes, normalize_section(section, 'node')));
	uci.foreach(package_name, 'outbound', (section) =>
		push(model.outbounds, normalize_section(section, 'outbound')));
	uci.foreach(package_name, 'dns_server', (section) =>
		push(model.dns_servers, normalize_section(section, 'dns_server')));
	uci.foreach(package_name, 'dns_profile', (section) =>
		push(model.dns_profiles, normalize_section(section, 'dns_profile')));
	uci.foreach(package_name, 'local_proxy', (section) =>
		push(model.local_proxies, normalize_section(section, 'local_proxy')));
	let default_rules = [];
	uci.foreach(package_name, 'rule', (section) => {
		const rule = normalize_section(section, 'rule');
		if (rule.default)
			push(default_rules, rule);
		else
			push(model.rules, rule);
	});
	for (let rule in default_rules)
		push(model.rules, rule);

	return model;
};

function issue(result, severity, code, object_type, object_id, option, message) {
	push(result[severity], {
		code,
		object_type,
		object_id,
		option,
		message
	});
}

function error(result, code, object_type, object_id, option, message) {
	issue(result, 'errors', code, object_type, object_id, option, message);
}

function warning(result, code, object_type, object_id, option, message) {
	issue(result, 'warnings', code, object_type, object_id, option, message);
}

function valid_id(id) {
	return has_text(id) && match(id, /^[a-z][a-z0-9_\-]{0,31}$/) != null;
}

function index_objects(result, objects, object_type) {
	let index = {};

	for (let object in objects) {
		if (!valid_id(object.id)) {
			error(result, 'INVALID_ID', object_type, object.id, 'id',
				'标识必须以小写字母开头，且只能包含小写字母、数字、下划线和连字符');
			continue;
		}

		if (index[object.id] != null) {
			error(result, 'DUPLICATE_ID', object_type, object.id, 'id', '同类对象标识重复');
			continue;
		}

		index[object.id] = object;
	}

	return index;
}

function validate_known_options(result, object_type, object) {
	if (object == null)
		return;

	let allowed = {};
	for (let option in (SECTION_OPTIONS[object_type] || []))
		allowed[option] = true;

	for (let option in keys(object)) {
		if (option == 'id' || substr(option, 0, 2) == '__')
			continue;
		if (allowed[option] == null)
			error(result, 'UNKNOWN_OPTION', object_type, object.id, option,
				`字段不属于 ${object_type}，可能由跨类型 ID 冲突或旧配置写入`);
	}
}

function validate_global_ids(result, model) {
	let seen = {};
	let objects_by_type = {
		steer: model.main == null ? [] : [ model.main ],
		bootstrap: model.bootstrap == null ? [] : [ model.bootstrap ],
		node: model.nodes || [],
		outbound: model.outbounds || [],
		dns_server: model.dns_servers || [],
		dns_profile: model.dns_profiles || [],
		local_proxy: model.local_proxies || [],
		rule: model.rules || []
	};

	for (let object_type in keys(objects_by_type)) {
		for (let object in objects_by_type[object_type]) {
			if (!has_text(object?.id))
				continue;
			if (seen[object.id] != null)
				error(result, 'GLOBAL_DUPLICATE_ID', object_type, object.id, 'id',
					`标识已被 ${seen[object.id]} 使用；所有实体必须共享同一 ID 命名空间`);
			else
				seen[object.id] = object_type;
		}
	}
}

function require_text(result, object_type, object, option) {
	if (!has_text(object[option]))
		error(result, 'REQUIRED', object_type, object.id, option, '该字段不能为空');
}

function validate_port(result, object_type, object, option) {
	const value = object[option];

	if (value == null || value < 1 || value > 65535)
		error(result, 'INVALID_PORT', object_type, object.id, option, '端口必须在 1 到 65535 之间');
}

function validate_integer_range(result, object_type, object, option, minimum, maximum) {
	const value = object[option];

	if (value == null || value < minimum || value > maximum)
		error(result, 'INVALID_INTEGER', object_type, object.id, option,
			`该字段必须是 ${minimum} 到 ${maximum} 之间的整数`);
}

function validate_optional_integer_range(result, object_type, object, option, minimum, maximum) {
	if (object[option] != null)
		validate_integer_range(result, object_type, object, option, minimum, maximum);
}

function validate_hysteria2_port_range(result, node, value) {
	if (!has_text(value) || match(value, /^[0-9]+(:[0-9]+)?$/) == null) {
		error(result, 'INVALID_PORT_RANGE', 'node', node.id, 'server_ports',
			`Hysteria2 端口或端口范围无效：${value}`);
		return;
	}

	const parts = split(value, ':');
	const first = int(parts[0]);
	const last = int(parts[1] ?? parts[0]);
	if (first < 1 || first > 65535 || last < 1 || last > 65535 || first > last)
		error(result, 'INVALID_PORT_RANGE', 'node', node.id, 'server_ports',
			`Hysteria2 端口或端口范围无效：${value}`);
}

function validate_node(result, node) {
	if (!node.enabled)
		return;

	if (!(node.type in [ 'vless', 'hysteria2', 'trojan' ])) {
		error(result, 'UNSUPPORTED_NODE_TYPE', 'node', node.id, 'type',
			'第一版只支持 vless、hysteria2 和 trojan');
		return;
	}

	require_text(result, 'node', node, 'server');
	validate_port(result, 'node', node, 'server_port');

	if (node.type == 'vless') {
		require_text(result, 'node', node, 'uuid');
		if (has_text(node.flow) && node.flow != 'xtls-rprx-vision')
			error(result, 'UNSUPPORTED_VLESS_FLOW', 'node', node.id, 'flow',
				'第一版只支持空 flow 或 xtls-rprx-vision');

		if (has_text(node.reality_public_key)) {
			require_text(result, 'node', node, 'tls_server_name');
			require_text(result, 'node', node, 'reality_short_id');
		}
		if (has_text(node.packet_encoding) && !(node.packet_encoding in [ 'xudp', 'packetaddr' ]))
			error(result, 'INVALID_PACKET_ENCODING', 'node', node.id, 'packet_encoding',
				'VLESS UDP packet encoding 只支持 xudp 或 packetaddr');
	}
	else if (node.type == 'hysteria2') {
		require_text(result, 'node', node, 'password');
		require_text(result, 'node', node, 'tls_server_name');
		if (has_text(node.obfs_type) && node.obfs_type != 'salamander')
			error(result, 'UNSUPPORTED_HYSTERIA2_OBFS', 'node', node.id, 'obfs_type',
				'sing-box 1.13 基线只接受 salamander 混淆');
		if (has_text(node.obfs_type))
			require_text(result, 'node', node, 'obfs_password');
		for (let value in (node.server_ports || []))
			validate_hysteria2_port_range(result, node, value);
		if (has_text(node.hop_interval) && match(node.hop_interval, /^[1-9][0-9]*(ms|s|m|h)$/) == null)
			error(result, 'INVALID_DURATION', 'node', node.id, 'hop_interval',
				'Hysteria2 端口跳跃间隔必须是正整数加 ms、s、m 或 h');
		validate_optional_integer_range(result, 'node', node, 'up_mbps', 1, 1000000);
		validate_optional_integer_range(result, 'node', node, 'down_mbps', 1, 1000000);
	}
	else if (node.type == 'trojan') {
		require_text(result, 'node', node, 'password');
		require_text(result, 'node', node, 'tls_server_name');
	}

	if (node.insecure)
		warning(result, 'INSECURE_TLS', 'node', node.id, 'insecure', '该节点跳过 TLS 证书校验');
}

function is_ip_literal(value) {
	if (!has_text(value))
		return false;

	if (match(value, /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/) != null) {
		const parts = split(value, '.');
		for (let part in parts) {
			if (int(part) < 0 || int(part) > 255)
				return false;
		}
		return true;
	}

	return index(value, ':') >= 0 && match(value, /^[0-9A-Fa-f:]+$/) != null;
}

function validate_bootstrap(result, bootstrap) {
	if (bootstrap == null) {
		error(result, 'MISSING_BOOTSTRAP', 'bootstrap', null, null,
			'必须配置独立的非递归 Bootstrap DNS');
		return;
	}

	if (!(bootstrap.protocol in [ 'udp', 'tcp' ]))
		error(result, 'UNSUPPORTED_BOOTSTRAP_PROTOCOL', 'bootstrap', bootstrap.id, 'protocol',
			'第一版 Bootstrap DNS 只支持 udp 或 tcp');

	if (!is_ip_literal(bootstrap.server))
		error(result, 'BOOTSTRAP_NOT_IP', 'bootstrap', bootstrap.id, 'server',
			'Bootstrap DNS 必须使用 IP 字面量，不能再次依赖域名解析');

	validate_port(result, 'bootstrap', bootstrap, 'server_port');

	if (!(bootstrap.strategy in [ 'prefer_ipv4', 'prefer_ipv6', 'ipv4_only', 'ipv6_only' ]))
		error(result, 'INVALID_BOOTSTRAP_STRATEGY', 'bootstrap', bootstrap.id, 'strategy',
			'Bootstrap strategy 无效');
}

function validate_outbound(result, outbound, node_index) {
	if (!(outbound.kind in [ 'direct', 'block', 'single' ])) {
		error(result, 'UNSUPPORTED_OUTBOUND_KIND', 'outbound', outbound.id, 'kind',
			'第一版只支持 direct、block 和 single');
		return;
	}

	if (outbound.failure != 'block')
		error(result, 'UNSUPPORTED_FAILURE_POLICY', 'outbound', outbound.id, 'failure',
			'第一版只实现显式阻断，尚未实现连接失败后直连或切换出口');

	if (outbound.kind == 'single') {
		require_text(result, 'outbound', outbound, 'node');
		if (has_text(outbound.node) && node_index[outbound.node] == null)
			error(result, 'DANGLING_NODE', 'outbound', outbound.id, 'node', '引用的节点不存在');
		else if (has_text(outbound.node) && !node_index[outbound.node].enabled)
			error(result, 'DISABLED_NODE', 'outbound', outbound.id, 'node', '引用的节点已禁用');
	}
}

function safe_token(value) {
	return has_text(value) && match(value, /^[A-Za-z0-9_.:\-]+$/) != null;
}

function validate_dns_server(result, server) {
	if (!server.enabled)
		return;

	if (!(server.protocol in [ 'udp', 'tcp', 'tls', 'https' ]))
		error(result, 'UNSUPPORTED_DNS_PROTOCOL', 'dns_server', server.id, 'protocol',
			'第一版 DNS 只支持 udp、tcp、tls 和 https');

	require_text(result, 'dns_server', server, 'server');
	if (has_text(server.server) && !safe_token(server.server))
		error(result, 'INVALID_DNS_SERVER', 'dns_server', server.id, 'server', '服务器地址包含不允许的字符');
	validate_port(result, 'dns_server', server, 'server_port');

	if (server.protocol in [ 'tls', 'https' ])
		require_text(result, 'dns_server', server, 'tls_server_name');
	if (has_text(server.tls_server_name) && match(server.tls_server_name, /^[A-Za-z0-9_.\-]+$/) == null)
		error(result, 'INVALID_TLS_SERVER_NAME', 'dns_server', server.id, 'tls_server_name',
			'TLS server name 包含不允许的字符');

	if (server.protocol == 'https' && has_text(server.path) && match(server.path, /^\/[A-Za-z0-9_./?=&%\-]*$/) == null)
		error(result, 'INVALID_DNS_PATH', 'dns_server', server.id, 'path', 'DoH 路径包含不允许的字符');
}

function validate_dns_profile(result, profile, dns_server_index) {
	if (!profile.enabled)
		return;

	validate_port(result, 'dns_profile', profile, 'listen_port');

	if (length(profile.server || []) == 0)
		error(result, 'NO_DNS_SERVER', 'dns_profile', profile.id, 'server', 'DNS Profile 至少需要一个上游服务器');

	for (let server_id in (profile.server || [])) {
		if (dns_server_index[server_id] == null || !dns_server_index[server_id].enabled)
			error(result, 'DANGLING_DNS_SERVER', 'dns_profile', profile.id, 'server',
				`引用的 DNS Server 不存在或已禁用：${server_id}`);
	}

	if (!(profile.response_mode in [ 'first-ping', 'fastest-ip', 'fastest-response' ]))
		error(result, 'INVALID_RESPONSE_MODE', 'dns_profile', profile.id, 'response_mode',
			'SmartDNS response-mode 必须是 first-ping、fastest-ip 或 fastest-response');

	if (has_text(profile.speed_check_mode) &&
	    match(profile.speed_check_mode, /^(none|ping|tcp:[0-9]+|tcp-syn:[0-9]+)(,(ping|tcp:[0-9]+|tcp-syn:[0-9]+))*$/) == null)
		error(result, 'INVALID_SPEED_CHECK_MODE', 'dns_profile', profile.id, 'speed_check_mode',
			'SmartDNS speed-check-mode 格式无效');

	if (profile.failure != 'block')
		error(result, 'UNSUPPORTED_DNS_FAILURE_POLICY', 'dns_profile', profile.id, 'failure',
			'DNS Profile 上游全部失败时必须显式失败，不能隐藏降级到其他 Profile');

	validate_optional_integer_range(result, 'dns_profile', profile, 'cache_size', 0, 10000000);
	validate_optional_integer_range(result, 'dns_profile', profile, 'rr_ttl_min', 0, 604800);
}

function validate_local_proxy(result, proxy) {
	if (!proxy.enabled)
		return;

	if (!(proxy.protocol in [ 'mixed', 'socks', 'http' ]))
		error(result, 'UNSUPPORTED_LOCAL_PROXY_PROTOCOL', 'local_proxy', proxy.id, 'protocol',
			'本地代理入口只支持 mixed、socks 或 http');

	if (!is_ip_literal(proxy.listen))
		error(result, 'INVALID_LISTEN_ADDRESS', 'local_proxy', proxy.id, 'listen',
			'本地代理入口必须使用明确的 IP 监听地址');
	validate_port(result, 'local_proxy', proxy, 'listen_port');

	const has_username = has_text(proxy.username);
	const has_password = has_text(proxy.password);
	if (has_username != has_password)
		error(result, 'INCOMPLETE_LOCAL_PROXY_AUTH', 'local_proxy', proxy.id, 'username',
			'用户名和密码必须同时设置或同时留空');

	if (!(proxy.listen in [ '127.0.0.1', '::1' ]) && !(has_username && has_password))
		error(result, 'LOCAL_PROXY_AUTH_REQUIRED', 'local_proxy', proxy.id, 'username',
			'非回环地址上的本地代理入口必须配置用户名和密码');
}

function geo_rule_set_id(kind, category) {
	return `${kind}-${category}`;
}

function rule_set_path(rule_set) {
	return `/var/lib/steer/geodata/current/rules/${rule_set.id}.srs`;
}

function rule_set_tag(kind, category) {
	return `steer-${geo_rule_set_id(kind, category)}`;
}

function geo_category_valid(category) {
	return match(category, /^[a-z0-9_!.\-]+(@[a-z0-9_!.\-]+)?$/) != null;
}

function domain_name_valid(value) {
	if (!has_text(value) || length(value) > 253 || match(value, /^[A-Za-z0-9.\-]+$/) == null)
		return false;

	for (let label in split(value, '.')) {
		if (!has_text(label) || length(label) > 63 ||
		    substr(label, 0, 1) == '-' || substr(label, length(label) - 1, 1) == '-')
			return false;
	}

	return true;
}

function regular_expression_valid(value) {
	try {
		regexp(value);
		return true;
	}
	catch (exception) {
		return false;
	}
}

function parse_domain_match(value) {
	for (let item in [
		[ 'full:', 'domain' ],
		[ 'domain:', 'domain_suffix' ],
		[ 'regexp:', 'domain_regex' ],
		[ 'geosite:', 'geosite' ]
	]) {
		if (substr(value, 0, length(item[0])) == item[0])
			return { field: item[1], value: substr(value, length(item[0])) };
	}

	return { field: 'domain_keyword', value };
}

function parse_ip_match(value) {
	if (substr(value, 0, 6) == 'geoip:')
		return { field: 'geoip', value: substr(value, 6) };

	return { field: 'ip_cidr', value };
}

export function collect_geo_rule_sets(model, requested_kind) {
	let result = [];
	let seen = {};
	for (let rule in (model.rules || [])) {
		if (!rule.enabled || rule.default)
			continue;

		let matches = [];
		for (let value in (rule.domain_match || []))
			push(matches, parse_domain_match(value));
		for (let value in (rule.ip_match || []))
			push(matches, parse_ip_match(value));

		for (let item in matches) {
			if (!(item.field in [ 'geosite', 'geoip' ]) ||
			    (requested_kind != null && item.field != requested_kind))
				continue;
			const id = geo_rule_set_id(item.field, item.value);
			if (seen[id])
				continue;
			seen[id] = true;
			push(result, { id, kind: item.field, category: item.value });
		}
	}
	return result;
};

function has_rule_match(rule) {
	for (let option in [
		'inbound', 'domain_match', 'ip_match', 'source_ip_cidr',
		'network', 'protocol', 'port'
	]) {
		if (length(rule[option] || []) > 0)
			return true;
	}

	return false;
}

function has_dns_rule_match(rule) {
	for (let option in [ 'inbound', 'domain_match', 'source_ip_cidr' ]) {
		if (length(rule[option] || []) > 0)
			return true;
	}
	return false;
}

function ip_cidr_valid(value) {
	const parts = split(value, '/');
	if (length(parts) > 2 || !is_ip_literal(parts[0]))
		return false;

	if (length(parts) == 2) {
		const maximum = index(parts[0], ':') >= 0 ? 128 : 32;
		if (match(parts[1], /^[0-9]+$/) == null || int(parts[1]) > maximum)
			return false;
	}

	return true;
}

function validate_ip_cidrs(result, rule, option) {
	for (let value in (rule[option] || [])) {
		if (!ip_cidr_valid(value))
			error(result, 'INVALID_IP_CIDR', 'rule', rule.id, option,
				`IP 或 CIDR 无效：${value}`);
	}
}

function validate_domain_matches(result, rule) {
	for (let value in (rule.domain_match || [])) {
		const item = parse_domain_match(value);
		if (!has_text(value) || trim(value) != value || match(value, /[\r\n]/) != null) {
			error(result, 'INVALID_DOMAIN_MATCH', 'rule', rule.id, 'domain_match',
				`域名表达式无效：${value}`);
			continue;
		}
		if (item.field in [ 'domain', 'domain_suffix' ] && !domain_name_valid(item.value))
			error(result, 'INVALID_DOMAIN_MATCH', 'rule', rule.id, 'domain_match',
				`域名表达式无效：${value}`);
		else if (item.field == 'domain_regex' &&
		         (!has_text(item.value) || !regular_expression_valid(item.value)))
			error(result, 'INVALID_DOMAIN_MATCH', 'rule', rule.id, 'domain_match',
				`域名正则表达式无效：${value}`);
		else if (item.field == 'geosite' && !geo_category_valid(item.value))
			error(result, 'INVALID_GEO_CATEGORY', 'rule', rule.id, 'domain_match',
				`geosite 分类名称无效：${item.value}`);
		else if (item.field == 'domain_keyword' &&
		         (match(item.value, /[\t ]/) != null || index(item.value, ':') >= 0))
			error(result, 'INVALID_DOMAIN_MATCH', 'rule', rule.id, 'domain_match',
				`域名关键词或前缀无效：${value}`);
	}
}

function validate_destination_matches(result, rule) {
	for (let value in (rule.ip_match || [])) {
		const item = parse_ip_match(value);
		if (item.field == 'geoip') {
			if (!geo_category_valid(item.value))
				error(result, 'INVALID_GEO_CATEGORY', 'rule', rule.id, 'ip_match',
					`geoip 分类名称无效：${item.value}`);
		}
		else if (!ip_cidr_valid(item.value))
			error(result, 'INVALID_IP_CIDR', 'rule', rule.id, 'ip_match',
				`目的 IP、CIDR 或 geoip 表达式无效：${value}`);
	}
}

function validate_rule_ports(result, rule) {
	for (let value in (rule.port || [])) {
		if (match(value, /^[0-9]+$/) == null || int(value) < 1 || int(value) > 65535)
			error(result, 'INVALID_RULE_PORT', 'rule', rule.id, 'port',
				`连接目标端口无效：${value}`);
	}
}

function validate_rule_networks(result, rule) {
	for (let value in (rule.network || [])) {
		if (!(value in [ 'tcp', 'udp' ]))
			error(result, 'INVALID_RULE_NETWORK', 'rule', rule.id, 'network',
				`透明代理规则的网络类型只支持 tcp 或 udp：${value}`);
	}
}

function validate_rule_reference(result, rule, index, option, code, disabled) {
	if (!has_text(rule[option])) {
		if (!disabled)
			error(result, 'REQUIRED', 'rule', rule.id, option, '该字段不能为空');
		return;
	}

	if (index[rule[option]] != null && index[rule[option]].enabled !== false)
		return;

	if (disabled)
		warning(result, code, 'rule', rule.id, option, '禁用规则保留了当前不可用的引用，不会进入生成配置');
	else
		error(result, code, 'rule', rule.id, option, '引用的对象不存在或已禁用');
}

function validate_rule(result, rule, outbound_index, dns_index, local_proxy_index) {
	const disabled = !rule.enabled;

	if (rule.__duplicate_main) {
		error(result, 'MULTIPLE_MAIN', 'steer', rule.id, null, '只能存在一个 steer 主配置段');
		return;
	}
	if (rule.__duplicate_bootstrap) {
		error(result, 'MULTIPLE_BOOTSTRAP', 'bootstrap', rule.id, null, '只能存在一个 Bootstrap DNS 配置段');
		return;
	}

	if (!rule.default && !has_rule_match(rule) && !disabled)
		error(result, 'EMPTY_MATCH', 'rule', rule.id, null, '非 Default 的启用规则至少需要一个匹配条件');

	if (rule.default && has_rule_match(rule))
		error(result, 'DEFAULT_HAS_MATCH', 'rule', rule.id, null, 'Default 规则不能包含匹配条件');

	if (!disabled) {
		validate_domain_matches(result, rule);
		validate_destination_matches(result, rule);
		validate_ip_cidrs(result, rule, 'source_ip_cidr');
		validate_rule_ports(result, rule);
		validate_rule_networks(result, rule);

		if (!rule.default && !has_dns_rule_match(rule))
			warning(result, 'DNS_PROJECTION_EMPTY', 'rule', rule.id, 'dns_profile',
				'该规则只有连接阶段条件；所选 DNS Profile 不会匹配查询，DNS 将继续匹配后续规则');
	}

	validate_rule_reference(result, rule, dns_index, 'dns_profile', 'DANGLING_DNS_PROFILE', disabled);
	validate_rule_reference(result, rule, outbound_index, 'outbound', 'DANGLING_OUTBOUND', disabled);

	for (let proxy_id in (rule.inbound || [])) {
		if (local_proxy_index[proxy_id] != null && local_proxy_index[proxy_id].enabled)
			continue;
		if (disabled)
			warning(result, 'DANGLING_LOCAL_PROXY', 'rule', rule.id, 'inbound',
				`禁用规则保留了不可用的本地代理入口：${proxy_id}`);
		else
			error(result, 'DANGLING_LOCAL_PROXY', 'rule', rule.id, 'inbound',
				`引用的本地代理入口不存在或已禁用：${proxy_id}`);
	}
}

export function validate_model(model) {
	let result = { ok: false, errors: [], warnings: [] };

	if (type(model) != 'object') {
		error(result, 'INVALID_MODEL', 'model', null, null, '配置模型必须是对象');
		return result;
	}

	if (model.main == null) {
		error(result, 'MISSING_MAIN', 'steer', null, null, '缺少 steer 主配置段');
		return result;
	}

	if (model.main.schema_version != SCHEMA_VERSION)
		error(result, 'UNSUPPORTED_SCHEMA', 'steer', model.main.id, 'schema_version',
			`当前只支持 schema ${SCHEMA_VERSION}`);

	validate_bootstrap(result, model.bootstrap);
	validate_global_ids(result, model);
	validate_known_options(result, 'steer', model.main);
	validate_known_options(result, 'bootstrap', model.bootstrap);
	for (let node in (model.nodes || []))
		validate_known_options(result, 'node', node);
	for (let outbound in (model.outbounds || []))
		validate_known_options(result, 'outbound', outbound);
	for (let server in (model.dns_servers || []))
		validate_known_options(result, 'dns_server', server);
	for (let profile in (model.dns_profiles || []))
		validate_known_options(result, 'dns_profile', profile);
	for (let proxy in (model.local_proxies || []))
		validate_known_options(result, 'local_proxy', proxy);
	for (let rule in (model.rules || []))
		validate_known_options(result, 'rule', rule);

	if (!(model.main.log_level in [ 'error', 'warn', 'info', 'debug' ]))
		error(result, 'INVALID_LOG_LEVEL', 'steer', model.main.id, 'log_level',
			'日志级别只支持 error、warn、info 或 debug');

	validate_port(result, 'steer', model.main, 'tproxy_port');
	validate_port(result, 'steer', model.main, 'dns_port');
	validate_integer_range(result, 'steer', model.main, 'dns_upstream_mark', 1, 4294967295);
	validate_integer_range(result, 'steer', model.main, 'routing_mark', 1, 4294967295);
	validate_integer_range(result, 'steer', model.main, 'tproxy_mark', 1, 4294967295);
	validate_integer_range(result, 'steer', model.main, 'mark_mask', 1, 4294967295);
	validate_integer_range(result, 'steer', model.main, 'route_table', 1, 252);
	validate_integer_range(result, 'steer', model.main, 'rule_priority', 1, 32765);

	if (model.main.tproxy_mark != null && model.main.mark_mask != null &&
	    (model.main.tproxy_mark & model.main.mark_mask) != model.main.tproxy_mark)
		error(result, 'MARK_OUTSIDE_MASK', 'steer', model.main.id, 'tproxy_mark',
			'TPROXY mark 不能包含 mask 以外的位');

	if (model.main.routing_mark != null && model.main.tproxy_mark != null &&
	    model.main.mark_mask != null &&
	    (model.main.routing_mark & model.main.mark_mask) == model.main.tproxy_mark)
		error(result, 'MARK_COLLISION', 'steer', model.main.id, 'routing_mark',
			'sing-box 出站绕行标记不能与 TPROXY 接管标记相同');

	if (model.main.dns_upstream_mark != null && model.main.mark_mask != null &&
	    (model.main.dns_upstream_mark & model.main.mark_mask) != 0)
		error(result, 'DNS_UPSTREAM_MARK_COLLISION', 'steer', model.main.id, 'dns_upstream_mark',
			'SmartDNS 上游防回环 mark 必须位于 TPROXY mark mask 之外');

	if (model.main.tproxy_port == model.main.dns_port)
		error(result, 'PORT_COLLISION', 'steer', model.main.id, 'dns_port', 'DNS 与 TPROXY 监听端口不能相同');
	if (model.main.enabled && length(model.main.managed_zone || []) == 0)
		error(result, 'NO_MANAGED_ZONE', 'steer', model.main.id, 'managed_zone',
			'启用透明代理前必须明确选择至少一个受管 firewall zone');

	for (let zone in (model.main.managed_zone || [])) {
		if (!has_text(zone) || length(zone) > 32 || match(zone, /^[A-Za-z0-9_]+$/) == null)
			error(result, 'INVALID_MANAGED_ZONE', 'steer', model.main.id, 'managed_zone',
				`firewall zone 名称无效：${zone}`);
	}

	const node_index = index_objects(result, model.nodes || [], 'node');
	const outbound_index = index_objects(result, model.outbounds || [], 'outbound');
	const dns_server_index = index_objects(result, model.dns_servers || [], 'dns_server');
	const dns_index = index_objects(result, model.dns_profiles || [], 'dns_profile');
	const local_proxy_index = index_objects(result, model.local_proxies || [], 'local_proxy');
	index_objects(result, model.rules || [], 'rule');

	for (let node in (model.nodes || []))
		validate_node(result, node);
	for (let outbound in (model.outbounds || []))
		validate_outbound(result, outbound, node_index);
	for (let server in (model.dns_servers || []))
		validate_dns_server(result, server);
	for (let profile in (model.dns_profiles || []))
		validate_dns_profile(result, profile, dns_server_index);
	for (let proxy in (model.local_proxies || []))
		validate_local_proxy(result, proxy);
	let listen_ports = {};
	for (let profile in (model.dns_profiles || [])) {
		if (!profile.enabled || profile.listen_port == null)
			continue;

		if (profile.listen_port == model.main.tproxy_port ||
		    profile.listen_port == model.main.dns_port)
			error(result, 'PORT_COLLISION', 'dns_profile', profile.id, 'listen_port',
				'DNS Profile 监听端口与 Steer 核心端口冲突');
		else if (listen_ports[profile.listen_port] != null)
			error(result, 'PORT_COLLISION', 'dns_profile', profile.id, 'listen_port',
				`与 DNS Profile ${listen_ports[profile.listen_port]} 使用相同监听端口`);
		else
			listen_ports[profile.listen_port] = profile.id;
	}
	for (let proxy in (model.local_proxies || [])) {
		if (!proxy.enabled || proxy.listen_port == null)
			continue;

		if (proxy.listen_port == model.main.tproxy_port ||
		    proxy.listen_port == model.main.dns_port)
			error(result, 'PORT_COLLISION', 'local_proxy', proxy.id, 'listen_port',
				'本地代理入口与 Steer 核心端口冲突');
		else if (listen_ports[proxy.listen_port] != null)
			error(result, 'PORT_COLLISION', 'local_proxy', proxy.id, 'listen_port',
				`与 ${listen_ports[proxy.listen_port]} 使用相同监听端口`);
		else
			listen_ports[proxy.listen_port] = `local_proxy ${proxy.id}`;
	}

	let enabled_default_count = 0;
	let seen_enabled_default = false;
	for (let rule in (model.rules || [])) {
		validate_rule(result, rule, outbound_index, dns_index, local_proxy_index);

		if (!rule.enabled)
			continue;

		if (rule.default) {
			enabled_default_count++;
			seen_enabled_default = true;
		}
		else if (seen_enabled_default) {
			error(result, 'RULE_AFTER_DEFAULT', 'rule', rule.id, null,
				'启用规则不能位于 Default 之后');
		}
	}

	if (enabled_default_count != 1)
		error(result, 'DEFAULT_COUNT', 'rule', null, null, '必须恰好存在一条启用的 Default 规则');

	result.ok = length(result.errors) == 0;
	return result;
};

function remove_empty(value) {
	if (type(value) == 'array') {
		let result = [];
		for (let item in value) {
			const clean = remove_empty(item);
			if (clean != null)
				push(result, clean);
		}
		return length(result) ? result : null;
	}

	if (type(value) == 'object') {
		let result = {};
		for (let key in keys(value)) {
			const clean = remove_empty(value[key]);
			if (clean != null)
				result[key] = clean;
		}
		return length(keys(result)) ? result : null;
	}

	if (value == null || value == '')
		return null;

	return value;
}

function outbound_tag(id) {
	return `steer-out-${id}`;
}

function node_tag(id) {
	return `steer-node-${id}`;
}

function dns_tag(id) {
	return `steer-dns-${id}`;
}

function local_proxy_tag(id) {
	return `steer-local-${id}`;
}

function compile_local_proxy(proxy) {
	let inbound = {
		type: proxy.protocol,
		tag: local_proxy_tag(proxy.id),
		listen: proxy.listen,
		listen_port: proxy.listen_port
	};

	if (has_text(proxy.username))
		inbound.users = [ { username: proxy.username, password: proxy.password } ];

	return inbound;
}

function compile_tls(source, reality) {
	let tls = {
		enabled: true,
		server_name: source.tls_server_name,
		insecure: source.insecure ? true : null
	};

	if (has_text(source.utls_fingerprint))
		tls.utls = { enabled: true, fingerprint: source.utls_fingerprint };

	if (reality)
		tls.reality = {
			enabled: true,
			public_key: source.reality_public_key,
			short_id: source.reality_short_id
		};

	return tls;
}

function compile_node(node, routing_mark) {
	let server_ports = node.type == 'hysteria2' && length(node.server_ports || [])
		? node.server_ports
		: null;
	let outbound = {
		type: node.type,
		tag: node_tag(node.id),
		server: node.server,
		server_port: server_ports == null ? node.server_port : null,
		routing_mark
	};

	if (node.type == 'vless') {
		outbound.uuid = node.uuid;
		outbound.flow = node.flow;
		outbound.packet_encoding = node.packet_encoding;
		if (has_text(node.tls_server_name) || has_text(node.reality_public_key))
			outbound.tls = compile_tls(node, has_text(node.reality_public_key));
	}
	else if (node.type == 'hysteria2') {
		outbound.password = node.password;
		outbound.server_ports = server_ports;
		outbound.hop_interval = node.hop_interval;
		outbound.up_mbps = node.up_mbps;
		outbound.down_mbps = node.down_mbps;
		outbound.tls = compile_tls(node, false);
		if (has_text(node.obfs_type))
			outbound.obfs = { type: node.obfs_type, password: node.obfs_password };
	}
	else if (node.type == 'trojan') {
		outbound.password = node.password;
		outbound.tls = compile_tls(node, false);
	}

	return remove_empty(outbound);
}

function compile_dns_profile(profile) {
	return {
		type: 'udp',
		tag: dns_tag(profile.id),
		server: '127.0.0.1',
		server_port: profile.listen_port
	};
}

function compile_bootstrap(bootstrap, routing_mark) {
	return {
		type: bootstrap.protocol,
		tag: 'steer-dns-bootstrap',
		server: bootstrap.server,
		server_port: bootstrap.server_port,
		routing_mark
	};
}

function smartdns_address(server) {
	if (index(server.server, ':') >= 0 && substr(server.server, 0, 1) != '[')
		return `[${server.server}]:${server.server_port}`;

	return `${server.server}:${server.server_port}`;
}

function compile_smartdns_server(server, dns_upstream_mark) {
	let directive;
	let address = smartdns_address(server);

	if (server.protocol == 'udp')
		directive = `server ${address}`;
	else if (server.protocol == 'tcp')
		directive = `server-tcp ${address}`;
	else if (server.protocol == 'tls')
		directive = `server-tls ${address}`;
	else
		directive = `server-https https://${address}${server.path || '/dns-query'}`;

	if (server.protocol in [ 'tls', 'https' ])
		directive += ` -host-name ${server.tls_server_name} -tls-host-verify ${server.tls_server_name}`;

	if (server.insecure)
		directive += ' -no-check-certificate';

	directive += ` -set-mark ${dns_upstream_mark}`;

	return directive;
}

function yes_no(value) {
	return value ? 'yes' : 'no';
}

function compile_smartdns_profile(profile, dns_server_index, dns_upstream_mark, bootstrap, routing_mark, log_level) {
	let lines = [
		`server-name steer-${profile.id}`,
		`bind 127.0.0.1:${profile.listen_port}`,
		`bind-tcp 127.0.0.1:${profile.listen_port}`,
		`cache-size ${profile.cache_size ?? 32768}`,
		`cache-persist ${yes_no(profile.cache_persist)}`,
		`cache-file /var/lib/steer/smartdns/${profile.id}.cache`,
		`serve-expired ${yes_no(profile.serve_expired)}`,
		`response-mode ${profile.response_mode}`,
		`speed-check-mode ${profile.speed_check_mode || 'none'}`,
		`log-level ${log_level || 'warn'}`,
		'log-console yes',
		'force-qtype-SOA -'
	];

	if (profile.rr_ttl_min != null)
		push(lines, `rr-ttl-min ${profile.rr_ttl_min}`);

	let bootstrap_directive = bootstrap.protocol == 'tcp' ? 'server-tcp' : 'server';
	push(lines,
		`${bootstrap_directive} ${smartdns_address(bootstrap)} -bootstrap-dns -exclude-default-group -set-mark ${routing_mark}`);

	for (let server_id in profile.server)
		push(lines, compile_smartdns_server(dns_server_index[server_id], dns_upstream_mark));

	return `${join('\n', lines)}\n`;
}

function append_match_value(result, field, value) {
	if (result[field] == null)
		result[field] = [];
	push(result[field], value);
}

function combine_groups(groups, mode) {
	if (length(groups) == 1)
		return groups[0];
	return { type: 'logical', mode, rules: groups };
}

function combine_match_fields(result, fields) {
	let groups = [];
	for (let field in fields) {
		if (length(result[field] || [])) {
			let group = {};
			group[field] = result[field];
			push(groups, group);
		}
	}
	return combine_groups(groups, 'or');
}

function compile_domain_group(rule) {
	let result = {};
	for (let value in (rule.domain_match || [])) {
		const item = parse_domain_match(value);
		if (item.field == 'geosite')
			append_match_value(result, 'rule_set', rule_set_tag('geosite', item.value));
		else
			append_match_value(result, item.field, item.value);
	}
	return combine_match_fields(result,
		[ 'domain', 'domain_suffix', 'domain_keyword', 'domain_regex', 'rule_set' ]);
}

function compile_destination_group(rule) {
	let result = {};
	for (let value in (rule.ip_match || [])) {
		const item = parse_ip_match(value);
		if (item.field == 'geoip')
			append_match_value(result, 'rule_set', rule_set_tag('geoip', item.value));
		else
			append_match_value(result, item.field, item.value);
	}
	return combine_match_fields(result, [ 'ip_cidr', 'rule_set' ]);
}

function compile_rule_match(rule) {
	let groups = [];
	if (length(rule.inbound || []))
		push(groups, { inbound: map(rule.inbound, local_proxy_tag) });
	if (length(rule.domain_match || []))
		push(groups, compile_domain_group(rule));
	if (length(rule.ip_match || []))
		push(groups, compile_destination_group(rule));
	for (let option in [ 'source_ip_cidr', 'network', 'protocol' ]) {
		if (length(rule[option] || [])) {
			let group = {};
			group[option] = rule[option];
			push(groups, group);
		}
	}
	if (length(rule.port || []))
		push(groups, { port: map(rule.port, (value) => int(value)) });

	return combine_groups(groups, 'and');
}

function compile_dns_rule_match(rule) {
	let groups = [];
	if (length(rule.inbound || []))
		push(groups, { inbound: map(rule.inbound, local_proxy_tag) });
	if (length(rule.domain_match || []))
		push(groups, compile_domain_group(rule));
	if (length(rule.source_ip_cidr || []))
		push(groups, { source_ip_cidr: rule.source_ip_cidr });

	return length(groups) ? combine_groups(groups, 'and') : {};
}

export function compile_model(model) {
	const validation = validate_model(model);
	if (!validation.ok)
		return { ok: false, validation };

	let config = {
		log: {
			level: model.main.log_level || 'warn',
			timestamp: true
		},
		dns: {
			servers: [ compile_bootstrap(model.bootstrap, model.main.routing_mark) ],
			rules: [],
			independent_cache: true,
			reverse_mapping: true
		},
		inbounds: [
			{
				type: 'tproxy',
				tag: 'steer-tproxy-in',
				listen: '::',
				listen_port: model.main.tproxy_port
			},
			{
				type: 'direct',
				tag: 'steer-dns-in',
				listen: '::',
				listen_port: model.main.dns_port
			}
		],
		outbounds: [],
		route: {
			rules: [
				{ inbound: 'steer-dns-in', action: 'hijack-dns' },
				{ inbound: 'steer-tproxy-in', action: 'sniff', timeout: '300ms' }
			],
			auto_detect_interface: true,
			default_domain_resolver: {
				server: 'steer-dns-bootstrap',
				strategy: model.bootstrap.strategy
			}
		}
	};

	let dns_server_index = {};
	for (let server in model.dns_servers)
		dns_server_index[server.id] = server;
	config.route.rule_set = [];
	for (let rule_set in collect_geo_rule_sets(model)) {
		push(config.route.rule_set, {
			type: 'local',
			tag: rule_set_tag(rule_set.kind, rule_set.category),
			format: 'binary',
			path: rule_set_path(rule_set)
		});
	}

	let local_proxy_tags = [];
	for (let proxy in model.local_proxies) {
		if (!proxy.enabled)
			continue;
		push(config.inbounds, compile_local_proxy(proxy));
		push(local_proxy_tags, local_proxy_tag(proxy.id));
	}
	if (length(local_proxy_tags))
		push(config.route.rules, {
			inbound: local_proxy_tags,
			action: 'sniff',
			timeout: '300ms'
		});

	for (let node in model.nodes) {
		if (node.enabled)
			push(config.outbounds, compile_node(node, model.main.routing_mark));
	}

	for (let outbound in model.outbounds) {
		if (outbound.kind == 'direct')
			push(config.outbounds, {
				type: 'direct', tag: outbound_tag(outbound.id), routing_mark: model.main.routing_mark
			});
		else if (outbound.kind == 'block')
			push(config.outbounds, { type: 'block', tag: outbound_tag(outbound.id) });
		else
			push(config.outbounds, {
				type: 'selector',
				tag: outbound_tag(outbound.id),
				outbounds: [ node_tag(outbound.node) ],
				default: node_tag(outbound.node),
				interrupt_exist_connections: false
			});
	}

	for (let profile in model.dns_profiles) {
		if (profile.enabled)
			push(config.dns.servers, compile_dns_profile(profile));
	}

	let default_rule = null;
	for (let rule in model.rules) {
		if (!rule.enabled)
			continue;

		if (rule.default) {
			default_rule = rule;
			continue;
		}

		let route_rule = compile_rule_match(rule);
		route_rule.action = 'route';
		route_rule.outbound = outbound_tag(rule.outbound);
		push(config.route.rules, route_rule);

		let dns_rule = compile_dns_rule_match(rule);
		if (length(keys(dns_rule)) > 0) {
			dns_rule.action = 'route';
			dns_rule.server = rule.dns_profile == 'bootstrap' ?
				'steer-dns-bootstrap' : dns_tag(rule.dns_profile);
			push(config.dns.rules, dns_rule);
		}
	}

	config.route.final = outbound_tag(default_rule.outbound);
	config.dns.final = dns_tag(default_rule.dns_profile);

	let smartdns_instances = [];
	for (let profile in model.dns_profiles) {
		if (!profile.enabled)
			continue;

		push(smartdns_instances, {
			id: profile.id,
			listen_port: profile.listen_port,
			config: compile_smartdns_profile(
				profile, dns_server_index, model.main.dns_upstream_mark, model.bootstrap,
				model.main.routing_mark, model.main.log_level
			)
		});
	}

	return {
		ok: true,
		validation,
		sing_box: remove_empty(config),
		smartdns_instances
	};
};
