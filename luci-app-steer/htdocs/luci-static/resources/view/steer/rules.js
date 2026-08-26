/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

'use strict';
'require form';
'require uci';
'require view';
'require steer as steer';
'require steer.ui-spec as uiSpec';

function asList(value) {
	if (value == null)
		return [];
	return Array.isArray(value) ? value : [ value ];
}

function collectReferences(objects, rules, field, label) {
	const known = {};
	const references = [];
	objects.forEach((object) => {
		known[object['.name']] = true;
		const detail = object.server && object.server_port ? '%s:%s'.format(object.server, object.server_port)
			: (object.listen && object.listen_port ? '%s:%s'.format(object.listen, object.listen_port)
				: (object.kind == 'single' ? _('Node: %s').format(object.node || _('not selected')) : ''));
		references.push({ id: object['.name'], label: label ? label(object) : (object.name || object['.name']), detail });
	});
	rules.forEach((rule) => {
		asList(rule[field]).forEach((value) => {
			if (value && !known[value]) {
				known[value] = true;
				references.push({ id: value, label: _('Missing: %s').format(value) });
			}
		});
	});
	return steer.disambiguateReferences(references).map((reference) => [ reference.id, reference.label ]);
}

function routeReferenceLabel(route) {
	if (route.name)
		return route.name;
	switch (route.kind) {
	case 'direct': return _('Direct');
	case 'block': return _('Reject');
	default: return route['.name'];
	}
}

function addReferences(option, references) {
	references.forEach((reference) => option.value(reference[0], reference[1]));
}

function editorLines(value) {
	return String(value || '').split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
}

function currentLine(value, cursor) {
	const text = String(value || '');
	const position = Math.max(0, Math.min(cursor == null ? text.length : cursor, text.length));
	const start = text.lastIndexOf('\n', position - 1) + 1;
	const next = text.indexOf('\n', position);
	const end = next < 0 ? text.length : next;
	return { start, end, value: text.slice(start, end).trim() };
}

function editorSuggestions(kind, value, catalog) {
	const query = String(value || '').trim().toLowerCase();
	const prefixes = kind == 'domain' ? uiSpec.domain_prefixes : uiSpec.ip_prefixes;
	const geoPrefix = kind == 'domain' ? 'geosite:' : 'geoip:';
	if (query.startsWith(geoPrefix)) {
		const needle = query.slice(geoPrefix.length);
		return (catalog || []).filter((name) =>
			String(name).toLowerCase().includes(needle)).map((name) => geoPrefix + name);
	}
	if (query == '' || prefixes.some((prefix) => prefix.startsWith(query)))
		return prefixes.filter((prefix) => prefix.startsWith(query));
	return [];
}

function acceptsSuggestion(key) {
	return key == 'Tab';
}

