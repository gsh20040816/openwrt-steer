/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

'use strict';
'require form';
'require uci';
'require view';
'require steer as steer';
'require steer.ui-spec as uiSpec';

function parseIPv4Literal(value) {
	const pieces = String(value).split('.');
	if (pieces.length != 4)
		return null;
	const octets = [];
	for (const piece of pieces) {
		if (!/^(0|[1-9]\d{0,2})$/.test(piece) || Number(piece) > 255)
			return null;
		octets.push(Number(piece));
	}
	return octets;
}

function parseIPv6Literal(value) {
	let address = String(value);
	const zoneIndex = address.indexOf('%');
	if (zoneIndex >= 0) {
		if (zoneIndex == 0 || zoneIndex == address.length - 1 || address.indexOf('%', zoneIndex + 1) >= 0)
			return null;
		address = address.slice(0, zoneIndex);
	}
	if (!address.includes(':') || address.indexOf('::') != address.lastIndexOf('::'))
		return null;

	const compressed = address.includes('::');
	const halves = compressed ? address.split('::') : [ address ];
	const parseWords = function(part, allowIPv4Tail) {
		if (!part)
			return [];
		const tokens = part.split(':');
		const words = [];
		for (let index = 0; index < tokens.length; index++) {
			const token = tokens[index];
			if (token.includes('.')) {
				if (!allowIPv4Tail || index != tokens.length - 1)
					return null;
				const octets = parseIPv4Literal(token);
				if (!octets)
					return null;
				words.push((octets[0] << 8) | octets[1], (octets[2] << 8) | octets[3]);
			}
			else {
				if (!/^[0-9a-f]{1,4}$/i.test(token))
					return null;
				words.push(Number.parseInt(token, 16));
			}
		}
		return words;
	};

	const left = parseWords(halves[0], !compressed);
	const right = compressed ? parseWords(halves[1], true) : [];
	if (!left || !right)
		return null;
	if (!compressed)
		return left.length == 8 ? left : null;
	const omitted = 8 - left.length - right.length;
	return omitted >= 1 ? [ ...left, ...Array(omitted).fill(0), ...right ] : null;
}

function classifyLocalProxyListen(value) {
	const ipv4 = parseIPv4Literal(value);
	if (ipv4)
		return ipv4[0] == 127 ? 'loopback' : 'non_loopback';
	const ipv6 = parseIPv6Literal(value);
	if (!ipv6)
		return 'invalid';
	return ipv6.slice(0, 7).every((word) => word == 0) && ipv6[7] == 1 ? 'loopback' : 'non_loopback';
}

function validateLocalProxyForm(listen, username, password) {
	const classification = classifyLocalProxyListen(listen);
	if (classification == 'invalid')
		return _('Listen address must be an IP literal.');
	if ((username == '') != (password == ''))
		return _('Username and password must either both be set or both be empty.');
	if (classification == 'non_loopback' && username == '')
		return _('Non-loopback listeners are exposed beyond this device and require username/password authentication.');
	return true;
}

return view.extend({
	classifyLocalProxyListen: classifyLocalProxyListen,
	validateLocalProxyForm: validateLocalProxyForm,

	load: function() {
		return uci.load('steer');
	},

	render: function() {
		let m, s, o, listenOption, usernameOption, passwordOption;
		steer.loadStyle();

		m = new form.Map('steer', _('Local proxies'));

		s = m.section(form.GridSection, 'local_proxy', _('Named proxy entry points'));
		steer.configureNamedSection(s, steer.creationDefaults('local_proxies'));
		steer.configureRemovalGuard(s, (sectionId) => steer.collectionReferences('local_proxies', sectionId),
			_('Local proxy is still referenced'));
		s.addremove = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add local proxy');
		s.sectiontitle = function(sectionId) {
			return uci.get('steer', sectionId, 'name') || _('Unnamed');
		};

		o = s.option(form.Flag, 'enabled', _('Enabled'));
		o.default = '1';
		o.editable = true;

		o = s.option(form.Value, 'name', _('Name'));
		o.rmempty = true;
		o.optional = true;
		o.modalonly = true;

		o = s.option(form.ListValue, 'protocol', _('Protocol'));
		uiSpec.local_proxy_protocols.forEach((item) => o.value(item.value, steer.uiSpecLabel(item.label)));
		o.rmempty = false;
		o.editable = true;

		listenOption = o = s.option(form.Value, 'listen', _('Listen address'));
		o.datatype = 'ipaddr';
		o.placeholder = '127.0.0.1';
		o.rmempty = false;
		o.editable = true;
		o.description = E('strong', { 'class': 'steer-exposure-warning' },
			_('Exposure warning: non-loopback listeners may be reachable from LAN or public networks and require authentication.'));

		o = s.option(form.Value, 'listen_port', _('Port'));
		o.datatype = 'port';
		o.rmempty = false;
		o.editable = true;

		usernameOption = o = s.option(form.Value, 'username', _('Username'));
		o.modalonly = true;

		passwordOption = o = s.option(form.Value, 'password', _('Password'));
		o.password = true;
		o.modalonly = true;
		o.description = _('Username and password must either both be set or both be empty.');

		const formValue = function(option, sectionId) {
			return String(option.formvalue(sectionId) ?? '');
		};
		listenOption.validate = function(sectionId, value) {
			return validateLocalProxyForm(String(value ?? ''), formValue(usernameOption, sectionId), formValue(passwordOption, sectionId));
		};
		usernameOption.validate = function(sectionId, value) {
			return validateLocalProxyForm(formValue(listenOption, sectionId), String(value ?? ''), formValue(passwordOption, sectionId));
		};
		passwordOption.validate = function(sectionId, value) {
			return validateLocalProxyForm(formValue(listenOption, sectionId), formValue(usernameOption, sectionId), String(value ?? ''));
		};

		return m.render().then((formNode) => steer.focusSection(s, 'local_proxy').then(() => formNode));
	},

	handleSaveApply: function(ev, mode) {
		return steer.apply(this, ev, mode);
	}
});
