/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

'use strict';
'require form';
'require uci';
'require ui';
'require view';
'require steer as steer';
'require steer.ui-spec as uiSpec';

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

function collectRouteReferences(routes) {
	const references = routes.filter((route) => route.kind == 'single').map((route) => ({
		id: route['.name'],
		label: route.name || route['.name']
	}));
	const known = Object.fromEntries(references.map((reference) => [ reference.id, true ]));
	routes.forEach((route) => {
		if (route.detour && !known[route.detour]) {
			known[route.detour] = true;
			references.push({ id: route.detour, label: _('Missing: %s').format(route.detour) });
		}
	});
	return references;
}

function addSystemRouteSection(map, route) {
	if (!route)
		return;
	const direct = route.kind == 'direct';
	let section = map.section(form.NamedSection, route['.name'], 'route', direct ? _('Direct') : _('Block'));
	section.addremove = false;
	section.anonymous = true;
	section.nodescriptions = true;
	let option;
	if (direct) {
		option = section.option(form.DummyValue, '_system_status', _('Status'));
		option.textvalue = function() { return _('Required · always enabled'); };
	}
	else {
		option = section.option(form.Flag, 'enabled', _('Enabled'));
		option.default = '1';
	}
	option = section.option(form.Value, 'name', _('Name'));
	option.rmempty = false;
	option = section.option(form.DummyValue, '_system_kind', _('Kind'));
	option.textvalue = function() { return direct ? _('Direct') : _('Block'); };
}

function configureSubscriptionRemoval(section, nodes, routes) {
	section.handleRemove = function(sectionId) {
		const owned = nodes.filter((node) => node.source_subscription == sectionId);
		const ownedIDs = Object.fromEntries(owned.map((node) => [ node['.name'], true ]));
		const references = routes.filter((route) => route.node && ownedIDs[route.node]);
		if (references.length) {
			ui.addNotification(_('Subscription cannot be removed'), E('p', {},
				_('%d route(s) still use nodes from this subscription.').format(references.length)), 'danger');
			return;
		}
		const config = this.uciconfig || this.map.config;
		owned.forEach((node) => this.map.data.remove(config, node['.name']));
		this.map.data.remove(config, sectionId);
		return this.map.save(null, true);
	};
}

function protocolLabel(value) {
	return uiSpec.node_types.find((item) => item.value == value)?.label || value || null;
}

function nodeFieldLabel(field) {
	const labels = {
		uuid: _('UUID'), username: _('Username'), password: _('Password'), method: 'Method', plugin: 'Plugin',
		plugin_options: 'Plugin options', security: 'Security', alter_id: 'Alter ID', network: 'Network',
		packet_encoding: _('UDP packet encoding'), flow: _('Flow'), transport: _('Transport'),
		transport_path: 'Transport path', transport_host: 'Transport host', service_name: 'gRPC service name',
		server_ports: _('Port hopping ranges'), hop_interval: _('Port hopping interval'), obfs_type: _('Obfuscation'),
		obfs_password: _('Obfuscation password'), up_mbps: _('Upload Mbps'), down_mbps: _('Download Mbps'),
		version: 'Version', congestion_control: 'Congestion control', udp_relay_mode: 'UDP relay mode',
		udp_over_stream: 'UDP over stream', zero_rtt_handshake: '0-RTT handshake', heartbeat: 'Heartbeat',
		quic: 'QUIC', quic_congestion_control: 'QUIC congestion control', insecure_concurrency: 'Insecure concurrency',
		private_key: 'Private key', host_key: 'Host key', host_key_algorithms: 'Host key algorithms',
		executable_path: 'Executable path', extra_args: 'Extra arguments', data_directory: 'Data directory',
		tls_server_name: _('TLS server name'), insecure: _('Skip certificate verification'),
		reality_public_key: _('REALITY public key'), reality_short_id: _('REALITY short ID'),
		utls_fingerprint: _('uTLS fingerprint')
	};
	return labels[field.key] || field.label;
}

function nodeChoiceLabel(item) {
	const labels = {
		'': _('Default'), 'None': _('None'), 'Default': _('Default'),
		'System default': _('System default')
	};
	return labels[item.label] || item.label;
}

