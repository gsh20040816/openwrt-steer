/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

'use strict';
'require form';
'require uci';
'require ui';
'require view';
'require steer as steer';
'require steer.share-url as shareUrl';

function collectNodeReferences(nodes, routes) {
	const known = {};
	const references = [];
	nodes.forEach((node) => {
		known[node['.name']] = true;
		references.push([ node['.name'], node.name || node['.name'] ]);
	});
	routes.forEach((route) => {
		const node = route.kind == 'single' ? route.node : null;
		if (node && !known[node]) {
			known[node] = true;
			references.push([ node, _('Missing: %s').format(node) ]);
		}
	});
	return references;
}

function addNodeValues(option, references) {
	references.forEach((reference) => option.value(reference[0], reference[1]));
}

function importErrorText(error) {
	const detail = error?.detail ? ' (%s)'.format(error.detail) : '';
	const messages = {
		EMPTY_URL: _('Paste one share URL.'),
		INVALID_URL: _('The share URL is malformed.'),
		INVALID_ENCODING: _('The share URL contains invalid percent encoding.'),
		UNSUPPORTED_SCHEME: _('This share URL protocol is not supported.'),
		MISSING_CREDENTIAL: _('The share URL does not contain credentials.'),
		INVALID_HOST: _('The share URL contains an invalid server address.'),
		INVALID_AUTHORITY: _('The share URL contains an invalid server and port.'),
		IPV6_BRACKETS_REQUIRED: _('An IPv6 server address must be enclosed in brackets.'),
		INVALID_PORT: _('The share URL contains an invalid port or port range.'),
		UNSUPPORTED_PATH: _('This share URL uses an unsupported path.'),
		CONFLICTING_PARAMETER: _('The share URL contains conflicting parameter aliases.'),
		INVALID_BOOLEAN: _('The share URL contains an invalid boolean parameter.'),
		INVALID_INTEGER: _('The share URL contains an invalid positive integer.'),
		UNSUPPORTED_PARAMETER: _('The share URL contains a parameter that Steer cannot preserve.'),
		REQUIRED_PARAMETER: _('The share URL is missing a required parameter.'),
		UNSUPPORTED_TRANSPORT: _('Steer cannot preserve this transport type.'),
		UNSUPPORTED_HEADER: _('Steer cannot preserve this transport header.'),
		UNSUPPORTED_ENCRYPTION: _('Steer cannot preserve this VLESS encryption mode.'),
		UNSUPPORTED_SECURITY: _('Steer cannot preserve this security mode.'),
		UNSUPPORTED_FLOW: _('Steer cannot preserve this VLESS flow.'),
		UNSUPPORTED_PACKET_ENCODING: _('Steer cannot preserve this UDP packet encoding.'),
		UNSUPPORTED_HYSTERIA2_OBFS: _('Steer currently supports only Hysteria2 Salamander obfuscation.'),
		INVALID_VLESS_UUID: _('The VLESS credential is not a UUID.'),
		PARAMETER_REQUIRES_REALITY: _('This parameter requires REALITY security.'),
		PARAMETER_REQUIRES_TLS: _('This parameter requires TLS or REALITY security.'),
		PARAMETER_REQUIRES_OBFS: _('This parameter requires Hysteria2 obfuscation.')
	};
	return (messages[error?.code] || _('The share URL could not be parsed.')) + detail;
}

function previewFacts(node) {
	const values = [
		[ _('Protocol'), node.type ],
		[ _('Server'), node.server ],
		[ _('Port'), node.server_port ],
		[ _('Credentials'), _('Present; hidden from preview') ]
	];
	if (node.server_ports?.length)
		values.push([ _('Port hopping ranges'), node.server_ports.join(', ') ]);
	if (node.tls_server_name)
		values.push([ _('TLS server name'), node.tls_server_name ]);
	if (node.flow)
		values.push([ _('Flow'), node.flow ]);
	if (node.packet_encoding)
		values.push([ _('UDP packet encoding'), node.packet_encoding ]);
	if (node.utls_fingerprint)
		values.push([ _('uTLS fingerprint'), node.utls_fingerprint ]);
	if (node.reality_public_key)
		values.push([ _('REALITY parameters'), _('Present; hidden from preview') ]);
	if (node.obfs_type)
		values.push([ _('Obfuscation'), node.obfs_type ]);
	if (node.hop_interval)
		values.push([ _('Port hopping interval'), node.hop_interval ]);
	if (node.up_mbps)
		values.push([ _('Upload Mbps'), node.up_mbps ]);
	if (node.down_mbps)
		values.push([ _('Download Mbps'), node.down_mbps ]);
	values.push([ _('Certificate verification'), node.insecure == '1' ? _('Disabled') : _('Enabled') ]);
	return values;
}

