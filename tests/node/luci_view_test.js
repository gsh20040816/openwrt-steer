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
	return { tag, attributes: attributes || {}, children: values };
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

		value(key, label) {
			this.values.push([ String(key), String(label == null ? key : label) ]);
		}

		depends() {}
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
	const steer = {
		loadStyle: () => {},
		status: () => Promise.resolve({}),
		apply: () => Promise.resolve()
	};
	const ui = {};
	const shareUrl = {};
	const translate = (value) => String(value);

	return { form, uci, view, steer, ui, shareUrl, maps, translate };
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
	await view.render([ null, {}, catalog ]);
	return environment;
}

async function renderNodes(sections) {
	const environment = createEnvironment(sections);
	const view = loadView(
		'luci-app-steer/htdocs/luci-static/resources/view/steer/nodes.js',
		{
			form: environment.form,
			uci: environment.uci,
			ui: environment.ui,
			view: environment.view,
			steer: environment.steer,
			shareUrl: environment.shareUrl,
			E: element,
			_: environment.translate
		}
	);
	await view.render();
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
			outbound: 'direct'
		} ],
		dns_profile: [ { '.name': 'direct_dns' } ],
		outbound: [ { '.name': 'direct' } ],
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
		defaultSection.options.map((option) => option.name).join(',') == 'dns_profile,outbound',
		'Default is a fixed non-modal row that exposes only DNS profile and route');
	assert.equal(options.some((option) => option.name == 'default'), false,
		'Ordinary rules cannot be converted into or edit the Default rule');

	const geoSections = {
		rule: [ {
			'.name': 'geo_rule',
			domain_match: [ 'domain:example.com', 'geosite:category-example' ],
			ip_match: [ '192.0.2.0/24', 'geoip:example' ],
			dns_profile: 'direct_dns',
			outbound: 'direct'
		}, {
			'.name': 'default', default: '1', dns_profile: 'direct_dns', outbound: 'direct'
		} ],
		dns_profile: [ { '.name': 'direct_dns' } ],
		outbound: [ { '.name': 'direct' } ],
		local_proxy: []
	};
	environment = await renderRules(geoSections, {
		geosite: { ok: true, names: [ 'category-example', 'category-media@test' ] },
		geoip: { ok: true, names: [ 'example', 'private' ] }
	});
	options = allOptions(environment);
	const domainMatch = options.find((option) => option.name == 'domain_match');
	const ipMatch = options.find((option) => option.name == 'ip_match');
	assert.ok(domainMatch && ipMatch && domainMatch.type == ipMatch.type,
		'Domain and destination IP use the same multiline match editor');
	assert.deepEqual(domainMatch.editorSuggestions('domain', 'geosite:media', domainMatch.catalogNames),
		[ 'geosite:category-media@test' ],
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
		'Known GeoSite names pass editor validation');
	assert.notEqual(domainMatch.validate('geo_rule', 'geosite:not-present'), true,
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
			outbound: 'direct'
		} ],
		dns_profile: [ { '.name': 'direct_dns' } ],
		outbound: [ { '.name': 'direct' } ],
		local_proxy: []
	});
	options = allOptions(environment);
	const inbound = options.find((option) => option.name == 'inbound');
	assert.ok(inbound, 'A dangling inbound reference must remain visible for repair');
	assert.deepEqual(inbound.values, [ [ 'missing_proxy', 'Missing: missing_proxy' ] ]);

	environment = await renderNodes({
		node: [],
		outbound: [
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
		outbound: [ { '.name': 'broken', kind: 'single', node: 'missing_node' } ]
	});
	options = allOptions(environment);
	const missingNode = options.find((option) => option.name == 'node');
	assert.ok(missingNode, 'A dangling route node must remain visible for repair');
	assert.deepEqual(missingNode.values, [ [ 'missing_node', 'Missing: missing_node' ] ]);

	environment = await renderDns({ dns_profile: [], dns_server: [] });
	assertGeneratedIds(environment, 'DNS profiles and servers');
	environment = await renderLocalProxies({ local_proxy: [] });
	assertGeneratedIds(environment, 'Local proxies');

	const nodeSource = fs.readFileSync(path.join(root,
		'luci-app-steer/htdocs/luci-static/resources/view/steer/nodes.js'), 'utf8');
	assert.ok(nodeSource.includes("const id = uci.add('steer', 'node');") &&
		!nodeSource.includes("uci.add('steer', 'node', id)"),
		'Share URL import uses an internal UCI-generated node ID');

	console.log('LuCI view regression tests passed.');
}

main().catch((error) => {
	console.error(error.stack || error);
	process.exit(1);
});
