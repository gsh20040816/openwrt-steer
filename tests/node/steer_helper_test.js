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
		declare: ({ method }) => (...args) => {
			if (method == 'commit') {
				assert.deepEqual(args, [ 'steer' ]);
				runtime.sequence.push('commit');
				runtime.commitCalls++;
				return Promise.resolve(0);
			}
			if (method == 'status') {
				runtime.statusCalls++;
				return Promise.resolve(Object.assign({}, runtime.status, {
					last_apply: runtime.commitCalls > 0 ?
						{ sequence: '11', result: runtime.applyResult } :
						{ sequence: '10', result: { ok: true } }
				}));
			}
			if (method == 'rollback') {
				runtime.rollbackCalls++;
				return Promise.resolve(runtime.rollbackResult);
			}
			if (method == 'apply')
				throw new Error('LuCI must not start a second Apply after UCI commit triggered procd');
			return Promise.resolve({});
		}
	};
	const ui = {
		changes: {
			apply: () => { throw new Error('ui.changes.apply() is non-awaitable and must not be used'); },
			renderChangeIndicator: () => { runtime.indicatorRefreshed = true; }
		},
		showModal: (title, content) => { runtime.modalTitle = title; runtime.modalContent = content; },
		hideModal: () => { runtime.modalHidden = true; },
		addNotification: (title, content, level) => runtime.notifications.push({ title, content, level }),
		createHandlerFn: (context, handler) => (...args) => handler.apply(context, args)
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
	const uci = { changes: () => Promise.resolve({}) };
	const window = { setTimeout: (callback) => callback(), location: { reload: () => { runtime.reloaded = true; } } };

	return new Function('baseclass', 'rpc', 'uci', 'ui', 'E', '_', 'L', 'document', 'window', source)(
		baseclass, rpc, uci, ui, element, translate, L, document, window);
}

async function main() {
	const runtime = {
		status: {},
		applyResult: { ok: true, output: 'applied' },
		statusCalls: 0,
		commitCalls: 0,
		rollbackCalls: 0,
		rollbackResult: { ok: true },
		sequence: [],
		notifications: [],
		currentStatusNode: {
			replaceWith: (value) => { runtime.replacement = value; }
		}
	};
	const helper = loadHelper(runtime);

	let rendered = helper.renderStatus({
		desired_enabled: false,
		validation: { ok: true, errors: [], warnings: [] },
		core_running: false,
		tun_ready: false,
		firewall_ready: false,
		listeners_ready: false
	});
	assert.ok(textContent(rendered).includes('Steer is disabled'),
		'An intentionally disabled Steer reports a disabled state');

	rendered = helper.renderStatus({
		desired_enabled: true,
		validation: { ok: true, errors: [], warnings: [] },
		core_running: false,
		tun_ready: false,
		firewall_ready: false,
		listeners_ready: false,
		healthy: false
	});
	assert.ok(textContent(rendered).includes('Traffic steering is not healthy'),
		'Live component readiness determines unhealthy state');

	rendered = helper.renderStatus({
		desired_enabled: true,
		validation: { ok: false, errors: [ { code: 'DANGLING_ROUTE', object_type: 'rule', object_id: 'broken', option: 'route', message: 'route is missing' } ], warnings: [] }
	});
	const failedText = textContent(rendered);
	assert.ok(failedText.includes('The saved configuration is invalid') && failedText.includes('route is missing'),
		'Invalid saved intent exposes the exact validation issue');

	runtime.status = {
		desired_enabled: true,
		validation: { ok: true, errors: [], warnings: [] },
		core_running: true,
		tun_ready: true,
		firewall_ready: true,
		listeners_ready: true,
		healthy: true
	};
	await helper.apply({ handleSave: () => { runtime.sequence.push('save'); return Promise.resolve(); } }, null, '1');
	assert.deepEqual(runtime.sequence, [ 'save', 'commit' ],
		'LuCI saves and commits once, leaving the resulting config.change Apply to procd');
	assert.equal(runtime.indicatorRefreshed, true, 'LuCI refreshes the pending change indicator after commit');
	assert.equal(runtime.modalTitle, 'Applying Steer',
		'Apply shows an explicit transaction progress modal');
	assert.equal(runtime.modalHidden, true, 'Apply closes the progress modal after RPC completion');
	assert.equal(runtime.statusCalls, 2, 'LuCI snapshots the Apply sequence and observes exactly one newer result');
	assert.ok(textContent(runtime.replacement).includes('Traffic steering is active'),
		'Apply replaces stale overview status with the final runtime result');
	assert.equal(runtime.notifications.at(-1).level, 'info');

	rendered = helper.renderStatus({
		desired_enabled: true,
		validation: { ok: true, errors: [], warnings: [] },
		rollback_available: true
	});
	assert.ok(textContent(rendered).includes('Restore previous configuration'),
		'Status exposes the single-use rollback action when a backup exists');
	helper.confirmRollback();
	const actions = runtime.modalContent.at(-1).children;
	const confirm = actions.find((child) => child?.attributes?.class?.includes('negative'));
	await confirm.attributes.click();
	assert.equal(runtime.rollbackCalls, 1, 'LuCI rollback invokes the single backend rollback command once');
	assert.equal(runtime.reloaded, true, 'Successful rollback reloads the form from restored UCI');
	assert.equal(runtime.notifications.at(-1).level, 'info');

	console.log('Steer LuCI helper regression tests passed.');
}

main().catch((error) => {
	console.error(error.stack || error);
	process.exit(1);
});
