#!/usr/bin/env node

'use strict';

const assert = require('assert');
const fs = require('fs');
const path = require('path');

if (typeof String.prototype.format != 'function') {
	Object.defineProperty(String.prototype, 'format', {
		value: function(...values) {
			let index = 0;
			return String(this).replace(/%[sd]/g, () => String(values[index++]));
		}
	});
}

const root = path.resolve(__dirname, '../..');

function element(tag, attributes, children) {
	if (Array.isArray(tag)) {
		children = attributes;
		attributes = {};
	}
	else if (arguments.length == 2 &&
		(Array.isArray(attributes) || typeof attributes != 'object' || attributes == null)) {
		children = attributes;
		attributes = {};
	}

	const values = children == null ? [] : (Array.isArray(children) ? children : [ children ]);
	assert.ok(!values.includes(null), 'LuCI element children must not contain null');
	return {
		tag,
		attributes: attributes || {},
		children: values,
		disabled: false,
		replaceChildren: function(...replacement) { this.children = replacement; }
	};
}

function findElements(root, predicate) {
	if (root == null || typeof root != 'object')
		return [];
	const matches = predicate(root) ? [ root ] : [];
	return matches.concat((root.children || []).flatMap((child) => findElements(child, predicate)));
}

function createEnvironment(sections) {
	const maps = [];
	class DynamicList {}
	DynamicList.prototype.renderWidget = function() {};
	DynamicList.extend = function(definition) {
		class ExtendedDynamicList extends DynamicList {}
		Object.assign(ExtendedDynamicList.prototype, definition);
		return ExtendedDynamicList;
	};
	class TextValue {}
	TextValue.prototype.renderWidget = function() {};
	TextValue.extend = function(definition) {
		class ExtendedTextValue extends TextValue {}
		Object.assign(ExtendedTextValue.prototype, definition);
		return ExtendedTextValue;
	};

	class Option {
		constructor(type, name) {
			this.type = type;
			this.name = name;
			this.values = [];
		}

		value(key, label, description) {
			const value = [ String(key), String(label == null ? key : label) ];
			if (description != null)
				value.push(String(description));
			this.values.push(value);
		}

		depends() {}

		submit(sectionId, value) {
			/* RichListValue passes only `optional` to LuCI's Dropdown widget. */
			if (this.type == 'RichListValue' && (value == null || value === '') && this.optional !== true)
				return Promise.reject(new TypeError(`${this.name} must not be empty`));

			if (value == null || value === '') {
				if (this.rmempty || this.optional)
					return Promise.resolve(this.remove(sectionId));
				return Promise.reject(new TypeError(`${this.name} must not be empty`));
			}

			return Promise.resolve(this.write(sectionId, value));
		}

		write(sectionId, value) {
			uci.set('steer', sectionId, this.name, value);
		}

		remove(sectionId) {
			uci.unset('steer', sectionId, this.name);
		}
	}

	class Section {
		constructor(type, sectionType) {
			this.type = type;
			this.sectionType = sectionType;
			this.options = [];
		}

		tab() {}

		option(type, name) {
			const option = new Option(type, name);
			this.options.push(option);
			return option;
		}

		taboption(tab, type, name) {
			return this.option(type, name);
		}
	}

	class Map {
		constructor() {
			this.sections = [];
			maps.push(this);
		}

		section(type, sectionType) {
			const section = new Section(type, sectionType);
			this.sections.push(section);
			return section;
		}

		render() {
			return Promise.resolve(element('form'));
		}
	}

	const form = {
		Map,
		GridSection: 'GridSection',
		TableSection: 'TableSection',
		NamedSection: 'NamedSection',
		Flag: 'Flag',
		Value: 'Value',
		DummyValue: 'DummyValue',
		ListValue: 'ListValue',
		RichListValue: 'RichListValue',
		MultiValue: 'MultiValue',
		DynamicList,
		TextValue,
		Button: 'Button'
	};
	const uci = {
		load: () => Promise.resolve(),
		sections: (config, type) => (sections[type] || []).map((section) => ({ ...section })),
		get: (config, sectionId, option) => {
			const section = Object.values(sections).flat()
				.find((candidate) => candidate['.name'] == sectionId);
			return option == null ? section : section?.[option];
		},
		set: (config, sectionId, option, value) => {
			const section = Object.values(sections).flat()
				.find((candidate) => candidate['.name'] == sectionId);
			section[option] = value;
		},
		unset: (config, sectionId, option) => {
			const section = Object.values(sections).flat()
				.find((candidate) => candidate['.name'] == sectionId);
			delete section[option];
		}
	};
	const view = { extend: (value) => value };
	const speedtestCalls = [];
	const routeSpeedtestCalls = [];
	const overviewProbeCalls = [];
	const cleanSubscriptionCalls = [];
	const notifications = [];
	const steer = {
		loadStyle: () => {},
		status: () => Promise.resolve({}),
		validate: () => Promise.resolve({ ok: true, errors: [], warnings: [] }),
		geodataCatalog: () => Promise.resolve({}),
		subscriptions: () => Promise.resolve({ subscriptions: [] }),
		renderStatus: () => element('div'),
		updateSubscription: () => Promise.resolve({ ok: true }),
		cleanSubscription: (id, node) => {
			cleanSubscriptionCalls.push({ id, node });
			return Promise.resolve(environment.cleanSubscriptionResult);
		},
		speedtest: (node, download) => {
			speedtestCalls.push({ node, download });
			return Promise.resolve({ ok: true, results: [ {
				url: 'https://speed.example/',
				ok: true,
				status: 204,
				attempts: 1,
				first_byte_milliseconds: 42,
				downloaded_bytes: 1000000,
				download_milliseconds: 1000
			} ] });
		},
		routeSpeedtest: (route, download) => {
			routeSpeedtestCalls.push({ route, download });
			return Promise.resolve({ ok: true, results: [ {
				url: 'https://speed.example/', ok: true, status: 204, attempts: 1, first_byte_milliseconds: 55,
				downloaded_bytes: 1000000, download_milliseconds: 1000
			} ] });
		},
		overviewProbe: (kind) => {
			overviewProbeCalls.push(kind);
			return Promise.resolve({ ok: true, results: [ { url: 'https://test.example/', ok: true, first_byte_milliseconds: 20 } ] });
		},
		apply: () => Promise.resolve()
	};
	const ui = {
		addNotification: (title, body, level) => notifications.push({ title, body, level })
	};
	const shareUrl = {};
	const window = { location: {
		pathname: '/cgi-bin/luci/admin/services/steer/nodes', search: '', href: '', reloadCount: 0,
		reload: function() { this.reloadCount++; }
	} };
	const translate = (value) => String(value);

	const environment = {
		form, uci, view, steer, ui, shareUrl, window, maps, translate,
		speedtestCalls, routeSpeedtestCalls, overviewProbeCalls, cleanSubscriptionCalls, notifications,
		cleanSubscriptionResult: { ok: true }
	};
	return environment;
}

