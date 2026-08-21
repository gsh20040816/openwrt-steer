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

const manualNodeGroup = '_manual';

function nodeGroupID(node) {
	return node.source_subscription || manualNodeGroup;
}

function collectNodeGroups(nodes, subscriptions) {
	const groups = [ { id: manualNodeGroup, label: _('Manual nodes'), count: 0 } ];
	const known = { [manualNodeGroup]: groups[0] };
	subscriptions.forEach((subscription) => {
		const id = subscription['.name'];
		const group = { id, label: subscription.name || id, count: 0 };
		known[id] = group;
		groups.push(group);
	});
	nodes.forEach((node) => {
		const id = nodeGroupID(node);
		if (!known[id]) {
			known[id] = { id, label: _('Missing subscription: %s').format(id), count: 0 };
			groups.push(known[id]);
		}
		known[id].count++;
	});
	return groups;
}

function collectNodeReferences(nodes, routes, groups) {
	const known = {};
	const groupOrder = {};
	groups.forEach((group, index) => { groupOrder[group.id] = index; });
	const references = [];
	nodes.slice().sort((left, right) => {
		const group = (groupOrder[nodeGroupID(left)] ?? groups.length) - (groupOrder[nodeGroupID(right)] ?? groups.length);
		return group || String(left.name || left['.name']).localeCompare(String(right.name || right['.name']));
	}).forEach((node) => {
		known[node['.name']] = true;
		const group = groups.find((candidate) => candidate.id == nodeGroupID(node));
		references.push({ id: node['.name'], label: node.name || _('Unnamed'), group: group?.label || nodeGroupID(node) });
	});
	routes.forEach((route) => {
		const node = route.kind == 'single' ? route.node : null;
		if (node && !known[node]) {
			known[node] = true;
			references.push({ id: node, label: _('Missing: %s').format(node), group: _('Missing references') });
		}
	});
	return references;
}

function addNodeValues(option, references) {
	references.forEach((reference) => option.value(reference.id, reference.label, _('Group: %s').format(reference.group)));
}

function nodeReferenceLabel(references, nodeId) {
	if (!nodeId)
		return null;
	const reference = references.find((candidate) => candidate.id == nodeId);
	return reference ? reference.label : _('Missing node');
}

function protocolLabel(value) {
	const labels = {
		socks: 'SOCKS',
		http: 'HTTP CONNECT',
		shadowsocks: 'Shadowsocks',
		vmess: 'VMess',
		vless: 'VLESS',
		hysteria: 'Hysteria',
		hysteria2: 'Hysteria2',
		shadowtls: 'ShadowTLS',
		tuic: 'TUIC',
		anytls: 'AnyTLS',
		naive: 'NaiveProxy',
		ssh: 'SSH',
		tor: 'Tor',
		trojan: 'Trojan'
	};
	return labels[value] || value || null;
}

function selectedNodeGroup(groups) {
	const requested = new URLSearchParams(window.location.search || '').get('node_group');
	return groups.some((group) => group.id == requested) ? requested : manualNodeGroup;
}

function renderNodeGroupNavigation(groups, activeGroup) {
	return E('nav', { 'class': 'steer-node-groups', 'aria-label': _('Node groups') }, groups.map((group) => {
		const query = new URLSearchParams(window.location.search || '');
		query.set('node_group', group.id);
		return E('a', {
			'class': 'steer-node-groups__item' + (group.id == activeGroup ? ' is-active' : ''),
			'href': (window.location.pathname || '') + '?' + query.toString(),
			'aria-current': group.id == activeGroup ? 'page' : null
		}, [ E('span', {}, group.label), E('strong', {}, String(group.count)) ]);
	}));
}