function addGeneratedNodeField(section, field) {
	let widget = form.Value;
	if (field.control == 'boolean')
		widget = form.Flag;
	else if (field.control == 'select' || field.control == 'select-integer')
		widget = form.ListValue;
	else if (field.control == 'string-list')
		widget = form.DynamicList;
	else if (field.multiline)
		widget = form.TextValue;

	const option = section.option(widget, field.key, nodeFieldLabel(field));
	option.modalonly = true;
	if (field.control == 'password')
		option.password = true;
	if (field.control == 'integer')
		option.datatype = 'uinteger';
	if (field.multiline)
		option.rows = 6;
	if (field.placeholder)
		option.placeholder = field.placeholder;
	if (field.default !== undefined)
		option.default = typeof(field.default) == 'boolean' ? (field.default ? '1' : '0') : String(field.default);
	(field.options || []).forEach((item) => option.value(item.value, nodeChoiceLabel(item)));

	if (field.when) {
		field.types.forEach((type) => field.when.values.forEach((value) => {
			option.depends({ type: type, [field.when.field]: value });
		}));
	}
	else {
		field.types.forEach((type) => option.depends('type', type));
	}
	if ((field.required_types || []).length) {
		option.validate = function(sectionId, value) {
			const type = uci.get('steer', sectionId, 'type');
			if (field.required_types.includes(type) && (value == null || value === '' || (Array.isArray(value) && !value.length)))
				return _('%s is required for %s nodes.').format(nodeFieldLabel(field), protocolLabel(type));
			return true;
		};
	}
	return option;
}

function selectedNodeGroup(groups) {
	const requested = new URLSearchParams(window.location.search || '').get('node_group');
	return groups.some((group) => group.id == requested) ? requested : manualNodeGroup;
}

function nextManualNodeID() {
	let index = 1;
	while (uci.get('steer', 'manual_node_' + index))
		index++;
	return 'manual_node_' + index;
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
				const last = subscription.fetched_at ? new Date(subscription.fetched_at).toLocaleString() : (subscription.error ? _('Update failed') : _('Not fetched'));
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
							const skipped = (update.snapshots || []).reduce((count, snapshot) => count + (snapshot.skipped || 0), 0);
							const message = skipped
								? _('Subscription updated; %d invalid nodes skipped.').format(skipped)
								: _('Subscription updated.');
							ui.addNotification(null, E('p', {}, message), skipped ? 'warning' : 'info');
							window.location.reload();
							return update;
						});
					}
				}, _('Update now')) ];
				stale.forEach((node) => actions.push(' ', E('button', {
					'class': 'btn cbi-button-negative',
					'click': function() {
						return steer.cleanSubscription(subscription.id, node).then((clean) => {
							if (!clean?.ok) {
								ui.addNotification(_('Subscription node removal failed'), E('p', {}, clean?.error || _('Unknown error')), 'danger');
								return clean;
							}
							window.location.reload();
							return clean;
						});
					}
				}, _('Remove %s').format(node))));
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
				E('h3', {}, _('Import nodes')),
				E('p', {}, _('Paste one node link per line, or paste a Base64-wrapped subscription document. Steer parses and validates it before it enters the pending configuration.'))
			]),
			E('button', {
				'class': 'cbi-button cbi-button-add',
				click: function(ev) {
					ev.preventDefault();
					showImportDialog();
				}
			}, _('Import nodes'))
		])
	]);
}

function setSpeedtestButton(button, state, label, detail) {
	if (!button)
		return;
	button.disabled = state == 'testing';
	button.classList.toggle('spinning', state == 'testing');
	button.classList.toggle('cbi-button-positive', state == 'success');
	button.classList.toggle('cbi-button-negative', state == 'error');
	button.textContent = label;
	button.title = detail || '';
}

function speedtestResult(report, download) {
	const results = report?.results || [];
	const successful = results.filter((result) => result.ok === true);
	if (!successful.length)
		return { ok: false, label: _('Failed'), detail: _('See diagnostic logs for details.') };

	if (download) {
		const measured = successful.filter((result) => result.downloaded_bytes > 0 && result.download_milliseconds > 0)
			.map((result) => ({ result, mbps: result.downloaded_bytes * 8 / result.download_milliseconds / 1000 }))
			.sort((left, right) => right.mbps - left.mbps);
		if (!measured.length)
			return { ok: false, label: _('Failed'), detail: _('No download measurement was returned.') };
		return {
			ok: true,
			label: _('%s Mbps').format(measured[0].mbps.toFixed(1)),
			detail: measured.map((item) => '%s: %s Mbps · %s'.format(
				item.result.url,
				item.mbps.toFixed(1),
				_('%s bytes in %s ms · HTTP %s').format(item.result.downloaded_bytes, item.result.download_milliseconds, item.result.status)
			)).join('\n')
		};
	}

	const measured = successful.map((result) => ({
		result,
		milliseconds: result.first_byte_milliseconds ?? result.tls_milliseconds ?? result.connect_milliseconds ?? 0
	})).sort((left, right) => left.milliseconds - right.milliseconds);
	if (!measured.length)
		return { ok: false, label: _('Failed'), detail: _('No connection latency was returned.') };
	return {
		ok: true,
		label: _('%d ms').format(measured[0].milliseconds),
		detail: measured.map((item) => '%s: %s'.format(item.result.url,
			_('Connect %s ms · TLS %s ms · HTTP %s · %s attempt(s)').format(
				item.result.connect_milliseconds || 0,
				item.result.tls_milliseconds || 0,
				item.result.status,
				item.result.attempts
			))).join('\n')
	};
}