function renderImportButton() {
	return E('section', { 'class': 'cbi-section' }, [
		E('div', { 'class': 'steer-section-heading' }, [
			E('div', {}, [
				E('h3', {}, _('Import share URL')),
				E('p', {}, _('Parse one VLESS, Hysteria2 or Trojan share URL in this browser. The URL is never sent to a Steer RPC or written to logs.'))
			]),
			E('button', {
				'class': 'cbi-button cbi-button-add',
				click: function(ev) {
					ev.preventDefault();
					showImportDialog();
				}
			}, _('Import node'))
		])
	]);
}

function showImportDialog() {
	const input = E('textarea', {
		'class': 'cbi-input-textarea',
		'rows': 5,
		'autocomplete': 'off',
		'spellcheck': 'false',
		'placeholder': 'vless://…  hysteria2://…  hy2://…  trojan://…'
	});
	const preview = E('div', { 'class': 'steer-import-preview' });

	const review = function(ev) {
		ev.preventDefault();
		let parsed;
		try {
			parsed = shareUrl.parse(input.value);
		}
		catch (error) {
			preview.replaceChildren(E('div', { 'class': 'alert-message danger' }, importErrorText(error)));
			return;
		}

		input.value = '';
		const node = parsed.node;
		const nameInput = E('input', {
			'class': 'cbi-input-text',
			'value': node.name,
			'autocomplete': 'off'
		});
		const warnings = (parsed.warnings || []).map((warning) =>
			E('li', {}, _('Ignored compatibility-only parameter: %s').format(warning.detail)));

		const save = function(saveEvent) {
			saveEvent.preventDefault();
			node.name = nameInput.value.trim() || node.name;
			const id = uci.add('steer', 'node');
			Object.keys(node).forEach((option) => uci.set('steer', id, option, node[option]));
			saveEvent.currentTarget.disabled = true;
			return uci.save().then(() => {
				ui.hideModal();
				window.location.href = L.url('admin/services/steer/nodes');
			}).catch((error) => {
				saveEvent.currentTarget.disabled = false;
				ui.addNotification(_('Node import failed'), E('p', {}, String(error)), 'danger');
			});
		};

		preview.replaceChildren(E('div', {}, [
			E('h4', {}, _('Review imported node')),
			...(warnings.length ? [ E('div', { 'class': 'alert-message warning' }, [
				E('strong', {}, _('Review parser warnings')),
				E('ul', {}, warnings)
			]) ] : []),
			E('div', { 'class': 'cbi-value' }, [
				E('label', { 'class': 'cbi-value-title' }, _('Name')),
				E('div', { 'class': 'cbi-value-field' }, nameInput)
			]),
			E('dl', { 'class': 'steer-status__facts' }, previewFacts(node).map((fact) =>
				E('div', {}, [ E('dt', {}, fact[0]), E('dd', {}, String(fact[1])) ]))),
			E('p', {}, _('Unsupported transport or security fields stop the import instead of being silently discarded.')),
			E('div', { 'class': 'right' }, E('button', {
				'class': 'cbi-button cbi-button-positive',
				click: save
			}, _('Add to pending changes')))
		]));
	};

	ui.showModal(_('Import proxy node'), [
		E('p', {}, _('Paste exactly one share URL. Credentials are hidden from the preview and are stored only after you confirm.')),
		input,
		preview,
		E('div', { 'class': 'right' }, [
			E('button', { 'class': 'cbi-button', 'click': ui.hideModal }, _('Cancel')),
			' ',
			E('button', { 'class': 'cbi-button cbi-button-action', 'click': review }, _('Parse and review'))
		])
	]);
	input.focus();
}