const MatchEditor = form.TextValue.extend({
	__name__: 'Steer.MatchEditor',
	currentLine: currentLine,
	editorSuggestions: editorSuggestions,
	acceptsSuggestion: acceptsSuggestion,

	cfgvalue: function(sectionId) {
		return asList(uci.get('steer', sectionId, this.option)).join('\n');
	},

	write: function(sectionId, value) {
		const lines = editorLines(value);
		if (lines.length)
			uci.set('steer', sectionId, this.option, lines);
		else
			uci.unset('steer', sectionId, this.option);
	},

	remove: function(sectionId) {
		uci.unset('steer', sectionId, this.option);
	},

	renderWidget: function() {
		const widget = form.TextValue.prototype.renderWidget.apply(this, arguments);
		const textarea = widget.matches?.('textarea') ? widget : widget.querySelector('textarea');
		if (!textarea)
			return widget;
		textarea.classList.add('steer-match-editor__input');
		textarea.setAttribute('autocomplete', 'off');
		textarea.setAttribute('autocapitalize', 'off');
		textarea.setAttribute('spellcheck', 'false');

		const list = E('div', {
			'class': 'steer-match-editor__suggestions',
			'role': 'listbox',
			'hidden': ''
		});
		const status = E('div', { 'class': 'steer-match-editor__status' });
		let matches = [];
		let active = 0;

		const select = (value) => {
			const line = currentLine(textarea.value, textarea.selectionStart);
			textarea.value = textarea.value.slice(0, line.start) + value + textarea.value.slice(line.end);
			const position = line.start + value.length;
			textarea.setSelectionRange(position, position);
			textarea.dispatchEvent(new Event('input', { bubbles: true }));
			textarea.focus();
		};

		const renderSuggestions = () => {
			const line = currentLine(textarea.value, textarea.selectionStart);
			matches = editorSuggestions(this.matchKind, line.value, this.catalogNames);
			active = Math.min(active, Math.max(0, matches.length - 1));
			list.replaceChildren();
			if (!matches.length) {
				list.hidden = true;
				status.textContent = '';
				return;
			}
			const visible = matches.slice(0, 50);
			visible.forEach((value, index) => {
				const button = E('button', {
					'type': 'button',
					'class': 'steer-match-editor__suggestion' + (index == active ? ' is-active' : ''),
					'role': 'option',
					'aria-selected': index == active ? 'true' : 'false'
				}, value);
				button.addEventListener('mousedown', (event) => event.preventDefault());
				button.addEventListener('click', () => select(value));
				list.appendChild(button);
			});
			status.textContent = matches.length > visible.length ?
				_('%d matches; showing the first %d.').format(matches.length, visible.length) :
				_('%d matches.').format(matches.length);
			list.hidden = false;
		};

		const resetSuggestions = () => {
			active = 0;
			renderSuggestions();
		};
		textarea.addEventListener('input', resetSuggestions);
		textarea.addEventListener('click', resetSuggestions);
		textarea.addEventListener('focus', resetSuggestions);
		textarea.addEventListener('keydown', (event) => {
			if (list.hidden || !matches.length)
				return;
			if (event.key == 'ArrowDown' || event.key == 'ArrowUp') {
				const direction = event.key == 'ArrowDown' ? 1 : -1;
				active = (active + direction + Math.min(matches.length, 50)) % Math.min(matches.length, 50);
				renderSuggestions();
				event.preventDefault();
			}
			else if (acceptsSuggestion(event.key)) {
				select(matches[active]);
				event.preventDefault();
			}
			else if (event.key == 'Escape') {
				list.hidden = true;
				status.textContent = '';
				event.preventDefault();
			}
		});

		return E('div', { 'class': 'steer-match-editor' }, [ widget, list, status ]);
	}
});

function configureMatchEditor(option, catalog, kind) {
	const geoKind = kind == 'domain' ? 'geosite' : 'geoip';
	const source = catalog?.[geoKind] || {};
	const names = Array.isArray(source.names) ? source.names : [];
	option.matchKind = kind;
	option.catalogNames = names;
	option.currentLine = currentLine;
	option.editorSuggestions = editorSuggestions;
	option.editorLines = editorLines;
	option.acceptsSuggestion = acceptsSuggestion;
	option.rows = 7;
	option.description = source.ok === true ?
		_('%d valid %s names are available for dynamic completion.').format(names.length, geoKind) :
		_('The valid-name catalog is unavailable: %s').format(steer.rpcErrorText(source));
}

function matchSummaryTokens(sectionId) {
	if (uci.get('steer', sectionId, 'default') == '1')
		return [ 'default' ];
	return uiSpec.rule_match_fields.flatMap((field) => {
		const values = asList(uci.get('steer', sectionId, field));
		if (!values.length) return [];
		return [ (field == 'network' || field == 'protocol')
			? '%s:%s'.format(field, values.join('/'))
			: '%s:%d'.format(field, values.length) ];
	});
}

function matchDNSContinues(sectionId) {
	const populated = uiSpec.rule_match_fields.filter((field) => asList(uci.get('steer', sectionId, field)).length > 0);
	return populated.length > 0 && populated.every((field) => uiSpec.rule_connection_only_fields.includes(field));
}

function matchSummary(sectionId) {
	let parts = [];
	const inbounds = asList(uci.get('steer', sectionId, 'inbound'));
	const domains = asList(uci.get('steer', sectionId, 'domain_match')).length;
	const clients = asList(uci.get('steer', sectionId, 'source_ip_cidr')).length;
	const macs = asList(uci.get('steer', sectionId, 'source_mac_address')).length;
	const addresses = asList(uci.get('steer', sectionId, 'ip_match')).length;
	const networks = asList(uci.get('steer', sectionId, 'network'));
	const protocols = asList(uci.get('steer', sectionId, 'protocol'));
	const ports = asList(uci.get('steer', sectionId, 'port')).length;
	if (inbounds.length)
		parts.push(_('%d inbounds').format(inbounds.length));
	if (domains)
		parts.push(_('%d domain patterns').format(domains));
	if (clients)
		parts.push(_('%d client ranges').format(clients));
	if (macs)
		parts.push(_('%d client MAC addresses').format(macs));
	if (addresses)
		parts.push(_('%d destination IP expressions').format(addresses));
	if (networks.length)
		parts.push(networks.join('/').toUpperCase());
	if (protocols.length)
		parts.push(_('Protocols: %s').format(protocols.join('/').toUpperCase()));
	if (ports)
		parts.push(_('%d ports').format(ports));
	if (matchDNSContinues(sectionId))
		parts.push(_('DNS continues to the next rule'));
	return parts.length ? parts.join(', ') : _('No match condition');
}