function runSpeedtest(sectionId, download, button) {
	setSpeedtestButton(button, 'testing', _('Testing…'));
	return steer.speedtest(sectionId, download).then((report) => {
		const result = speedtestResult(report, download);
		setSpeedtestButton(button, result.ok ? 'success' : 'error', result.label, result.detail);
		return result.ok;
	}).catch((error) => {
		setSpeedtestButton(button, 'error', _('Failed'), _('See diagnostic logs for details.'));
		return false;
	});
}

function runRouteSpeedtest(sectionId, download, button) {
	setSpeedtestButton(button, 'testing', _('Testing…'));
	return steer.routeSpeedtest(sectionId, download).then((report) => {
		const result = speedtestResult(report, download);
		setSpeedtestButton(button, result.ok ? 'success' : 'error', result.label, result.detail);
		return result.ok;
	}).catch((error) => {
		setSpeedtestButton(button, 'error', _('Failed'), _('See diagnostic logs for details.'));
		return false;
	});
}

function findSpeedtestButton(sectionId, download) {
	const option = download ? '_download_speedtest' : '_connect_speedtest';
	const id = 'cbid.steer.%s.%s'.format(sectionId, option);
	return document.querySelector('output[for="%s"] button'.format(id));
}

function runBatchSpeedtest(sectionIds, download, button) {
	const title = download ? _('Batch download test') : _('Batch connection test');
	const buttons = Array.from(document.querySelectorAll('.steer-speedtest-batch button'));
	let cursor = 0;
	let completed = 0;
	let succeeded = 0;
	buttons.forEach((candidate) => { candidate.disabled = true; });
	button.textContent = '%s · 0/%d'.format(title, sectionIds.length);

	const worker = async function() {
		while (cursor < sectionIds.length) {
			const sectionId = sectionIds[cursor++];
			if (await runSpeedtest(sectionId, download, findSpeedtestButton(sectionId, download)))
				succeeded++;
			completed++;
			button.textContent = '%s · %d/%d'.format(title, completed, sectionIds.length);
		}
	};

	const workers = [];
	for (let i = 0; i < Math.min(4, sectionIds.length); i++)
		workers.push(worker());
	return Promise.all(workers).then(() => {
		button.textContent = '%s · %d/%d'.format(title, succeeded, sectionIds.length);
		button.title = _('%d/%d succeeded; click to test again.').format(succeeded, sectionIds.length);
		buttons.forEach((candidate) => { candidate.disabled = false; });
	});
}

