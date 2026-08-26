/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require baseclass';
'require rpc';
'require uci';
'require ui';

const callStatus = rpc.declare({ object: 'luci.steer', method: 'status', expect: { '': {} } });
const callOverviewState = rpc.declare({ object: 'luci.steer', method: 'overview_state', expect: { '': {} } });
const callApplySaved = rpc.declare({ object: 'luci.steer', method: 'apply_saved', expect: { '': {} } });
const callValidate = rpc.declare({ object: 'luci.steer', method: 'validate', expect: { '': {} } });
const callCommitCandidate = rpc.declare({ object: 'luci.steer', method: 'commit_candidate', expect: { '': {} } });
const callIntentPreview = rpc.declare({ object: 'luci.steer', method: 'intent_preview', params: [ 'reveal' ], expect: { '': {} } });
const callRuntime = rpc.declare({ object: 'luci.steer', method: 'runtime', expect: { '': {} } });
const callLogs = rpc.declare({ object: 'luci.steer', method: 'logs', expect: { '': {} } });
const callDiagnostics = rpc.declare({ object: 'luci.steer', method: 'diagnostics', expect: { '': {} } });
const callGeodataCatalog = rpc.declare({ object: 'luci.steer', method: 'geodata_catalog', expect: { '': {} } });
const callSubscriptions = rpc.declare({ object: 'luci.steer', method: 'subscriptions', expect: { '': {} } });
const callSubscriptionUpdate = rpc.declare({ object: 'luci.steer', method: 'subscription_update', params: [ 'id' ], expect: { '': {} } });
const callSubscriptionClean = rpc.declare({ object: 'luci.steer', method: 'subscription_clean', params: [ 'id', 'node' ], expect: { '': {} } });
const callNodeSpeedtest = rpc.declare({ object: 'luci.steer', method: 'node_speedtest', params: [ 'node', 'download' ], expect: { '': {} } });
const callRouteSpeedtest = rpc.declare({ object: 'luci.steer', method: 'route_speedtest', params: [ 'route', 'download' ], expect: { '': {} } });
const callOverviewProbe = rpc.declare({ object: 'luci.steer', method: 'overview_probe', params: [ 'kind' ], expect: { '': {} } });
const callNodeImport = rpc.declare({ object: 'luci.steer', method: 'node_import', params: [ 'document' ], expect: { '': {} } });
const sectionIDPattern = /^[a-z][a-z0-9_]{0,31}$/;

function issueText(issue) {
	let target = issue.object_type || _('Configuration');
	if (issue.object_id)
		target += ' “%s”'.format(issue.object_id);
	if (issue.option)
		target += ' / %s'.format(issue.option);
	return '%s: %s'.format(target, issue.message || issue.code);
}

function resultMessage(result) {
	if (result?.validation?.errors?.length)
		return E('ul', {}, result.validation.errors.map((issue) => E('li', {}, issueText(issue))));
	return E('p', {}, result?.error || _('Apply failed without a diagnostic message.'));
}

function waitForApply(sequence, attempts) {
	return L.resolveDefault(callStatus(), null).then((status) => {
		if (status?.last_apply?.sequence && status.last_apply.sequence !== sequence)
			return { result: status.last_apply.result, status };
		if (attempts <= 0)
			return { result: { ok: false, error: _('Timed out waiting for the committed Steer configuration to reload.') }, status };
		return new Promise((resolve) => window.setTimeout(resolve, 250))
			.then(() => waitForApply(sequence, attempts - 1));
	});
}