function renderSubscriptionStatus(result) {
	const subscriptions = result?.subscriptions || [];
	return E('section', { 'class': 'steer-subscription-status' }, [
		E('h3', {}, _('Subscription status')),
		subscriptions.length ? E('div', { 'class': 'table' }, [
			E('div', { 'class': 'tr table-titles' }, [ E('div', { 'class': 'th' }, _('Name')), E('div', { 'class': 'th' }, _('Last update')), E('div', { 'class': 'th' }, _('Nodes')), E('div', { 'class': 'th' }, _('Action')) ]),
			...subscriptions.map((subscription) => {
				const stale = subscription.stale_node_ids || [];
				const last = subscription.fetched_at ? new Date(subscription.fetched_at).toLocaleString() : (subscription.error || _('Not fetched'));
				const actions = [ E('button', {
					'class': 'btn cbi-button-action',
					'click': function() {
						ui.showModal(_('Updating subscription'), [ E('p', { 'class': 'spinning' }, _('Downloading and validating every node.')) ]);
						return steer.updateSubscription(subscription.id).then((update) => {
							ui.hideModal();
							if (!update?.ok) {
								ui.addNotification(_('Subscription update failed'), E('p', {}, update?.error || _('Unknown error')), 'danger');
								return update;
							}
							ui.addNotification(null, E('p', {}, _('Subscription updated.')), 'info');
							window.location.reload();
							return update;
						});
					}
				}, _('Update now')) ];
				stale.forEach((node) => actions.push(' ', E('button', { 'class': 'btn cbi-button-negative', 'click': function() { return steer.cleanSubscription(subscription.id, node).then(() => window.location.reload()); } }, _('Remove %s').format(node))));
				return E('div', { 'class': 'tr' }, [
					E('div', { 'class': 'td' }, subscription.name || subscription.id),
					E('div', { 'class': 'td' }, last),
					E('div', { 'class': 'td' }, stale.length ? _('%d (%d stale)').format(subscription.node_count, stale.length) : String(subscription.node_count || 0)),
					E('div', { 'class': 'td' }, actions)
				]);
			})
		]) : E('p', {}, _('No subscriptions configured.'))
	]);
}