return view.extend({
	load: function() {
		return uci.load('steer');
	},

	render: function() {
		let m, s, o;
		const nodes = uci.sections('steer', 'node');
		const routes = uci.sections('steer', 'route');
		const nodeReferences = collectNodeReferences(nodes, routes);
		steer.loadStyle();

		m = new form.Map('steer', _('Nodes & Routes'),
			_('Nodes describe transport credentials. Rules never point at a node directly; they point at a named route so subscriptions and failover can evolve without rewriting policy.'));

		s = m.section(form.GridSection, 'route', _('Routes'));
		s.anonymous = true;
		s.addremove = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add route');
		s.sectiontitle = function(sectionId) {
			return uci.get('steer', sectionId, 'name') || _('Unnamed');
		};

		o = s.option(form.Flag, 'enabled', _('Enabled'));
		o.default = '1';
		o.editable = true;

		o = s.option(form.Value, 'name', _('Name'));
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.ListValue, 'kind', _('Kind'));
		o.value('direct', _('Direct'));
		o.value('block', _('Block'));
		if (nodeReferences.length)
			o.value('single', _('Single node'));
		else
			o.description = _('Create a proxy node before adding a single-node route.');
		o.rmempty = false;
		o.editable = true;

		if (nodeReferences.length) {
			o = s.option(form.ListValue, 'node', _('Node'));
			o.depends('kind', 'single');
			o.rmempty = false;
			addNodeValues(o, nodeReferences);
		}

		s = m.section(form.GridSection, 'node', _('Proxy nodes'));
		s.anonymous = true;
		s.addremove = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add proxy node');
		s.sectiontitle = function(sectionId) {
			return uci.get('steer', sectionId, 'name') || _('Unnamed');
		};
		s.tab('general', _('Connection'));
		s.tab('tls', _('TLS / REALITY'));
		s.tab('protocol', _('Protocol'));

		o = s.taboption('general', form.Flag, 'enabled', _('Enabled'));
		o.default = '1';
		o.editable = true;

		o = s.taboption('general', form.Value, 'name', _('Name'));
		o.rmempty = false;
		o.modalonly = true;

		o = s.taboption('general', form.ListValue, 'type', _('Protocol'));
		o.value('vless', 'VLESS');
		o.value('hysteria2', 'Hysteria2');
		o.value('trojan', 'Trojan');
		o.rmempty = false;
		o.editable = true;

		o = s.taboption('general', form.Value, 'server', _('Server'));
		o.rmempty = false;
		o.editable = true;

		o = s.taboption('general', form.Value, 'server_port', _('Port'));
		o.datatype = 'port';
		o.rmempty = false;
		o.editable = true;

		o = s.taboption('protocol', form.Value, 'uuid', _('UUID'));
		o.modalonly = true;
		o.depends('type', 'vless');

		o = s.taboption('protocol', form.ListValue, 'flow', _('Flow'));
		o.modalonly = true;
		o.value('', _('None'));
		o.value('xtls-rprx-vision', 'XTLS Vision');
		o.depends('type', 'vless');

		o = s.taboption('protocol', form.ListValue, 'packet_encoding', _('UDP packet encoding'));
		o.modalonly = true;
		o.value('xudp', 'XUDP');
		o.value('packetaddr', 'PacketAddr');
		o.depends('type', 'vless');

		o = s.taboption('protocol', form.Value, 'password', _('Password'));
		o.password = true;
		o.modalonly = true;
		o.depends('type', 'hysteria2');
		o.depends('type', 'trojan');

		o = s.taboption('protocol', form.DynamicList, 'server_ports', _('Port hopping ranges'));
		o.placeholder = '20000:21000';
		o.modalonly = true;
		o.depends('type', 'hysteria2');

		o = s.taboption('protocol', form.Value, 'hop_interval', _('Port hopping interval'));
		o.placeholder = '30s';
		o.modalonly = true;
		o.depends('type', 'hysteria2');

		o = s.taboption('protocol', form.ListValue, 'obfs_type', _('Obfuscation'));
		o.modalonly = true;
		o.value('', _('None'));
		o.value('salamander', 'Salamander');
		o.depends('type', 'hysteria2');

		o = s.taboption('protocol', form.Value, 'obfs_password', _('Obfuscation password'));
		o.password = true;
		o.modalonly = true;
		o.depends('obfs_type', 'salamander');

		o = s.taboption('protocol', form.Value, 'up_mbps', _('Upload Mbps'));
		o.datatype = 'uinteger';
		o.modalonly = true;
		o.depends('type', 'hysteria2');

		o = s.taboption('protocol', form.Value, 'down_mbps', _('Download Mbps'));
		o.datatype = 'uinteger';
		o.modalonly = true;
		o.depends('type', 'hysteria2');

		o = s.taboption('tls', form.Value, 'tls_server_name', _('TLS server name'));
		o.modalonly = true;

		o = s.taboption('tls', form.Flag, 'insecure', _('Skip certificate verification'));
		o.default = '0';
		o.modalonly = true;

		o = s.taboption('tls', form.Value, 'reality_public_key', _('REALITY public key'));
		o.modalonly = true;
		o.depends('type', 'vless');

		o = s.taboption('tls', form.Value, 'reality_short_id', _('REALITY short ID'));
		o.modalonly = true;
		o.depends('type', 'vless');

		o = s.taboption('tls', form.Value, 'utls_fingerprint', _('uTLS fingerprint'));
		o.placeholder = 'chrome';
		o.modalonly = true;
		o.depends('type', 'vless');
		o.depends('type', 'trojan');

		return m.render().then((formNode) => E([], [ renderImportButton(), formNode ]));
	},

	handleSaveApply: function(ev, mode) {
		return steer.apply(this, ev, mode);
	}
});