return baseclass.extend({
	loadStyle: function() {
		if (!document.getElementById('steer-stylesheet'))
			document.head.appendChild(E('link', { id: 'steer-stylesheet', rel: 'stylesheet', href: L.resource('steer/steer.css') }));
		this.mountLifecycleBar();
	},

	mountLifecycleBar: function() {
		const existing = document.getElementById('steer-lifecycle-global');
		existing?.remove();
		const page = (window.location.pathname || '').split('/').pop();
		if ([ 'overview', 'steer', 'diagnostics', 'system' ].includes(page)) return;
		const host = document.getElementById('maincontent');
		if (!host?.prepend) return;
		this.overviewState().then((state) => {
			if (state?.ok !== true) return;
			const active = state.active || {};
			const actions = [];
			const run = (button, operation) => {
				button.disabled = true;
				operation().then((result) => {
					if (result?.ok === false) throw new Error(result.error || _('Operation failed.'));
					window.location.reload();
				}).catch((error) => {
					button.disabled = false;
					ui.addNotification(_('Operation failed'), E('p', {}, String(error)), 'danger');
				});
			};
			if (state.pending) {
				const apply = E('button', { 'class': 'btn cbi-button-positive' }, _('Save & Apply pending changes'));
				const discard = E('button', { 'class': 'btn cbi-button-negative' }, _('Discard pending changes'));
				apply.addEventListener('click', () => run(apply, () => this.applyPending()));
				discard.addEventListener('click', () => run(discard, () => this.discardPending()));
				actions.push(apply, ' ', discard);
			}
			else if (state.pending_apply) {
				const applySaved = E('button', { 'class': 'btn cbi-button-positive' }, _('Apply Saved configuration'));
				applySaved.addEventListener('click', () => run(applySaved, () => this.applySaved()));
				actions.push(applySaved);
			}
			const bar = E('section', { id: 'steer-lifecycle-global', 'class': 'cbi-section' }, [
				E('strong', {}, _('Draft / Saved / Active')),
				E('span', {}, ' · ' + (state.pending ? _('Pending Draft') : _('Draft clean'))),
				E('span', {}, ' · ' + _('Saved %s').format(state.saved?.digest ? state.saved.digest.slice(0, 12) : '—')),
				E('span', {}, ' · ' + _('Active %s').format(active.generation || _('stopped'))),
				state.pending_apply ? E('span', { 'class': 'label warning' }, ' pending_apply') : '',
				...actions
			]);
			host.prepend(bar);
		});
	},

	status: function() { return L.resolveDefault(callStatus(), {}); },
	overviewState: function() { return L.resolveDefault(callOverviewState(), {}); },
	applySaved: function() { return callApplySaved(); },
	validate: function() { return L.resolveDefault(callValidate(), {}); },
	commitCandidate: function() { return L.resolveDefault(callCommitCandidate(), {}); },
	intentPreview: function(reveal) { return L.resolveDefault(callIntentPreview(reveal === true), {}); },
	runtime: function() { return L.resolveDefault(callRuntime(), {}); },
	logs: function() { return L.resolveDefault(callLogs(), {}); },
	diagnostics: function() { return L.resolveDefault(callDiagnostics(), {}); },
	geodataCatalog: function() { return L.resolveDefault(callGeodataCatalog(), {}); },
	subscriptions: function() { return L.resolveDefault(callSubscriptions(), {}); },
	updateSubscription: function(id) { return callSubscriptionUpdate(id); },
	cleanSubscription: function(id, node) { return callSubscriptionClean(id, node); },
	speedtest: function(node, download) { return callNodeSpeedtest(node, download); },
	routeSpeedtest: function(route, download) { return callRouteSpeedtest(route, download); },
	overviewProbe: function(kind) { return callOverviewProbe(kind); },
	importNodes: function(document) { return callNodeImport(document); },

	configureNamedSection: function(section, defaults, beforeSectionId) {
		section.anonymous = false;
		section.handleAdd = function(ev, sectionId) {
			if (!sectionIDPattern.test(sectionId)) {
				ui.addNotification(_('Invalid section ID'), E('p', {}, _('Use 1–32 lowercase characters beginning with a letter.')), 'danger');
				return;
			}
			const config = this.uciconfig || this.map.config;
			this.map.data.add(config, this.sectiontype, sectionId);
			Object.entries(defaults || {}).forEach((entry) => this.map.data.set(config, sectionId, entry[0], entry[1]));
			if (beforeSectionId)
				this.map.data.move(config, sectionId, beforeSectionId, false);
			return this.map.save(null, true);
		};
		return section;
	},

	apply: function(view, ev, mode) {
		let previousSequence = '';
		return L.resolveDefault(callStatus(), {})
			.then((status) => { previousSequence = status?.last_apply?.sequence || ''; })
			.then(() => view.handleSave(ev))
			.then(() => this.commitCandidate())
			.then((commit) => {
				const validation = commit?.validation;
				if (commit?.committed !== true) {
					return uci.changes()
						.then((changes) => ui.changes.renderChangeIndicator(changes))
						.then(() => {
							const result = { ok: false, saved: false, validation, error: commit?.error };
							ui.addNotification(_('Steer rejected the candidate'), resultMessage(result), 'danger');
							return result;
						});
				}
				return uci.changes()
					.then((changes) => ui.changes.renderChangeIndicator(changes))
					.then(() => {
						window.dispatchEvent?.(new Event('steer-uci-state-changed'));
						ui.showModal(_('Applying Steer'), [ E('p', { 'class': 'spinning' }, _('Compiling and verifying the candidate configuration.')) ]);
						return waitForApply(previousSequence, 240);
					})
					.then(({ result, status }) => {
						if (status)
							this.refreshStatus(status, validation);
						ui.hideModal();
						if (!result?.ok) {
							ui.addNotification(_('Steer rejected the candidate'), resultMessage(result), 'danger');
							return result;
						}
						ui.addNotification(null, E('p', {}, _('Steer configuration applied.')), 'info');
						return result;
					});
			});
	},

	applyPending: function() {
		let previousSequence = '';
		return L.resolveDefault(callStatus(), {})
			.then((status) => { previousSequence = status?.last_apply?.sequence || ''; })
			.then(() => this.commitCandidate())
			.then((commit) => {
				const validation = commit?.validation;
				if (commit?.committed !== true) {
					const result = { ok: false, saved: false, validation, error: commit?.error };
					ui.addNotification(_('Steer rejected the candidate'), resultMessage(result), 'danger');
					return result;
				}
				return uci.changes()
					.then((changes) => ui.changes.renderChangeIndicator(changes))
					.then(() => {
						window.dispatchEvent?.(new Event('steer-uci-state-changed'));
						ui.showModal(_('Applying Steer'), [ E('p', { 'class': 'spinning' }, _('Compiling and verifying the candidate configuration.')) ]);
						return waitForApply(previousSequence, 240);
					})
					.then(({ result, status }) => {
						if (status) this.refreshStatus(status, validation);
						ui.hideModal();
						if (!result?.ok) {
							ui.addNotification(_('Steer rejected the candidate'), resultMessage(result), 'danger');
							return result;
						}
						ui.addNotification(null, E('p', {}, _('Steer configuration applied.')), 'info');
						return result;
					});
			});
	},

	discardPending: function() {
		return uci.revert('steer')
			.then(() => uci.load('steer'))
			.then(() => uci.changes())
			.then((changes) => ui.changes.renderChangeIndicator(changes))
			.then(() => window.dispatchEvent?.(new Event('steer-uci-state-changed')));
	},

	refreshStatus: function(status, validation) {
		const current = document.getElementById('steer-runtime-status');
		if (current)
			current.replaceWith(this.renderStatus(status, validation, uci.get('steer', 'main', 'enabled') == '1'));
	},

	renderStatus: function(status, validation, desiredEnabled) {
		const valid = validation?.ok === true;
		let headline = _('Steer is disabled');
		let stateClass = 'is-stopped';
		let panelClass = '';
		if (!valid) {
			headline = _('The saved configuration is invalid');
			panelClass = ' steer-status--error';
		}
		else if (!desiredEnabled) {
			headline = _('Steer is disabled');
		}
		else if (status?.healthy) {
			headline = _('Traffic steering is active');
			stateClass = 'is-running';
		}
		else {
			headline = _('Traffic steering is not healthy');
			panelClass = ' steer-status--error';
		}
		return E('div', { 'id': 'steer-runtime-status' }, E('div', { 'class': 'steer-status' + panelClass }, [
			E('div', { 'class': 'steer-status__lead' }, [ E('span', { 'class': 'steer-status__eyebrow' }, _('Current state')), E('strong', { 'class': stateClass }, headline) ]),
			!valid && validation?.errors?.length ? E('ul', {}, validation.errors.map((issue) => E('li', {}, issueText(issue)))) : ''
		]));
	}
});
