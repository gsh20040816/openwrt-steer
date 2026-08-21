/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

'use strict';
'require baseclass';

function fail(code, detail) {
	const error = new Error(code);
	error.code = code;
	error.detail = detail || '';
	throw error;
}

function decode(value, field) {
	try {
		return decodeURIComponent(value || '');
	}
	catch (error) {
		fail('INVALID_ENCODING', field);
	}
}

function splitLink(input) {
	const value = String(input || '').trim();
	if (!value)
		fail('EMPTY_URL');

	const schemeEnd = value.indexOf('://');
	if (schemeEnd <= 0)
		fail('INVALID_URL');

	const scheme = value.substring(0, schemeEnd).toLowerCase();
	let remainder = value.substring(schemeEnd + 3);
	let fragment = '';
	const fragmentAt = remainder.indexOf('#');
	if (fragmentAt >= 0) {
		fragment = decode(remainder.substring(fragmentAt + 1), 'fragment');
		remainder = remainder.substring(0, fragmentAt);
	}

	let query = '';
	const queryAt = remainder.indexOf('?');
	if (queryAt >= 0) {
		query = remainder.substring(queryAt + 1);
		remainder = remainder.substring(0, queryAt);
	}

	const slashAt = remainder.indexOf('/');
	if (slashAt >= 0) {
		const path = remainder.substring(slashAt);
		if (path != '/' && path != '')
			fail('UNSUPPORTED_PATH', path);
		remainder = remainder.substring(0, slashAt);
	}

	const userAt = remainder.lastIndexOf('@');
	if (userAt < 0)
		fail('MISSING_CREDENTIAL');

	const credential = decode(remainder.substring(0, userAt), 'credential');
	const authority = remainder.substring(userAt + 1);
	if (!credential)
		fail('MISSING_CREDENTIAL');

	return {
		scheme,
		credential,
		authority,
		fragment,
		params: new URLSearchParams(query)
	};
}