function importErrorText(error) {
	const detail = error?.detail ? ' (%s)'.format(error.detail) : '';
	const messages = {
		EMPTY_URL: _('Paste one share URL.'),
		INVALID_URL: _('The share URL is malformed.'),
		INVALID_ENCODING: _('The share URL contains invalid percent encoding.'),
		INVALID_BASE64: _('The share URL contains invalid Base64.'),
		INVALID_CREDENTIAL: _('The share URL contains invalid credentials.'),
		INVALID_VMESS_JSON: _('The VMess Base64 payload is not valid JSON.'),
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
				E('p', {}, _('Parse a standard proxy share URL in this browser. The URL is never sent to a Steer RPC or written to logs.'))
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

function runSpeedtest(sectionId, download) {
	const testTitle = download ? _('Download test') : _('Connection test');
	ui.showModal(testTitle, [ E('p', { 'class': 'spinning' }, _('Starting a temporary sing-box instance.')) ]);
	return steer.speedtest(sectionId, download).then((report) => {
		const results = report?.results || [];
		ui.showModal(testTitle, [
			results.length ? E('div', { 'class': 'table' }, [
				E('div', { 'class': 'tr table-titles' }, [ E('div', { 'class': 'th' }, _('URL')), E('div', { 'class': 'th' }, _('Connect')), E('div', { 'class': 'th' }, _('TLS')), E('div', { 'class': 'th' }, _('First byte')), E('div', { 'class': 'th' }, _('Download')) ]),
				...results.map((result) => E('div', { 'class': 'tr' }, [
					E('div', { 'class': 'td' }, result.url),
					E('div', { 'class': 'td' }, result.error || _('%d ms').format(result.connect_milliseconds || 0)),
					E('div', { 'class': 'td' }, result.error ? '-' : _('%d ms').format(result.tls_milliseconds || 0)),
					E('div', { 'class': 'td' }, result.error ? '-' : _('%d ms').format(result.first_byte_milliseconds || 0)),
					E('div', { 'class': 'td' }, result.error ? '-' : _('%d bytes / %d ms').format(result.downloaded_bytes || 0, result.download_milliseconds || 0))
				]))
			]) : E('div', { 'class': 'alert-message danger' }, report?.error || _('No speed-test result was returned.')),
			E('div', { 'class': 'right' }, E('button', { 'class': 'btn', 'click': ui.hideModal }, _('Close')))
		]);
	}).catch((error) => {
		ui.showModal(_('Node speed test failed'), [ E('p', {}, String(error)), E('div', { 'class': 'right' }, E('button', { 'class': 'btn', 'click': ui.hideModal }, _('Close'))) ]);
	});
}

function showImportDialog() {
	const input = E('textarea', {
		'class': 'cbi-input-textarea',
		'rows': 5,
		'autocomplete': 'off',
		'spellcheck': 'false',
			'placeholder': 'vless://…  vmess://…  ss://…  hysteria2://…  tuic://…  trojan://…'
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
		return Promise.all([ uci.load('steer'), steer.subscriptions() ]);
	},

	render: function(data) {
		let m, s, o;
		const nodes = uci.sections('steer', 'node');
		const routes = uci.sections('steer', 'route');
		const subscriptions = uci.sections('steer', 'subscription');
		const nodeGroups = collectNodeGroups(nodes, subscriptions);
		const activeNodeGroup = selectedNodeGroup(nodeGroups);
		const activeGroup = nodeGroups.find((group) => group.id == activeNodeGroup);
		const nodeReferences = collectNodeReferences(nodes, routes, nodeGroups);
		const summaryOnly = activeNodeGroup != manualNodeGroup;
		steer.loadStyle();

		m = new form.Map('steer', _('Nodes & Routes'));

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
			o = s.option(form.RichListValue, 'node', _('Node'));
			o.depends('kind', 'single');
			o.rmempty = false;
			addNodeValues(o, nodeReferences);
			o.textvalue = function(sectionId) {
				return nodeReferenceLabel(nodeReferences, uci.get('steer', sectionId, 'node'));
			};
		}

		s = m.section(form.GridSection, 'node', _('Proxy nodes — %s (%d)').format(activeGroup.label, activeGroup.count));
		s.anonymous = true;
		s.addremove = activeNodeGroup == manualNodeGroup;
		/* Subscription nodes are generated data; avoid building editable widgets for every row. */
		s.readonly = summaryOnly;
		s.nodescriptions = true;
		s.addbtntitle = _('Add proxy node');
		s.filter = function(sectionId) {
			return nodeGroupID(uci.get('steer', sectionId)) == activeNodeGroup;
		};
		s.sectiontitle = function(sectionId) {
			return uci.get('steer', sectionId, 'name') || _('Unnamed');
		};
		s.tab('general', _('Connection'));
		s.tab('tls', _('TLS / REALITY'));
		s.tab('protocol', _('Protocol'));

		o = s.taboption('general', form.Flag, 'enabled', _('Enabled'));
		o.default = '1';
		o.editable = !summaryOnly;

		o = s.taboption('general', form.Button, '_connect_speedtest', _('Connection test'));
		o.default = '1';
		o.editable = true;
		o.inputtitle = _('Test');
		o.inputstyle = 'action';
		o.onclick = function(sectionId) { return runSpeedtest(sectionId, false); };

		o = s.taboption('general', form.Button, '_download_speedtest', _('Download test'));
		o.default = '1';
		o.editable = true;
		o.inputtitle = _('Test');
		o.inputstyle = 'action';
		o.onclick = function(sectionId) { return runSpeedtest(sectionId, true); };

		o = s.taboption('general', form.Value, 'name', _('Name'));
		o.rmempty = false;
		o.modalonly = true;

		o = s.taboption('general', form.ListValue, 'type', _('Protocol'));
		o.value('socks', 'SOCKS');
		o.value('http', 'HTTP CONNECT');
		o.value('shadowsocks', 'Shadowsocks');
		o.value('vmess', 'VMess');
		o.value('vless', 'VLESS');
		o.value('hysteria', 'Hysteria');
		o.value('hysteria2', 'Hysteria2');
		o.value('shadowtls', 'ShadowTLS');
		o.value('tuic', 'TUIC');
		o.value('anytls', 'AnyTLS');
		o.value('naive', 'NaiveProxy');
		o.value('ssh', 'SSH');
		o.value('tor', 'Tor');
		o.value('trojan', 'Trojan');
		o.rmempty = false;
		o.editable = !summaryOnly;
		o.textvalue = function(sectionId) {
			return protocolLabel(uci.get('steer', sectionId, 'type'));
		};

		o = s.taboption('general', form.Value, 'server', _('Server'));
		o.rmempty = false;
		o.editable = !summaryOnly;

		o = s.taboption('general', form.Value, 'server_port', _('Port'));
		o.datatype = 'port';
		o.rmempty = false;
		o.editable = !summaryOnly;

		o = s.taboption('protocol', form.Value, 'uuid', _('UUID'));
		o.modalonly = true;
		o.depends('type', 'vless');
		o.depends('type', 'vmess');
		o.depends('type', 'tuic');

		o = s.taboption('protocol', form.Value, 'username', _('Username'));
		o.modalonly = true;
		o.depends('type', 'socks');
		o.depends('type', 'http');
		o.depends('type', 'naive');
		o.depends('type', 'ssh');

		o = s.taboption('protocol', form.Value, 'method', 'Method');
		o.modalonly = true;
		o.depends('type', 'shadowsocks');

		o = s.taboption('protocol', form.Value, 'plugin', 'Plugin');
		o.modalonly = true;
		o.depends('type', 'shadowsocks');

		o = s.taboption('protocol', form.Value, 'plugin_options', 'Plugin options');
		o.modalonly = true;
		o.depends('type', 'shadowsocks');

		o = s.taboption('protocol', form.ListValue, 'security', 'Security');
		o.value('', _('Default'));
		o.value('auto', 'auto');
		o.value('none', 'none');
		o.value('zero', 'zero');
		o.value('aes-128-gcm', 'aes-128-gcm');
		o.value('chacha20-poly1305', 'chacha20-poly1305');
		o.depends('type', 'vmess');

		o = s.taboption('protocol', form.Value, 'alter_id', 'Alter ID');
		o.datatype = 'uinteger';
		o.modalonly = true;
		o.depends('type', 'vmess');

		o = s.taboption('protocol', form.ListValue, 'transport', _('Transport'));
		o.value('tcp', 'TCP');
		o.value('ws', 'WebSocket');
		o.value('grpc', 'gRPC');
		o.value('http', 'HTTP');
		o.value('quic', 'QUIC');
		o.modalonly = true;
		o.depends('type', 'vmess');
		o.depends('type', 'vless');
		o.depends('type', 'trojan');

		o = s.taboption('protocol', form.Value, 'transport_path', 'Transport path');
		o.modalonly = true;
		o.depends('transport', 'ws');
		o.depends('transport', 'http');

		o = s.taboption('protocol', form.Value, 'transport_host', 'Transport host');
		o.modalonly = true;
		o.depends('transport', 'ws');
		o.depends('transport', 'http');

		o = s.taboption('protocol', form.Value, 'service_name', 'gRPC service name');
		o.modalonly = true;
		o.depends('transport', 'grpc');

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
		o.depends('type', 'shadowsocks');
		o.depends('type', 'http');
		o.depends('type', 'anytls');
		o.depends('type', 'shadowtls');
		o.depends('type', 'tuic');
		o.depends('type', 'hysteria');
		o.depends('type', 'naive');
		o.depends('type', 'ssh');

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
		o.depends('type', 'hysteria');

		o = s.taboption('protocol', form.ListValue, 'version', 'Version');
		o.value('1', '1'); o.value('2', '2'); o.value('3', '3');
		o.modalonly = true;
		o.depends('type', 'shadowtls');

		o = s.taboption('protocol', form.ListValue, 'congestion_control', 'Congestion control');
		o.value('', _('Default')); o.value('cubic', 'cubic'); o.value('new_reno', 'new_reno'); o.value('bbr', 'bbr');
		o.modalonly = true;
		o.depends('type', 'tuic');

		o = s.taboption('protocol', form.ListValue, 'udp_relay_mode', 'UDP relay mode');
		o.value('', _('Default')); o.value('native', 'native'); o.value('quic', 'quic');
		o.modalonly = true;
		o.depends('type', 'tuic');

		o = s.taboption('protocol', form.Flag, 'udp_over_stream', 'UDP over stream');
		o.modalonly = true;
		o.depends('type', 'tuic');

		o = s.taboption('protocol', form.Flag, 'zero_rtt_handshake', '0-RTT handshake');
		o.modalonly = true;
		o.depends('type', 'tuic');

		o = s.taboption('protocol', form.Value, 'heartbeat', 'Heartbeat');
		o.modalonly = true;
		o.depends('type', 'tuic');

		o = s.taboption('protocol', form.Flag, 'quic', 'QUIC');
		o.modalonly = true;
		o.depends('type', 'naive');

		o = s.taboption('protocol', form.ListValue, 'quic_congestion_control', 'QUIC congestion control');
		o.value('', _('Default')); o.value('bbr', 'bbr'); o.value('bbr2', 'bbr2'); o.value('cubic', 'cubic'); o.value('reno', 'reno');
		o.modalonly = true;
		o.depends('type', 'naive');

		o = s.taboption('protocol', form.Value, 'insecure_concurrency', 'Insecure concurrency');
		o.datatype = 'uinteger';
		o.modalonly = true;
		o.depends('type', 'naive');

		o = s.taboption('protocol', form.Value, 'private_key', 'Private key');
		o.modalonly = true;
		o.depends('type', 'ssh');

		o = s.taboption('protocol', form.Value, 'host_key', 'Host key');
		o.modalonly = true;
		o.depends('type', 'ssh');

		o = s.taboption('protocol', form.DynamicList, 'host_key_algorithms', 'Host key algorithms');
		o.modalonly = true;
		o.depends('type', 'ssh');

		o = s.taboption('protocol', form.Value, 'executable_path', 'Executable path');
		o.modalonly = true;
		o.depends('type', 'tor');

		o = s.taboption('protocol', form.DynamicList, 'extra_args', 'Extra arguments');
		o.modalonly = true;
		o.depends('type', 'tor');

		o = s.taboption('protocol', form.Value, 'data_directory', 'Data directory');
		o.modalonly = true;
		o.depends('type', 'tor');

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

		return m.render().then((formNode) => {
			const contents = [ renderSubscriptionStatus(data?.[1]), renderNodeGroupNavigation(nodeGroups, activeNodeGroup) ];
			if (activeNodeGroup == manualNodeGroup)
				contents.push(renderImportButton());
			contents.push(formNode);
			return E([], contents);
		});
	},

	handleSaveApply: function(ev, mode) {
		return steer.apply(this, ev, mode);
	}
});