function loadView(file, dependencies) {
	const source = fs.readFileSync(path.join(root, file), 'utf8');
	const names = Object.keys(dependencies);
	const values = names.map((name) => dependencies[name]);
	return new Function(...names, source)(...values);
}

function allOptions(environment) {
	return environment.maps.flatMap((map) => map.sections.flatMap((section) => section.options));
}

async function renderRules(sections, catalog = {}) {
	const environment = createEnvironment(sections);
	const view = loadView(
		'luci-app-steer/htdocs/luci-static/resources/view/steer/rules.js',
		{
			dom: {},
			form: environment.form,
			uci: environment.uci,
			view: environment.view,
			steer: environment.steer,
			E: element,
			_: environment.translate
		}
	);
	await view.render([ null, catalog ]);
	return environment;
}

async function renderNodes(sections, search = '', subscriptionStatus) {
	const environment = createEnvironment(sections);
	environment.window.location.search = search;
	const view = loadView(
		'luci-app-steer/htdocs/luci-static/resources/view/steer/nodes.js',
		{
			form: environment.form,
			uci: environment.uci,
			ui: environment.ui,
			view: environment.view,
			steer: environment.steer,
			shareUrl: environment.shareUrl,
			window: environment.window,
			E: element,
			_: environment.translate
		}
	);
	environment.rendered = await view.render([ null, subscriptionStatus ]);
	return environment;
}