function renderBatchSpeedtests(sectionIds) {
	if (!sectionIds.length)
		return null;
	return E('div', { 'class': 'steer-speedtest-batch' }, [
		E('button', { 'class': 'btn cbi-button-action', 'click': function(ev) { return runBatchSpeedtest(sectionIds, false, ev.currentTarget); } }, _('Batch connection test')),
		E('button', { 'class': 'btn cbi-button-action', 'click': function(ev) { return runBatchSpeedtest(sectionIds, true, ev.currentTarget); } }, _('Batch download test'))
	]);
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
		preview.replaceChildren(E('p', { 'class': 'spinning' }, _('Parsing and validating nodes…')));
		return steer.importNodes(input.value).then((parsed) => {
			const nodes = parsed?.nodes || [];
			if (!nodes.length)
				throw new Error(parsed?.error || _('No valid node was returned.'));
			input.value = '';
			const nameInput = nodes.length == 1 ? E('input', {
				'class': 'cbi-input-text', 'value': nodes[0].name, 'autocomplete': 'off'
			}) : null;
			const save = function(saveEvent) {
				saveEvent.preventDefault();
				if (nameInput)
					nodes[0].name = nameInput.value.trim() || nodes[0].name;
				nodes.forEach((node) => {
					const id = nextManualNodeID();
					uci.add('steer', 'node', id);
					Object.keys(node).filter((option) => option != 'id' && !option.startsWith('source_') && option != 'pinned_stale').forEach((option) => {
						let value = node[option];
						if (typeof(value) == 'boolean')
							value = value ? '1' : '0';
						else if (typeof(value) == 'number')
							value = String(value);
						uci.set('steer', id, option, value);
					});
				});
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
				E('h4', {}, _('%d node(s) ready to import').format(nodes.length)),
				parsed.skipped ? E('p', { 'class': 'alert-message warning' }, _('%d invalid node(s) were skipped.').format(parsed.skipped)) : '',
				nameInput ? E('div', { 'class': 'cbi-value' }, [
					E('label', { 'class': 'cbi-value-title' }, _('Name')),
					E('div', { 'class': 'cbi-value-field' }, nameInput)
				]) : '',
				E('dl', { 'class': 'steer-status__facts' }, previewFacts(nodes[0]).map((fact) =>
					E('div', {}, [ E('dt', {}, fact[0]), E('dd', {}, String(fact[1])) ]))),
				E('div', { 'class': 'right' }, E('button', {
					'class': 'cbi-button cbi-button-positive', click: save
				}, _('Import into pending configuration')))
			]));
		}).catch((error) => {
			preview.replaceChildren(E('div', { 'class': 'alert-message danger' }, String(error)));
		});
	};

	ui.showModal(_('Import nodes'), [
		E('p', {}, _('Paste one node link per line, or paste a Base64-wrapped subscription document. Steer parses and validates it before it enters the pending configuration.')),
		input,
		preview,
		E('div', { 'class': 'right' }, [
			E('button', { 'class': 'cbi-button', 'click': ui.hideModal }, _('Cancel')),
			' ',
			E('button', { 'class': 'cbi-button cbi-button-action', 'click': review }, _('Parse and preview'))
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
		const routeReferences = collectRouteReferences(routes);
		const enabledNodeIds = nodes.filter((node) => nodeGroupID(node) == activeNodeGroup && node.enabled != '0')
			.map((node) => node['.name']);
		const summaryOnly = activeNodeGroup != manualNodeGroup;
		const page = (window.location.pathname || '').split('/').pop();
		steer.loadStyle();

		m = new form.Map('steer', page == 'routes' ? _('Routes') : (page == 'subscriptions' ? _('Node subscriptions') : _('Proxy nodes')));

		if (page == 'subscriptions') {
			s = m.section(form.GridSection, 'subscription', _('Node subscriptions'));
			steer.configureNamedSection(s);
			configureSubscriptionRemoval(s, nodes, routes);
			s.addremove = true;
			s.nodescriptions = true;
			s.addbtntitle = _('Add subscription');
			s.sectiontitle = function(sectionId) {
				return uci.get('steer', sectionId, 'name') || uci.get('steer', sectionId, 'url') || _('Unnamed');
			};
			o = s.option(form.Flag, 'enabled', _('Enabled')); o.default = '1'; o.editable = true;
			o = s.option(form.Value, 'name', _('Name')); o.rmempty = false; o.modalonly = true;
			o = s.option(form.Value, 'url', _('Subscription URL')); o.datatype = 'url'; o.rmempty = false; o.editable = true;
			o = s.option(form.Value, 'update_interval', 'Update interval'); o.placeholder = '6h'; o.modalonly = true;
			return m.render().then((formNode) => E([], [ renderSubscriptionStatus(data?.[1]), formNode ]));
		}

		if (page == 'routes') {
			addSystemRouteSection(m, routes.find((route) => route.kind == 'direct'));
			addSystemRouteSection(m, routes.find((route) => route.kind == 'block'));

		s = m.section(form.GridSection, 'route', _('Single-node routes'));
		steer.configureNamedSection(s, { enabled: '1', kind: 'single' });
		s.addremove = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add single-node route');
		s.filter = function(sectionId) {
			return uci.get('steer', sectionId, 'kind') == 'single';
		};
		s.sectiontitle = function(sectionId) {
			return uci.get('steer', sectionId, 'name') || _('Unnamed');
		};

		o = s.option(form.Flag, 'enabled', _('Enabled'));
		o.default = '1';
		o.editable = true;

		o = s.option(form.Value, 'name', _('Name'));
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.DummyValue, '_single_kind', _('Kind'));
		o.textvalue = function() { return _('Single node'); };
		if (!nodeReferences.length)
			o.description = _('Create a proxy node before adding a single-node route.');

		if (nodeReferences.length) {
			o = s.option(form.RichListValue, 'node', _('Node'));
			o.depends('kind', 'single');
			o.rmempty = false;
			addNodeValues(o, nodeReferences);
			o.textvalue = function(sectionId) {
				return nodeReferenceLabel(nodeReferences, uci.get('steer', sectionId, 'node'));
			};
		}

		if (routeReferences.length) {
			o = s.option(form.RichListValue, 'detour', _('Detour route'));
			o.depends('kind', 'single');
			o.optional = true;
			o.rmempty = true;
			o.value('', _('Direct connection'));
			routeReferences.forEach((reference) => o.value(reference.id, reference.label));
			o.textvalue = function(sectionId) {
				const detour = uci.get('steer', sectionId, 'detour');
				if (!detour)
					return _('Direct connection');
				return routeReferences.find((reference) => reference.id == detour)?.label || _('Missing route');
			};
			o.description = _('The selected single-node route dials first. Apply rejects missing, disabled, non-single and cyclic detours.');
		}

		o = s.option(form.Button, '_route_connect_test', _('Chain connection test'));
		o.depends({ kind: 'single', enabled: '1' });
		o.editable = true;
		o.inputtitle = _('Test');
		o.inputstyle = 'action';
		o.write = function() {};
		o.remove = function() {};
		o.onclick = function(ev, sectionId) { return runRouteSpeedtest(sectionId, false, ev.currentTarget); };

		o = s.option(form.Button, '_route_download_test', _('Chain download test'));
		o.depends({ kind: 'single', enabled: '1' });
		o.editable = true;
		o.inputtitle = _('Test');
		o.inputstyle = 'action';
		o.write = function() {};
		o.remove = function() {};
		o.onclick = function(ev, sectionId) { return runRouteSpeedtest(sectionId, true, ev.currentTarget); };
			return m.render();
		}

		s = m.section(form.GridSection, 'node', _('Proxy nodes — %s (%d)').format(activeGroup.label, activeGroup.count));
		steer.configureNamedSection(s);
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
		o = s.option(form.Flag, 'enabled', _('Enabled'));
		o.default = '1';
		o.editable = !summaryOnly;

		o = s.option(form.Button, '_connect_speedtest', _('Connection test'));
		o.editable = true;
		o.inputtitle = _('Test');
		o.inputstyle = 'action';
		o.write = function() {};
		o.remove = function() {};
		o.onclick = function(ev, sectionId) { return runSpeedtest(sectionId, false, ev.currentTarget); };

		o = s.option(form.Button, '_download_speedtest', _('Download test'));
		o.editable = true;
		o.inputtitle = _('Test');
		o.inputstyle = 'action';
		o.write = function() {};
		o.remove = function() {};
		o.onclick = function(ev, sectionId) { return runSpeedtest(sectionId, true, ev.currentTarget); };

		o = s.option(form.Value, 'name', _('Name'));
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.ListValue, 'type', _('Protocol'));
		uiSpec.node_types.forEach((item) => o.value(item.value, protocolLabel(item.value)));
		o.rmempty = false;
		o.editable = !summaryOnly;
		o.textvalue = function(sectionId) {
			return protocolLabel(uci.get('steer', sectionId, 'type'));
		};

		o = s.option(form.Value, 'server', _('Server'));
		o.rmempty = false;
		o.editable = !summaryOnly;
		uiSpec.node_types.filter((item) => item.value != 'tor').forEach((item) => o.depends('type', item.value));

		o = s.option(form.Value, 'server_port', _('Port'));
		o.datatype = 'port';
		o.rmempty = false;
		o.editable = !summaryOnly;
		uiSpec.node_types.filter((item) => item.value != 'tor').forEach((item) => o.depends('type', item.value));

		uiSpec.node_fields
			.filter((field) => ![ 'enabled', 'name', 'server', 'server_port' ].includes(field.key))
			.forEach((field) => addGeneratedNodeField(s, field));

		return m.render().then((formNode) => {
			const contents = [ renderNodeGroupNavigation(nodeGroups, activeNodeGroup) ];
			const batchSpeedtests = renderBatchSpeedtests(enabledNodeIds);
			if (batchSpeedtests)
				contents.push(batchSpeedtests);
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