function parseAuthority(authority, defaultPort, allowMultiplePorts) {
	let host;
	let ports = '';

	if (authority.substring(0, 1) == '[') {
		const closing = authority.indexOf(']');
		if (closing < 0)
			fail('INVALID_HOST', authority);
		host = authority.substring(1, closing);
		const suffix = authority.substring(closing + 1);
		if (suffix && suffix.substring(0, 1) != ':')
			fail('INVALID_AUTHORITY', authority);
		ports = suffix.substring(1);
	}
	else {
		const colon = authority.lastIndexOf(':');
		if (colon >= 0) {
			if (authority.indexOf(':') != colon)
				fail('IPV6_BRACKETS_REQUIRED', authority);
			host = authority.substring(0, colon);
			ports = authority.substring(colon + 1);
		}
		else {
			host = authority;
		}
	}

	host = decode(host, 'host');
	if (!host || /[\s/?#]/.test(host))
		fail('INVALID_HOST', host);

	ports = ports || String(defaultPort);
	if (!allowMultiplePorts && !/^[0-9]+$/.test(ports))
		fail('INVALID_PORT', ports);

	const serverPorts = parseServerPorts(ports);
	return {
		host,
		port: Number(serverPorts[0].split(':')[0]),
		serverPorts: serverPorts.length > 1 || serverPorts[0].indexOf(':') >= 0 ? serverPorts : []
	};
}

function parseServerPorts(ports) {
	const serverPorts = [];
	for (const item of ports.split(',')) {
		const match = item.match(/^([0-9]+)(?:-([0-9]+))?$/);
		if (!match)
			fail('INVALID_PORT', item);
		const first = Number(match[1]);
		const last = Number(match[2] || match[1]);
		if (first < 1 || first > 65535 || last < first || last > 65535)
			fail('INVALID_PORT', item);
		serverPorts.push(first == last ? String(first) : '%d:%d'.format(first, last));
	}

	return serverPorts;
}

function parameterValues(params, names) {
	const values = [];
	for (const name of names)
		for (const value of params.getAll(name))
			if (!values.includes(value))
				values.push(value);
	if (values.length > 1)
		fail('CONFLICTING_PARAMETER', names.join('/'));
	return values[0] ?? '';
}

function booleanParameter(params, names, fallback) {
	const value = parameterValues(params, names);
	if (value == '')
		return fallback;
	if (value == '1' || value.toLowerCase() == 'true')
		return true;
	if (value == '0' || value.toLowerCase() == 'false')
		return false;
	fail('INVALID_BOOLEAN', names[0]);
}

function integerParameter(params, names) {
	const value = parameterValues(params, names);
	if (value == '')
		return null;
	if (!/^[1-9][0-9]*$/.test(value))
		fail('INVALID_INTEGER', names[0]);
	return Number(value);
}

function validateKnownParameters(params, known) {
	for (const name of params.keys())
		if (!known.includes(name))
			fail('UNSUPPORTED_PARAMETER', name);
}

function requireParameter(value, name) {
	if (!value)
		fail('REQUIRED_PARAMETER', name);
	return value;
}

function baseNode(link, authority, type) {
	return {
		type,
		enabled: '1',
		name: link.fragment || '%s %s:%d'.format(type.toUpperCase(), authority.host, authority.port),
		server: authority.host,
		server_port: String(authority.port)
	};
}

function decodeBase64(value) {
	try {
		let normalized = String(value || '').replace(/-/g, '+').replace(/_/g, '/');
		while (normalized.length % 4)
			normalized += '=';
		const binary = atob(normalized);
		const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
		return new TextDecoder().decode(bytes);
	}
	catch (error) {
		fail('INVALID_BASE64');
	}
}

function parseSimpleCredential(link, type, tls) {
	const known = [ 'sni', 'serverName', 'allowInsecure', 'insecure', 'version' ];
	validateKnownParameters(link.params, known);
	const authority = parseAuthority(link.authority, tls ? 443 : 1080, false);
	const credential = link.credential.split(':');
	const node = baseNode(link, authority, type);
	if (credential.length > 2)
		fail('INVALID_CREDENTIAL');
	if (credential.length == 2) {
		node.username = credential[0];
		node.password = credential[1];
	}
	else if (type == 'socks' || type == 'http') {
		node.password = credential[0];
	}
	if (tls) {
		node.tls_server_name = parameterValues(link.params, [ 'sni', 'serverName' ]) || authority.host;
		node.insecure = booleanParameter(link.params, [ 'allowInsecure', 'insecure' ], false) ? '1' : '0';
	}
	return { node, warnings: [] };
}

function parseShadowsocksURL(input) {
	let value = String(input || '').trim().substring(5);
	const hashAt = value.indexOf('#');
	const fragment = hashAt >= 0 ? decode(value.substring(hashAt + 1), 'fragment') : '';
	if (hashAt >= 0)
		value = value.substring(0, hashAt);
	const at = value.lastIndexOf('@');
	if (at < 0)
		fail('MISSING_CREDENTIAL');
	const decoded = decodeBase64(value.substring(0, at));
	const pair = decoded.split(':');
	if (pair.length != 2)
		fail('INVALID_CREDENTIAL');
	const authority = parseAuthority(value.substring(at + 1), 443, false);
	return { node: Object.assign(baseNode({ fragment }, authority, 'shadowsocks'), { method: pair[0], password: pair[1] }), warnings: [] };
}

function parseVMessURL(input) {
	const payload = decodeBase64(String(input || '').trim().substring(8));
	let value;
	try { value = JSON.parse(payload); }
	catch (error) { fail('INVALID_VMESS_JSON'); }
	const authority = parseAuthority('%s:%s'.format(value.add, value.port || 443), 443, false);
	const node = baseNode({ fragment: value.ps || '' }, authority, 'vmess');
	node.uuid = value.id || '';
	node.alter_id = String(value.aid || 0);
	node.security = value.scy || 'auto';
	node.network = value.net || 'tcp';
	node.transport = value.net || 'tcp';
	node.transport_host = value.host || '';
	node.transport_path = value.path || '';
	if (value.tls && value.tls != 'none')
		node.tls_server_name = value.sni || value.add;
	return { node, warnings: [] };
}

function parseExtended(link) {
	if (link.scheme == 'socks' || link.scheme == 'socks5')
		return parseSimpleCredential(link, 'socks', false);
	if (link.scheme == 'http' || link.scheme == 'https')
		return parseSimpleCredential(link, 'http', link.scheme == 'https');
	if (link.scheme == 'shadowtls') {
		const result = parseSimpleCredential(link, 'shadowtls', true);
		result.node.password = link.credential;
		result.node.username = '';
		result.node.version = parameterValues(link.params, [ 'version' ]) || '1';
		return result;
	}
	if (link.scheme == 'anytls') {
		const result = parseSimpleCredential(link, 'anytls', true);
		result.node.password = link.credential;
		result.node.username = '';
		return result;
	}
	if (link.scheme == 'naive+https') {
		const result = parseSimpleCredential(link, 'naive', true);
		return result;
	}
	if (link.scheme == 'ssh')
		return parseSimpleCredential(link, 'ssh', false);
	if (link.scheme == 'tuic') {
		const known = [ 'congestion_control', 'udp_relay_mode', 'udp_over_stream', 'zero_rtt_handshake', 'heartbeat', 'sni', 'insecure' ];
		validateKnownParameters(link.params, known);
		const authority = parseAuthority(link.authority, 443, false);
		const credential = link.credential.split(':');
		if (credential.length != 2)
			fail('INVALID_CREDENTIAL');
		const node = baseNode(link, authority, 'tuic');
		node.uuid = credential[0]; node.password = credential[1];
		node.congestion_control = parameterValues(link.params, [ 'congestion_control' ]);
		node.udp_relay_mode = parameterValues(link.params, [ 'udp_relay_mode' ]);
		node.udp_over_stream = booleanParameter(link.params, [ 'udp_over_stream' ], false) ? '1' : '0';
		node.zero_rtt_handshake = booleanParameter(link.params, [ 'zero_rtt_handshake' ], false) ? '1' : '0';
		node.heartbeat = parameterValues(link.params, [ 'heartbeat' ]);
		node.tls_server_name = parameterValues(link.params, [ 'sni' ]) || authority.host;
		node.insecure = booleanParameter(link.params, [ 'insecure' ], false) ? '1' : '0';
		return { node, warnings: [] };
	}
	return null;
}

function parseVless(link) {
	const known = [
		'encryption', 'flow', 'security', 'sni', 'serverName', 'fp', 'fingerprint',
		'pbk', 'publicKey', 'sid', 'shortId', 'type', 'headerType', 'packetEncoding',
		'packet_encoding', 'allowInsecure', 'insecure', 'spx', 'path', 'host',
		'serviceName', 'authority', 'mode', 'extra', 'alpn', 'ech', 'fm'
	];
	validateKnownParameters(link.params, known);

	const authority = parseAuthority(link.authority, 443, false);
	if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(link.credential))
		fail('INVALID_VLESS_UUID', 'uuid');
	const transport = parameterValues(link.params, [ 'type' ]) || 'tcp';
	if (!([ 'tcp', 'raw' ].includes(transport)))
		fail('UNSUPPORTED_TRANSPORT', transport);
	for (const name of [ 'path', 'host', 'serviceName', 'authority', 'mode', 'extra', 'alpn', 'ech', 'fm' ])
		if (link.params.has(name) && link.params.get(name) != '')
			fail('UNSUPPORTED_PARAMETER', name);
	const header = parameterValues(link.params, [ 'headerType' ]);
	if (header && header != 'none')
		fail('UNSUPPORTED_HEADER', header);
	const encryption = parameterValues(link.params, [ 'encryption' ]);
	if (encryption && encryption != 'none')
		fail('UNSUPPORTED_ENCRYPTION', encryption);

	const security = parameterValues(link.params, [ 'security' ]) || 'none';
	if (!([ 'none', 'tls', 'reality' ].includes(security)))
		fail('UNSUPPORTED_SECURITY', security);
	const flow = parameterValues(link.params, [ 'flow' ]);
	if (flow && flow != 'xtls-rprx-vision')
		fail('UNSUPPORTED_FLOW', flow);
	const packetEncoding = parameterValues(link.params, [ 'packetEncoding', 'packet_encoding' ]);
	if (packetEncoding && !([ 'xudp', 'packetaddr' ].includes(packetEncoding)))
		fail('UNSUPPORTED_PACKET_ENCODING', packetEncoding);

	const sni = parameterValues(link.params, [ 'sni', 'serverName' ]);
	const publicKey = parameterValues(link.params, [ 'pbk', 'publicKey' ]);
	const shortId = parameterValues(link.params, [ 'sid', 'shortId' ]);
	const fingerprint = parameterValues(link.params, [ 'fp', 'fingerprint' ]);
	const insecure = booleanParameter(link.params, [ 'allowInsecure', 'insecure' ], false);
	if (security != 'reality' && (publicKey || shortId))
		fail('PARAMETER_REQUIRES_REALITY', publicKey ? 'pbk' : 'sid');
	if (security == 'none' && (sni || fingerprint || insecure))
		fail('PARAMETER_REQUIRES_TLS', sni ? 'sni' : fingerprint ? 'fp' : 'insecure');
	if (flow && security == 'none')
		fail('PARAMETER_REQUIRES_TLS', 'flow');

	const node = baseNode(link, authority, 'vless');
	node.uuid = link.credential;
	if (flow)
		node.flow = flow;
	if (packetEncoding)
		node.packet_encoding = packetEncoding;
	if (insecure)
		node.insecure = '1';

	if (security == 'tls') {
		node.tls_server_name = requireParameter(sni, 'sni');
		if (fingerprint)
			node.utls_fingerprint = fingerprint;
	}
	else if (security == 'reality') {
		node.tls_server_name = requireParameter(sni, 'sni');
		node.reality_public_key = requireParameter(publicKey, 'pbk');
		node.reality_short_id = requireParameter(shortId, 'sid');
		node.utls_fingerprint = requireParameter(fingerprint, 'fp');
	}

	const warnings = [];
	if (link.params.has('spx'))
		warnings.push({ code: 'IGNORED_NOOP_PARAMETER', detail: 'spx' });
	return { node, warnings };
}

function parseHysteria2(link) {
	const known = [
		'obfs', 'obfs-password', 'sni', 'insecure', 'pinSHA256', 'ech',
		'hop-interval', 'hopInterval', 'upmbps', 'upMbps', 'downmbps', 'downMbps',
		'mport'
	];
	validateKnownParameters(link.params, known);
	if (link.params.has('pinSHA256'))
		fail('UNSUPPORTED_PARAMETER', 'pinSHA256');
	if (link.params.has('ech'))
		fail('UNSUPPORTED_PARAMETER', 'ech');

	const authority = parseAuthority(link.authority, 443, true);
	const node = baseNode(link, authority, 'hysteria2');
	node.password = link.credential;
	node.tls_server_name = parameterValues(link.params, [ 'sni' ]) || authority.host;
	const multiPort = parameterValues(link.params, [ 'mport' ]);
	if (multiPort && authority.serverPorts.length)
		fail('CONFLICTING_PARAMETER', 'mport');
	if (multiPort)
		node.server_ports = parseServerPorts(multiPort);
	else if (authority.serverPorts.length)
		node.server_ports = authority.serverPorts;
	if (booleanParameter(link.params, [ 'insecure' ], false))
		node.insecure = '1';

	const obfs = parameterValues(link.params, [ 'obfs' ]);
	if (obfs && obfs != 'salamander')
		fail('UNSUPPORTED_HYSTERIA2_OBFS', obfs);
	if (!obfs && parameterValues(link.params, [ 'obfs-password' ]))
		fail('PARAMETER_REQUIRES_OBFS', 'obfs-password');
	if (obfs) {
		node.obfs_type = obfs;
		node.obfs_password = requireParameter(
			parameterValues(link.params, [ 'obfs-password' ]), 'obfs-password');
	}

	const hopInterval = parameterValues(link.params, [ 'hop-interval', 'hopInterval' ]);
	if (hopInterval)
		node.hop_interval = hopInterval;
	const upMbps = integerParameter(link.params, [ 'upmbps', 'upMbps' ]);
	const downMbps = integerParameter(link.params, [ 'downmbps', 'downMbps' ]);
	if (upMbps != null)
		node.up_mbps = String(upMbps);
	if (downMbps != null)
		node.down_mbps = String(downMbps);
	return { node, warnings: [] };
}

function parseTrojan(link) {
	const known = [
		'security', 'sni', 'serverName', 'fp', 'fingerprint', 'type', 'headerType',
		'allowInsecure', 'insecure', 'path', 'host', 'serviceName', 'authority',
		'alpn', 'ech'
	];
	validateKnownParameters(link.params, known);

	const authority = parseAuthority(link.authority, 443, false);
	const transport = parameterValues(link.params, [ 'type' ]) || 'tcp';
	if (!([ 'tcp', 'raw' ].includes(transport)))
		fail('UNSUPPORTED_TRANSPORT', transport);
	for (const name of [ 'path', 'host', 'serviceName', 'authority', 'alpn', 'ech' ])
		if (link.params.has(name) && link.params.get(name) != '')
			fail('UNSUPPORTED_PARAMETER', name);
	const header = parameterValues(link.params, [ 'headerType' ]);
	if (header && header != 'none')
		fail('UNSUPPORTED_HEADER', header);
	const security = parameterValues(link.params, [ 'security' ]) || 'tls';
	if (security != 'tls')
		fail('UNSUPPORTED_SECURITY', security);

	const node = baseNode(link, authority, 'trojan');
	node.password = link.credential;
	node.tls_server_name = parameterValues(link.params, [ 'sni', 'serverName' ]) || authority.host;
	const fingerprint = parameterValues(link.params, [ 'fp', 'fingerprint' ]);
	if (fingerprint)
		node.utls_fingerprint = fingerprint;
	if (booleanParameter(link.params, [ 'allowInsecure', 'insecure' ], false))
		node.insecure = '1';
	return { node, warnings: [] };
}

return baseclass.extend({
	parse: function(input) {
		const raw = String(input || '').trim();
		if (raw.toLowerCase().startsWith('vmess://'))
			return parseVMessURL(raw);
		if (raw.toLowerCase().startsWith('ss://'))
			return parseShadowsocksURL(raw);
		const link = splitLink(raw);
		const extended = parseExtended(link);
		if (extended)
			return extended;
		if (link.scheme == 'vless')
			return parseVless(link);
		if (link.scheme == 'hysteria2' || link.scheme == 'hy2')
			return parseHysteria2(link);
		if (link.scheme == 'trojan')
			return parseTrojan(link);
		fail('UNSUPPORTED_SCHEME', link.scheme);
	}
});