async function renderDns(sections) {
	const environment = createEnvironment(sections);
	const view = loadView(
		'luci-app-steer/htdocs/luci-static/resources/view/steer/dns.js',
		{
			form: environment.form,
			uci: environment.uci,
			view: environment.view,
			steer: environment.steer,
			E: element,
			_: environment.translate
		}
	);
	await view.render();
	return environment;
}

async function renderLocalProxies(sections) {
	const environment = createEnvironment(sections);
	const view = loadView(
		'luci-app-steer/htdocs/luci-static/resources/view/steer/local-proxies.js',
		{
			form: environment.form,
			uci: environment.uci,
			view: environment.view,
			steer: environment.steer,
			E: element,
			_: environment.translate
		}
	);
	await view.render();
	return environment;
}

async function renderOverview(sections) {
	const environment = createEnvironment(sections);
	const view = loadView(
		'luci-app-steer/htdocs/luci-static/resources/view/steer/overview.js',
		{
			form: environment.form,
			uci: environment.uci,
			ui: environment.ui,
			view: environment.view,
			steer: environment.steer,
			E: element,
			_: environment.translate
		}
	);
	environment.rendered = await view.render([ null, {}, { ok: true, errors: [], warnings: [] } ]);
	return environment;
}

function assertGeneratedIds(environment, message) {
	const addable = environment.maps.flatMap((map) => map.sections)
		.filter((section) => section.addremove);
	assert.ok(addable.length > 0, message + ': expected at least one addable section');
	assert.ok(addable.every((section) => section.anonymous === true),
		message + ': addable entities must use UCI-generated IDs');
	assert.ok(addable.every((section) => section.options.some((option) =>
		option.name == 'name' && option.rmempty === false)),
		message + ': every addable entity must require a user-facing name');
}

