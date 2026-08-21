/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require baseclass';
'require rpc';
'require uci';
'require ui';

const callStatus = rpc.declare({ object: 'luci.steer', method: 'status', expect: { '': {} } });
const callPlan = rpc.declare({ object: 'luci.steer', method: 'plan', expect: { '': {} } });
const callGeodataCatalog = rpc.declare({ object: 'luci.steer', method: 'geodata_catalog', expect: { '': {} } });
const callSubscriptions = rpc.declare({ object: 'luci.steer', method: 'subscriptions', expect: { '': {} } });
const callSubscriptionUpdate = rpc.declare({ object: 'luci.steer', method: 'subscription_update', params: [ 'id' ], expect: { '': {} } });
const callSubscriptionClean = rpc.declare({ object: 'luci.steer', method: 'subscription_clean', params: [ 'id', 'node' ], expect: { '': {} } });
const callNodeSpeedtest = rpc.declare({ object: 'luci.steer', method: 'node_speedtest', params: [ 'node' ], expect: { '': {} } });
const callRollback = rpc.declare({ object: 'luci.steer', method: 'rollback', expect: { '': {} } });
const callUCICommit = rpc.declare({ object: 'uci', method: 'commit', params: [ 'config' ], expect: { '': 0 } });

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

function stateText(value) {
	return value ? _('Ready') : _('Not ready');
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
		if (document.getElementById('steer-stylesheet'))
			return;
		document.head.appendChild(E('link', { id: 'steer-stylesheet', rel: 'stylesheet', href: L.resource('steer/steer.css') }));
	},

	status: function() { return L.resolveDefault(callStatus(), {}); },
	plan: function() { return L.resolveDefault(callPlan(), {}); },
	geodataCatalog: function() { return L.resolveDefault(callGeodataCatalog(), {}); },
	subscriptions: function() { return L.resolveDefault(callSubscriptions(), {}); },
	updateSubscription: function(id) { return callSubscriptionUpdate(id); },
	cleanSubscription: function(id, node) { return callSubscriptionClean(id, node); },
	speedtest: function(node) { return callNodeSpeedtest(node); },

	apply: function(view, ev, mode) {
		let previousSequence = '';
		return L.resolveDefault(callStatus(), {})
			.then((status) => { previousSequence = status?.last_apply?.sequence || ''; })
			.then(() => view.handleSave(ev))
			.then(() => callUCICommit('steer'))
			.then(() => uci.changes())
			.then((changes) => ui.changes.renderChangeIndicator(changes))
			.then(() => {
				ui.showModal(_('Applying Steer'), [ E('p', { 'class': 'spinning' }, _('Compiling and verifying the candidate execution plan.')) ]);
				return waitForApply(previousSequence, 240);
			})
			.then(({ result, status }) => {
				if (status)
					this.refreshStatus(status);
				ui.hideModal();
				if (!result?.ok) {
					ui.addNotification(_('Steer rejected the candidate'), resultMessage(result), 'danger');
					return result;
				}
				ui.addNotification(null, E('p', {}, _('Steer configuration applied.')), 'info');
				return result;
			});
	},

	confirmRollback: function() {
		ui.showModal(_('Restore previous Steer configuration?'), [
			E('p', {}, _('This restores the single saved configuration and applies it immediately. The backup is deleted after a successful restore.')),
			E('div', { 'class': 'right' }, [
				E('button', { 'class': 'btn', 'click': ui.hideModal }, _('Cancel')),
				' ',
				E('button', {
					'class': 'btn cbi-button-negative',
					'click': ui.createHandlerFn(this, function() {
						ui.showModal(_('Restoring Steer'), [ E('p', { 'class': 'spinning' }, _('Restoring and applying the previous configuration.')) ]);
						return callRollback()
							.then((result) => callStatus().then((status) => ({ result, status })))
							.then(({ result, status }) => {
								this.refreshStatus(status);
								ui.hideModal();
								if (!result?.ok) {
									ui.addNotification(_('Steer restore failed'), resultMessage(result), 'danger');
									return result;
								}
								ui.addNotification(null, E('p', {}, _('Previous Steer configuration restored.')), 'info');
								window.location.reload();
								return result;
							});
					})
				}, _('Restore previous configuration'))
			])
		]);
	},

	refreshStatus: function(status) {
		const current = document.getElementById('steer-runtime-status');
		if (current)
			current.replaceWith(this.renderStatus(status));
	},

	renderStatus: function(status) {
		const valid = status?.validation?.ok === true;
		let headline = _('Steer is disabled');
		let stateClass = 'is-stopped';
		let panelClass = '';
		if (!valid) {
			headline = _('The saved configuration is invalid');
			panelClass = ' steer-status--error';
		}
		else if (status?.healthy) {
			headline = _('Traffic steering is active');
			stateClass = 'is-running';
		}
		else if (status?.desired_enabled) {
			headline = _('Traffic steering is not healthy');
			panelClass = ' steer-status--error';
		}
		const facts = [
			[ _('Configuration'), valid ? _('Valid') : _('Invalid') ],
			[ _('Core process'), stateText(status?.core_running) ],
			[ _('TUN interface'), stateText(status?.tun_ready) ],
			[ _('Firewall shim'), stateText(status?.firewall_ready) ],
			[ _('Listeners'), stateText(status?.listeners_ready) ]
		];
		return E('div', { 'id': 'steer-runtime-status' }, E('div', { 'class': 'steer-status' + panelClass }, [
			E('div', { 'class': 'steer-status__lead' }, [ E('span', { 'class': 'steer-status__eyebrow' }, _('Current state')), E('strong', { 'class': stateClass }, headline) ]),
			E('dl', { 'class': 'steer-status__facts' }, facts.map((fact) => E('div', {}, [ E('dt', {}, fact[0]), E('dd', {}, fact[1]) ]))),
			!valid && status?.validation?.errors?.length ? E('ul', {}, status.validation.errors.map((issue) => E('li', {}, issueText(issue)))) : '',
			status?.rollback_available ? E('p', {}, E('button', {
				'class': 'btn cbi-button cbi-button-negative',
				'click': ui.createHandlerFn(this, this.confirmRollback)
			}, _('Restore previous configuration'))) : ''
		]));
	}
});