function renderSystemBypass() {
	return E('section', { 'class': 'steer-system-rule' }, [
		E('div', { 'class': 'steer-system-rule__order' }, [
			E('span', {}, _('Before rule 1')),
			E('strong', {}, _('System'))
		]),
		E('div', { 'class': 'steer-system-rule__body' }, [
			E('strong', {}, _('System rescue direct')),
			E('p', {}, _('Non-globally-reachable and local destinations bypass Steer before user rules. Router-originated UDP NTP on port 123 always goes direct. Traditional TCP and UDP DNS on port 53 enters the dedicated DNS shim.')),
			E('details', {}, [
				E('summary', {}, _('Show fixed boundary')),
				E('p', {}, _('Loopback, private-use, shared, link-local, documentation, benchmarking, discard-only and multicast ranges. Globally reachable special-purpose exceptions remain eligible for user rules. The NTP exception applies only to traffic created by the router; LAN client UDP/123 still follows user rules.'))
			])
		]),
		E('dl', { 'class': 'steer-system-rule__facts' }, [ E('div', {}, [ E('dt', {}, _('Route')), E('dd', {}, 'DIRECT') ]) ])
	]);
}

return view.extend({
	load: function() {
		return Promise.all([ uci.load('steer'), steer.geodataCatalog() ]);
	},

	render: function(data) {
		let m, s, o;
		const catalog = data[1] || {};
		steer.loadStyle();
		const rules = uci.sections('steer', 'rule');
		const dnsProfiles = uci.sections('steer', 'dns_profile');
		const routes = uci.sections('steer', 'route');
		const localProxies = uci.sections('steer', 'local_proxy');
		const defaultRule = rules.find((rule) => rule.default == '1');
		const dnsReferences = collectReferences(dnsProfiles, rules, 'dns_profile');
		const routeReferences = collectReferences(routes, rules, 'route', routeReferenceLabel);
		const inboundReferences = collectReferences(localProxies, rules, 'inbound');

		m = new form.Map('steer', _('Rules'));

		s = m.section(form.GridSection, 'rule', _('Ordered steering rules'));
		const directRoute = routes.find((route) => route.kind == 'direct' && route.enabled != '0')
			|| routes.find((route) => route.enabled != '0');
		const initialDNS = dnsProfiles.find((profile) => profile.enabled != '0');
		steer.configureNamedSection(s, steer.creationDefaults('rules', {
			dns_profile: initialDNS?.['.name'] || '', route: directRoute?.['.name'] || ''
		}), defaultRule?.['.name']);
		const orderedSection = s;
		s.addremove = true;
		s.sortable = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add steering rule');
		s.sectiontitle = function(sectionId) {
			return uci.get('steer', sectionId, 'name') || _('Unnamed');
		};
		s.filter = function(sectionId) {
			return uci.get('steer', sectionId, 'default') != '1';
		};
		s.tab('intent', _('Intent'));
		s.tab('match', _('Match'));

		o = s.taboption('intent', form.Flag, 'enabled', _('Enabled'));
		o.default = '1';
		o.editable = true;

		o = s.taboption('intent', form.Value, 'name', _('Name'));
		o.rmempty = true;
		o.optional = true;
		o.modalonly = true;

		o = s.taboption('intent', form.DummyValue, '_match', _('Match'));
		o.textvalue = matchSummary;
		o.cfgvalue = matchSummary;
		o.summaryTokens = matchSummaryTokens;
		o.dnsContinues = matchDNSContinues;

		o = s.taboption('intent', form.ListValue, 'dns_profile', _('DNS profile'));
		o.rmempty = false;
		o.editable = true;
		addReferences(o, dnsReferences);

		o = s.taboption('intent', form.ListValue, 'route', _('Route'));
		o.rmempty = false;
		o.editable = true;
		addReferences(o, routeReferences);

		if (inboundReferences.length) {
			o = s.taboption('match', form.MultiValue, 'inbound', _('Inbound'));
			o.modalonly = true;
			addReferences(o, inboundReferences);
			o.description = _('Optionally limit this rule to one or more user-created local proxy endpoints.');
		}

		o = s.taboption('match', MatchEditor, 'domain_match', _('Domain match'));
		o.modalonly = true;
		o.placeholder = 'domain:example.com\nfull:www.example.com\nkeyword\ngeosite:geolocation-!cn\nregexp:^api\\d+\\.example\\.com$';
		configureMatchEditor(o, catalog, 'domain');
		o.description = _('One expression per line. Plain text is a keyword; full:, domain:, regexp: and geosite: select exact, suffix, regular-expression and GeoSite matching. Lines are OR.') + ' ' + o.description;

		o = s.taboption('match', MatchEditor, 'ip_match', _('Destination IP match'));
		o.modalonly = true;
		o.placeholder = '1.1.1.1\n10.0.0.0/8\ngeoip:cn';
		configureMatchEditor(o, catalog, 'ip');
		o.description = _('One IP, CIDR or geoip: expression per line. Lines are OR. This field affects traffic routing but not DNS selection.') + ' ' + o.description;

		o = s.taboption('match', form.DynamicList, 'source_ip_cidr', _('Client IP/CIDR'));
		o.modalonly = true;
		o.datatype = 'cidr';
		o.description = _('Use stable DHCP leases or stable IPv6 addresses when a rule belongs to one device.');

		o = s.taboption('match', form.DynamicList, 'source_mac_address', _('Client MAC address'));
		o.modalonly = true;
		o.datatype = 'macaddr';
		o.description = _('Matches LAN clients before IP routing, so the same rule covers IPv4, IPv6, SLAAC and temporary addresses. It cannot be combined with a local proxy inbound.');

		o = s.taboption('match', form.MultiValue, 'network', _('Network'));
		o.modalonly = true;
		o.description = _('Connection stage only. If a rule has only IP, network, protocol or port conditions, DNS continues to subsequent rules.');
		uiSpec.rule_networks.forEach((item) => o.value(item.value, steer.uiSpecLabel(item.label)));

		o = s.taboption('match', form.MultiValue, 'protocol', _('Detected protocol'));
		o.modalonly = true;
		o.description = _('Connection stage only. Values come from the shared protocol enumeration.');
		uiSpec.rule_protocols.forEach((item) => o.value(item.value, steer.uiSpecLabel(item.label)));

		o = s.taboption('match', form.DynamicList, 'port', _('Destination ports'));
		o.modalonly = true;
		o.datatype = 'port';
		o.description = _('Exact ports only. This condition is evaluated by traffic routing, not DNS.');

		s = m.section(form.TableSection, 'rule', _('Default'));
		s.anonymous = false;
		s.addremove = false;
		s.sortable = false;
		s.nodescriptions = true;
		s.sectiontitle = function() {
			return _('Default');
		};
		s.filter = function(sectionId) {
			return uci.get('steer', sectionId, 'default') == '1';
		};
		s.description = _('Default is always enabled and fixed after every ordered rule. Only its DNS profile and route can be changed.');

		o = s.option(form.ListValue, 'dns_profile', _('DNS profile'));
		o.rmempty = false;
		addReferences(o, dnsReferences);

		o = s.option(form.ListValue, 'route', _('Route'));
		o.rmempty = false;
		addReferences(o, routeReferences);

		const intent = E('div', { 'class': 'steer-intent', 'aria-label': _('Rule execution model') }, [
			E('div', { 'class': 'steer-intent__step steer-intent__step--match' }, [
				E('span', { 'class': 'steer-intent__label' }, _('First')), E('strong', {}, _('Match the request'))
			]),
			E('span', { 'class': 'steer-intent__arrow', 'aria-hidden': 'true' }, '→'),
			E('div', { 'class': 'steer-intent__step steer-intent__step--dns' }, [
				E('span', { 'class': 'steer-intent__label' }, _('DNS-visible conditions')), E('strong', {}, _('Choose DNS profile'))
			]),
			E('span', { 'class': 'steer-intent__arrow', 'aria-hidden': 'true' }, '→'),
			E('div', { 'class': 'steer-intent__step steer-intent__step--route' }, [
				E('span', { 'class': 'steer-intent__label' }, _('All conditions')), E('strong', {}, _('Choose route'))
			])
		]);

		return m.render().then((formNode) => steer.focusSection(orderedSection, 'rule').then(() =>
			E([], [ renderSystemBypass(), intent, formNode ])));
	},

	handleSaveApply: function(ev, mode) {
		return steer.apply(this, ev, mode);
	}
});