async function main() {
	let environment = await renderRules({
		rule: [ {
			'.name': 'default',
			name: 'Default',
			default: '1',
			dns_profile: 'direct_dns',
			route: 'direct'
		} ],
		dns_profile: [ { '.name': 'direct_dns' } ],
		route: [ { '.name': 'direct' } ],
		local_proxy: [],
	});
	let options = allOptions(environment);
	assertGeneratedIds(environment, 'Rules');
	assert.equal(options.some((option) => option.name == 'inbound'), false,
		'Rules must not create a choice-only MultiValue without candidates');
	const summary = options.find((option) => option.name == '_match');
	assert.ok(summary, 'Ordinary rules retain an explicit match summary');
	const sourceMac = options.find((option) => option.name == 'source_mac_address');
	assert.ok(sourceMac && sourceMac.datatype == 'macaddr' && sourceMac.modalonly,
		'Rules expose a validated source MAC field without asking users for IP aliases');
	const defaultSection = environment.maps[0].sections.find((section) =>
		section.filter?.('default'));
	assert.ok(defaultSection &&
		defaultSection.options.map((option) => option.name).join(',') == 'dns_profile,route',
		'Default is a fixed non-modal row that exposes only DNS profile and route');
	assert.equal(options.some((option) => option.name == 'default'), false,
		'Ordinary rules cannot be converted into or edit the Default rule');

	const geoSections = {
		rule: [ {
			'.name': 'geo_rule',
			domain_match: [ 'domain:example.com', 'geosite:category-example' ],
			ip_match: [ '192.0.2.0/24', 'geoip:example' ],
			dns_profile: 'direct_dns',
			route: 'direct'
		}, {
			'.name': 'default', default: '1', dns_profile: 'direct_dns', route: 'direct'
		} ],
		dns_profile: [ { '.name': 'direct_dns' } ],
		route: [ { '.name': 'direct' } ],
		local_proxy: []
	};
	environment = await renderRules(geoSections, {
		geosite: { ok: true, names: [ 'category-example', 'category-media' ] },
		geoip: { ok: true, names: [ 'example', 'private' ] }
	});
	options = allOptions(environment);
	const domainMatch = options.find((option) => option.name == 'domain_match');
	const ipMatch = options.find((option) => option.name == 'ip_match');
	assert.ok(domainMatch && ipMatch && domainMatch.type == ipMatch.type,
		'Domain and destination IP use the same multiline match editor');
	assert.deepEqual(domainMatch.editorSuggestions('domain', 'geosite:media', domainMatch.catalogNames),
		[ 'geosite:category-media' ],
		'Domain editor filters GeoSite names using the current line');
	assert.deepEqual(ipMatch.editorSuggestions('ip', 'geoip:pri', ipMatch.catalogNames),
		[ 'geoip:private' ],
		'Destination editor filters GeoIP names using the current line');
	assert.deepEqual(domainMatch.editorSuggestions('domain', 'do', domainMatch.catalogNames),
		[ 'domain:' ], 'Domain editor completes supported syntax prefixes');
	assert.equal(domainMatch.acceptsSuggestion('Tab'), true,
		'Tab accepts the active completion candidate');
	assert.equal(domainMatch.acceptsSuggestion('Enter'), false,
		'Enter remains available for inserting a new editor line');
	assert.deepEqual(domainMatch.editorLines('domain:example.com\n\n geosite:category-media@test '),
		[ 'domain:example.com', 'geosite:category-media@test' ],
		'Multiline editor stores one normalized expression per non-empty line');
	assert.deepEqual(domainMatch.currentLine('full:a.example\ngeosite:media\nregexp:x', 27),
		{ start: 15, end: 28, value: 'geosite:media' },
		'Completion reads only the line under the cursor');
	assert.equal(domainMatch.validate('geo_rule', 'geosite:category-media@test'), true,
		'GeoSite attribute selectors pass when their base category is known');
	assert.notEqual(domainMatch.validate('geo_rule', 'geosite:not-present@cn'), true,
		'Names absent from current Geo data are rejected in the editor');
	const editorInstance = Object.create(domainMatch.type.prototype);
	editorInstance.option = 'domain_match';
	assert.equal(editorInstance.cfgvalue('geo_rule'),
		'domain:example.com\ngeosite:category-example',
		'Editor displays a UCI list as one expression per line');
	editorInstance.write('geo_rule', ' full:www.example.com \n\n geosite:category-media@test ');
	assert.deepEqual(geoSections.rule[0].domain_match,
		[ 'full:www.example.com', 'geosite:category-media@test' ],
		'Editor writes normalized non-empty lines back as one UCI list');
	assert.equal(options.some((option) => [ 'domain', 'domain_suffix', 'domain_keyword', 'geosite', 'geoip', 'ip_cidr' ].includes(option.name)), false,
		'Legacy split fields are absent from the rule editor');
	assert.equal(options.some((option) => option.name == 'rule_set'), false,
		'Users never manage or reference an intermediate rule-set object');

	environment = await renderRules({
		rule: [ {
			'.name': 'stale_inbound',
			inbound: [ 'missing_proxy' ],
			dns_profile: 'direct_dns',
			route: 'direct'
		} ],
		dns_profile: [ { '.name': 'direct_dns' } ],
		route: [ { '.name': 'direct' } ],
		local_proxy: []
	});
	options = allOptions(environment);
	const inbound = options.find((option) => option.name == 'inbound');
	assert.ok(inbound, 'A dangling inbound reference must remain visible for repair');
	assert.deepEqual(inbound.values, [ [ 'missing_proxy', 'Missing: missing_proxy' ] ]);

	environment = await renderNodes({
		node: [],
		route: [
			{ '.name': 'direct', kind: 'direct' },
			{ '.name': 'block', kind: 'block' }
		]
	});
	options = allOptions(environment);
	assertGeneratedIds(environment, 'Nodes and routes');
	const emptyKind = options.find((option) => option.name == 'kind');
	assert.deepEqual(emptyKind.values.map((value) => value[0]), [ 'direct', 'block' ]);
	assert.equal(options.some((option) => option.name == 'node'), false,
		'Routes must not create a ListValue without node candidates');

	environment = await renderNodes({
		node: [],
		route: [ { '.name': 'broken', kind: 'single', node: 'missing_node' } ]
	});
	options = allOptions(environment);
	const missingNode = options.find((option) => option.name == 'node');
	assert.ok(missingNode, 'A dangling route node must remain visible for repair');
	assert.deepEqual(missingNode.values, [ [ 'missing_node', 'Missing: missing_node', 'Group: Missing references' ] ]);

	environment = await renderDns({ dns_profile: [] });
	assertGeneratedIds(environment, 'DNS profiles');
	const dnsProtocol = allOptions(environment).find((option) => option.name == 'protocol');
	assert.deepEqual(dnsProtocol.values.map((value) => value[0]), [ 'udp', 'tcp', 'tls', 'https', 'quic', 'h3' ],
		'DNS profiles expose exactly the six M1 transports');
	environment = await renderLocalProxies({ local_proxy: [] });
	assertGeneratedIds(environment, 'Local proxies');
	environment = await renderNodes({
		node: [
			{ '.name': 'cfg_manual', name: 'Manual' },
			{ '.name': 'jdub_0123456789ab', name: 'Subscribed', source_subscription: 'jdub' }
		],
		route: [
			{ '.name': 'route_proxy', kind: 'single', node: 'jdub_0123456789ab', detour: 'route_jp' },
			{ '.name': 'route_jp', kind: 'single', node: 'cfg_manual' }
		],
		subscription: [ { '.name': 'jdub', name: 'Jdub' } ]
	});
	const groupedNodes = environment.maps[0].sections.find((section) => section.sectionType == 'node');
	assert.ok(groupedNodes && groupedNodes.addremove === true && groupedNodes.filter('cfg_manual') && !groupedNodes.filter('jdub_0123456789ab'),
		'The default node group shows only manually added nodes');
	const groupedPicker = environment.maps[0].sections.flatMap((section) => section.options)
		.find((option) => option.name == 'node');
	assert.ok(groupedPicker && groupedPicker.type == 'RichListValue' && groupedPicker.values.some((value) => value[2]?.includes('Jdub')),
		'Node selectors expose the source subscription as a visual group');
	assert.equal(groupedPicker.textvalue('route_proxy'), 'Subscribed',
		'Route summaries show the node name instead of its internal UCI ID');
	const routeSection = environment.maps[0].sections.find((section) => section.sectionType == 'route');
	const detourPicker = routeSection.options.find((option) => option.name == 'detour');
	assert.deepEqual(detourPicker.values[0], [ '', 'Direct connection' ],
		'Detour picker can clear an existing detour and dial the node directly');
	assert.ok(detourPicker.values.some((value) => value[0] == 'route_proxy'),
		'Detour picker shows every single-node route, including the current route for backend cycle diagnostics');
	await detourPicker.submit('route_proxy', '');
	assert.equal(environment.uci.get('steer', 'route_proxy', 'detour'), undefined,
		'An empty RichListValue is valid and removes the detour UCI option on save');
	await detourPicker.submit('route_proxy', 'route_jp');
	assert.equal(environment.uci.get('steer', 'route_proxy', 'detour'), 'route_jp',
		'A non-empty detour remains writable');
	[ '_route_connect_test', '_route_download_test' ].forEach((name) => {
		const option = routeSection.options.find((candidate) => candidate.name == name);
		assert.ok(option && option.editable && option.default === undefined,
			`Route test action ${name} is visible without a persistent value`);
		assert.equal(option.write('route_proxy', '1'), undefined);
		assert.equal(option.remove('route_proxy'), undefined);
		assert.equal(environment.uci.get('steer', 'route_proxy', name), undefined,
			`Route test action ${name} never enters UCI`);
	});
	const routeTestButton = { disabled: false, textContent: '', title: '', classList: { toggle: () => {} } };
	await routeSection.options.find((option) => option.name == '_route_connect_test')
		.onclick({ currentTarget: routeTestButton }, 'route_proxy');
	assert.deepEqual(environment.routeSpeedtestCalls, [ { route: 'route_proxy', download: false } ],
		'Route chain test passes the route ID to RPC');
	environment = await renderNodes({
		node: [
			{ '.name': 'cfg_manual', name: 'Manual' },
			{ '.name': 'jdub_0123456789ab', name: 'Subscribed', source_subscription: 'jdub' }
		],
		route: [],
		subscription: [ { '.name': 'jdub', name: 'Jdub' } ]
	}, '?node_group=jdub');
	const subscriptionNodes = environment.maps[0].sections.find((section) => section.sectionType == 'node');
	assert.ok(subscriptionNodes && subscriptionNodes.addremove === false && !subscriptionNodes.filter('cfg_manual') && subscriptionNodes.filter('jdub_0123456789ab'),
		'A subscription group renders only that subscription and cannot create manual nodes inside it');
	assert.equal(subscriptionNodes.readonly, true,
		'Subscription nodes render as a compact read-only summary');
	[ 'enabled', 'type', 'server', 'server_port' ].forEach((name) => {
		const option = subscriptionNodes.options.find((candidate) => candidate.name == name);
		assert.equal(option && option.editable, false,
			`Subscription node option ${name} does not create an editable widget per row`);
	});
	[ '_connect_speedtest', '_download_speedtest' ].forEach((name) => {
		const option = subscriptionNodes.options.find((candidate) => candidate.name == name);
		assert.equal(option && option.editable, true,
			`Subscription node exposes the ${name} action`);
		assert.equal(option.default, undefined,
			`Speed-test action ${name} has no value for LuCI to persist`);
		assert.equal(option.write('jdub_0123456789ab', '1'), undefined,
			`Speed-test action ${name} ignores form writes`);
		assert.equal(option.remove('jdub_0123456789ab'), undefined,
			`Speed-test action ${name} ignores form removal`);
		assert.equal(environment.uci.get('steer', 'jdub_0123456789ab', name), undefined,
			`Speed-test action ${name} never enters the UCI node model`);
	});
	const speedtestButton = { disabled: false, textContent: '', title: '', classList: { toggle: () => {} } };
	const connectSpeedtest = subscriptionNodes.options.find((candidate) => candidate.name == '_connect_speedtest');
	await connectSpeedtest.onclick({ currentTarget: speedtestButton }, 'jdub_0123456789ab');
	assert.deepEqual(environment.speedtestCalls, [ { node: 'jdub_0123456789ab', download: false } ],
		'Row speed test passes the section ID instead of the click event to RPC');
	assert.equal(speedtestButton.textContent, '42 ms',
		'Connection result replaces the row test button label');
	assert.ok(speedtestButton.title.includes('HTTP 204') && speedtestButton.title.includes('1 attempt'),
		'Connection result exposes status and attempt diagnostics');
	environment = await renderNodes({
		node: [ { '.name': 'jdub_stale', name: 'Stale', source_subscription: 'jdub' } ],
		route: [],
		subscription: [ { '.name': 'jdub', name: 'Jdub' } ]
	}, '', { subscriptions: [ { id: 'jdub', name: 'Jdub', node_count: 1, stale_node_ids: [ 'jdub_stale' ] } ] });
	const removeStale = findElements(environment.rendered,
		(node) => node.tag == 'button' && node.children?.[0] == 'Remove jdub_stale')[0];
	assert.ok(removeStale, 'Subscription status exposes cleanup only for stale nodes');
	environment.cleanSubscriptionResult = { ok: false, error: 'NODE_STILL_REFERENCED' };
	await removeStale.attributes.click();
	assert.deepEqual(environment.cleanSubscriptionCalls, [ { id: 'jdub', node: 'jdub_stale' } ],
		'Stale cleanup passes subscription and node IDs to RPC');
	assert.equal(environment.window.location.reloadCount, 0,
		'Failed stale cleanup does not reload and hide the backend error');
	assert.equal(environment.notifications[0]?.level, 'danger');
	assert.equal(environment.notifications[0]?.body.children[0], 'NODE_STILL_REFERENCED',
		'Failed stale cleanup shows the backend diagnostic');
	environment.cleanSubscriptionResult = { ok: true };
	await removeStale.attributes.click();
	assert.equal(environment.window.location.reloadCount, 1,
		'Successful stale cleanup reloads the current subscription view');
	environment = await renderOverview({ subscription: [] });
	const probeOptions = [ 'probe_direct', 'probe_proxy', 'speedtest_proxy' ].map((name) =>
		allOptions(environment).find((option) => option.name == name));
	assert.ok(probeOptions.every((option) => option?.type == 'Value' && option.rmempty === false),
		'Schema 7 probe URLs are required scalar fields');
	const subscriptionSection = environment.maps[0].sections.find((section) => section.sectionType == 'subscription');
	assert.ok(subscriptionSection && subscriptionSection.addremove && subscriptionSection.anonymous === false,
		'Subscriptions require an explicit stable UCI section ID');
	assert.equal(typeof subscriptionSection.handleAdd, 'function',
		'Subscription creation validates the stricter Steer ID syntax');
	const overviewTestButtons = findElements(environment.rendered,
		(node) => node.tag == 'button' && typeof node.attributes?.click == 'function');
	assert.equal(overviewTestButtons.length, 3,
		'Overview renders direct, proxy and proxy speed-test actions without requiring a healthy status');
	await overviewTestButtons[1].attributes.click({ preventDefault: () => {}, currentTarget: overviewTestButtons[1] });
	assert.deepEqual(environment.overviewProbeCalls, [ 'proxy' ],
		'Overview proxy test remains clickable when no healthy running status was returned');
	const nodeSource = fs.readFileSync(path.join(root,
		'luci-app-steer/htdocs/luci-static/resources/view/steer/nodes.js'), 'utf8');
	assert.ok(nodeSource.includes("const id = uci.add('steer', 'node');") &&
		!nodeSource.includes("uci.add('steer', 'node', id)"),
		'Share URL import uses an internal UCI-generated node ID');
	const overviewSource = fs.readFileSync(path.join(root,
		'luci-app-steer/htdocs/luci-static/resources/view/steer/overview.js'), 'utf8');
	assert.ok(!overviewSource.includes('renderPlan') && !overviewSource.includes('renderSubscriptions'),
		'Overview keeps only runtime status and configuration');
	const nodesSource = fs.readFileSync(path.join(root,
		'luci-app-steer/htdocs/luci-static/resources/view/steer/nodes.js'), 'utf8');
	assert.ok(nodesSource.includes('steer.updateSubscription(subscription.id)') &&
		nodesSource.includes("_('Update now')") &&
		nodesSource.includes("_('Subscription updated; %d invalid nodes skipped.')") &&
		nodesSource.includes("_('Connection test')") &&
		nodesSource.includes("_('Download test')") &&
		nodesSource.includes("_('Batch connection test')") &&
		nodesSource.includes("_('Batch download test')") &&
		!nodesSource.includes('ui.showModal(testTitle'),
		'Nodes page exposes inline row and batch speed-test actions');
	const steerSource = fs.readFileSync(path.join(root,
		'luci-app-steer/htdocs/luci-static/resources/steer.js'), 'utf8');
	assert.ok(steerSource.includes("params: [ 'node', 'download' ]") &&
		steerSource.includes("params: [ 'route', 'download' ]") &&
		steerSource.includes("params: [ 'kind' ]") &&
		steerSource.includes('speedtest: function(node, download)') &&
		steerSource.includes('routeSpeedtest: function(route, download)') &&
		steerSource.includes('overviewProbe: function(kind)') &&
		steerSource.includes('validate: function()') &&
		!steerSource.includes('callPlan'),
		'LuCI helper exposes diagnostics and independent validation without a plan contract');
	const rpcSource = fs.readFileSync(path.join(root,
		'luci-app-steer/root/usr/share/rpcd/ucode/luci.steer'), 'utf8');
	assert.ok(rpcSource.includes("args: { node: '', download: false }") &&
		rpcSource.includes("args: { route: '', download: false }") &&
		rpcSource.includes("args: { kind: '' }") &&
		rpcSource.includes('request?.args?.node') &&
		rpcSource.includes('request?.args?.route') &&
		rpcSource.includes('request.args.download') &&
		rpcSource.includes("command += ' --download'") &&
		rpcSource.includes('shellquote(node)') &&
		!rpcSource.includes('command_json([') &&
		!rpcSource.includes('rollback') &&
		!rpcSource.includes('steer plan'),
		'RPC backend declares and reads the speed-test arguments');
	assert.ok(rpcSource.includes("args: { id: '' }") &&
		rpcSource.includes("args: { id: '', node: '' }"),
		'Subscription RPC methods declare their input arguments');

	console.log('LuCI view regression tests passed.');
}

main().catch((error) => {
	console.error(error.stack || error);
	process.exit(1);
});
