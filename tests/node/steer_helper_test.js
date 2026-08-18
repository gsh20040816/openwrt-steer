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

	return {
		tag,
		attributes: attributes || {},
		children: children == null ? [] : (Array.isArray(children) ? children : [ children ])
	};
}

function textContent(value) {
	if (value == null)
		return '';
	if (typeof value != 'object')
		return String(value);
	return (value.children || []).map(textContent).join(' ');
}

function loadHelper(runtime) {
	const source = fs.readFileSync(path.join(root,
		'luci-app-steer/htdocs/luci-static/resources/steer.js'), 'utf8');
	const baseclass = { extend: (value) => value };
	const rpc = {
		declare: ({ method }) => () => {
			if (method == 'status') {
				runtime.statusCalls++;
				return Promise.resolve(runtime.status);
			}
			if (method == 'apply')
				return Promise.resolve(runtime.applyResult);
			return Promise.resolve({});
		}
	};
	const ui = {
		changes: { apply: () => Promise.resolve() },
		showModal: (title) => { runtime.modalTitle = title; },
		hideModal: () => { runtime.modalHidden = true; },
		addNotification: (title, content, level) => runtime.notifications.push({ title, content, level })
	};
	const L = {
		resolveDefault: (promise, fallback) => Promise.resolve(promise).catch(() => fallback),
		resource: (value) => value
	};
	const document = {
		head: { appendChild: () => {} },
		getElementById: (id) => id == 'steer-runtime-status' ? runtime.currentStatusNode : null
	};
	const translate = (value) => String(value);

	return new Function('baseclass', 'rpc', 'ui', 'E', '_', 'L', 'document', source)(
		baseclass, rpc, ui, element, translate, L, document);
}

async function main() {
	const runtime = {
		status: {},
		applyResult: { ok: true, output: 'applied' },
		statusCalls: 0,
		notifications: [],
		currentStatusNode: {
			replaceWith: (value) => { runtime.replacement = value; }
		}
	};
	const helper = loadHelper(runtime);

	let rendered = helper.renderStatus({
		desired_enabled: false,
		core_running: false,
		dns_running: 0,
		dns_total: 0,
		conflicts: [ { name: 'passwall2', enabled: true, running: true } ]
	});
	assert.ok(textContent(rendered).includes('Steer is disabled'),
		'An intentionally disabled Steer reports a disabled state');
	assert.ok(!textContent(rendered).includes('Stop and disable conflicting services'),
		'Alternative services are not shown as errors while Steer is disabled');

	rendered = helper.renderStatus({
		desired_enabled: true,
		runtime_state: 'active',
		runtime_message: 'stale active message',
		core_running: false,
		network_loaded: false,
		dns_running: 0,
		dns_total: 0,
		conflicts: [ { name: 'smartdns', enabled: true, running: false } ]
	});
	assert.ok(!textContent(rendered).includes('stale active message'),
		'A stale lifecycle message cannot contradict live process and conflict status');

	rendered = helper.renderStatus({
		desired_enabled: true,
		runtime_state: 'failed',
		runtime_message: 'system SmartDNS conflict',
		core_running: false,
		dns_running: 0,
		dns_total: 2,
		conflicts: [ { name: 'smartdns', enabled: true, running: true } ]
	});
	const failedText = textContent(rendered);
	assert.ok(failedText.includes('The last apply failed') &&
		failedText.includes('smartdns: enabled at boot, currently running') &&
		failedText.includes('system SmartDNS conflict'),
		'Failed apply status exposes both the persisted reason and exact conflicting service state');

	runtime.status = {
		desired_enabled: true,
		runtime_state: 'active',
		core_running: true,
		network_loaded: true,
		dns_running: 2,
		dns_total: 2,
		conflicts: []
	};
	await helper.apply({ handleSave: () => Promise.resolve() }, null, '1');
	assert.equal(runtime.modalTitle, 'Starting Steer',
		'Apply shows an explicit startup progress modal');
	assert.equal(runtime.modalHidden, true, 'Apply closes the progress modal after RPC completion');
	assert.equal(runtime.statusCalls, 1, 'Apply immediately reloads runtime status');
	assert.ok(textContent(runtime.replacement).includes('Traffic steering is active'),
		'Apply replaces stale overview status with the final runtime result');
	assert.equal(runtime.notifications.at(-1).level, 'info');

	console.log('Steer LuCI helper regression tests passed.');
}

main().catch((error) => {
	console.error(error.stack || error);
	process.exit(1);
});
