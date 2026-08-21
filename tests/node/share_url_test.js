/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

'use strict';

const fs = require('fs');
const path = require('path');

if (!String.prototype.format) {
	String.prototype.format = function(...args) {
		let offset = 0;
		return this.replace(/%[sd]/g, () => String(args[offset++]));
	};
}

const source = fs.readFileSync(path.join(__dirname,
	'../../luci-app-steer/htdocs/luci-static/resources/steer/share-url.js'), 'utf8');
const parser = new Function('baseclass', source)({ extend: (value) => value });

function expect(condition, message) {
	if (!condition)
		throw new Error(message);
}

function expectError(code, input) {
	try {
		parser.parse(input);
	}
	catch (error) {
		expect(error.code == code, `expected ${code}, got ${error.code || error.message}`);
		return;
	}
	throw new Error(`expected ${code}`);
}

let result = parser.parse('vless://00000000-0000-4000-8000-000000000001@reality.example.com:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.example.com&fp=chrome&pbk=fixture-public-key&sid=0123456789abcdef&type=tcp&packetEncoding=xudp#Reality%20Fixture');
expect(result.node.type == 'vless', 'VLESS type');
expect(result.node.name == 'Reality Fixture', 'VLESS decoded name');
expect(result.node.uuid == '00000000-0000-4000-8000-000000000001', 'VLESS UUID');
expect(result.node.reality_public_key == 'fixture-public-key', 'Reality public key');
expect(result.node.reality_short_id == '0123456789abcdef', 'Reality short ID');
expect(result.node.packet_encoding == 'xudp', 'VLESS packet encoding');

result = parser.parse('hysteria2://fixture%3Apassword@[2001:db8::1]:443,12000-12010/?insecure=1&obfs=salamander&obfs-password=fixture&sni=hy2.example.com&hopInterval=45s#Hysteria2%20Fixture');
expect(result.node.type == 'hysteria2', 'Hysteria2 type');
expect(result.node.server == '2001:db8::1', 'Hysteria2 IPv6 host');
expect(result.node.password == 'fixture:password', 'Hysteria2 decoded auth');
expect(result.node.server_port == '443', 'Hysteria2 primary port');
expect(JSON.stringify(result.node.server_ports) == JSON.stringify([ '443', '12000:12010' ]),
	'Hysteria2 multi-port conversion');
expect(result.node.insecure == '1', 'Hysteria2 insecure');
expect(result.node.hop_interval == '45s', 'Hysteria2 hop interval');

result = parser.parse('hy2://password@example.com?sni=hy2.example.com');
expect(result.node.server_port == '443', 'Hysteria2 default port');

result = parser.parse('hy2://password@example.com:443?mport=12000-12010%2C13000&sni=hy2.example.com');
expect(result.node.server_port == '443', 'Hysteria2 mport preserves the primary URL port');
expect(JSON.stringify(result.node.server_ports) == JSON.stringify([ '12000:12010', '13000' ]),
	'Hysteria2 mport conversion');

result = parser.parse('trojan://fixture%40password@trojan.example.com:8443?security=tls&sni=edge.example.com&fp=chrome&allowInsecure=0&type=tcp#Trojan%20Fixture');
expect(result.node.type == 'trojan', 'Trojan type');
expect(result.node.password == 'fixture@password', 'Trojan decoded password');
expect(result.node.tls_server_name == 'edge.example.com', 'Trojan SNI');
expect(result.node.utls_fingerprint == 'chrome', 'Trojan fingerprint');

result = parser.parse('ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@example.com:8388#Shadowsocks');
expect(result.node.type == 'shadowsocks' && result.node.method == 'aes-256-gcm', 'Shadowsocks parser');

result = parser.parse('tuic://00000000-0000-4000-8000-000000000001:password@example.com:443?sni=edge.example.com');
expect(result.node.type == 'tuic' && result.node.uuid.startsWith('00000000-'), 'TUIC parser');

result = parser.parse('socks5://user:password@example.com:1080');
expect(result.node.type == 'socks' && result.node.username == 'user', 'SOCKS parser');

expectError('UNSUPPORTED_TRANSPORT', 'vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=example.com&type=ws&path=%2Fws');
expectError('UNSUPPORTED_TRANSPORT', 'trojan://password@example.com:443?type=grpc&sni=example.com');
expectError('UNSUPPORTED_HYSTERIA2_OBFS', 'hy2://password@example.com:443?sni=example.com&obfs=gecko&obfs-password=secret');
expectError('UNSUPPORTED_PARAMETER', 'hy2://password@example.com:443?sni=example.com&pinSHA256=deadbeef');
expectError('UNSUPPORTED_PARAMETER', 'trojan://password@example.com:443?sni=example.com&alpn=h2');
expectError('REQUIRED_PARAMETER', 'vless://00000000-0000-4000-8000-000000000001@example.com:443?security=reality&fp=chrome&pbk=key&sid=id&type=tcp');
expectError('PARAMETER_REQUIRES_REALITY', 'vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=example.com&pbk=key');
expectError('PARAMETER_REQUIRES_OBFS', 'hy2://password@example.com:443?sni=example.com&obfs-password=secret');
expectError('INVALID_PORT', 'hy2://password@example.com:443?sni=example.com&mport=12010-12000');
expectError('CONFLICTING_PARAMETER', 'hy2://password@example.com:443,12000-12010?sni=example.com&mport=13000-13010');
expectError('INVALID_VLESS_UUID', 'vless://not-a-uuid@example.com:443?security=tls&sni=example.com');

console.log('share URL parser tests passed');
