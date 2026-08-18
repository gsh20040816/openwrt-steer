/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

'use strict';
'require baseclass';
'require rpc';
'require ui';

const callStatus = rpc.declare({
	object: 'luci.steer',
	method: 'status',
	expect: { '': {} }
});

const callApply = rpc.declare({
	object: 'luci.steer',
	method: 'apply',
	expect: { '': {} }
});

const callGeodataUpdate = rpc.declare({
	object: 'luci.steer',
	method: 'geodata_update',
	expect: { '': {} }
});

const callGeodataCatalog = rpc.declare({
	object: 'luci.steer',
	method: 'geodata_catalog',
	expect: { '': {} }
});

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
	return E('p', {}, result?.output || result?.error || _('Apply failed without a diagnostic message.'));
}

function conflictDescription(conflict) {
	let states = [];
	if (conflict.enabled)
		states.push(_('enabled at boot'));
	if (conflict.running)
		states.push(_('currently running'));
	return '%s: %s'.format(conflict.name, states.join(', '));
}

return baseclass.extend({
	loadStyle: function() {
		const id = 'steer-stylesheet';
		if (document.getElementById(id))
			return;
		document.head.appendChild(E('link', {
			id,
			rel: 'stylesheet',
			href: L.resource('steer/steer.css')
		}));
	},

	status: function() {
		return L.resolveDefault(callStatus(), {});
	},

	geodataCatalog: function() {
		return L.resolveDefault(callGeodataCatalog(), {});
	},

	updateGeodata: function() {
		return callGeodataUpdate().then((result) => {
			ui.addNotification(null, E('p', {}, result?.ok ?
				(result.output || _('GeoSite and GeoIP check completed.')) :
				(result.output || result.error || _('GeoSite and GeoIP check failed.'))),
				result?.ok ? 'info' : 'danger');
			return result;
		});
	},

	renderGeodataAlert: function(status) {
		const failed = [ 'geosite', 'geoip' ].filter((kind) =>
			[ 'failed', 'blocked' ].includes(status?.geodata?.[kind]?.state));
		if (!failed.length)
			return null;
		return E('div', { 'class': 'steer-geodata-alert' }, [
			E('strong', {}, _('GeoSite / GeoIP update needs attention')),
			E('ul', {}, failed.map((kind) => E('li', {}, '%s: %s'.format(
				kind == 'geosite' ? 'GeoSite' : 'GeoIP',
				status.geodata[kind].message || status.geodata[kind].state))))
		]);
	},

	apply: function(view, ev, mode) {
		return view.handleSave(ev)
			.then(() => ui.changes.apply(mode == '0'))
			.then(() => {
				ui.showModal(_('Starting Steer'), [
					E('p', { 'class': 'spinning' },
						_('Checking conflicts, generating configuration and verifying services.')),
					E('p', {}, _('Do not start another transparent proxy or the system SmartDNS while this check is running.'))
				]);
				return L.resolveDefault(callApply(), {
					ok: false,
					error: _('The apply request failed before Steer returned a diagnostic message.')
				});
			})
			.then((result) => L.resolveDefault(callStatus(), null).then((status) => ({ result, status })))
			.then(({ result, status }) => {
				if (status != null)
					this.refreshStatus(status);
				ui.hideModal();
				if (!result?.ok) {
					ui.addNotification(_('Steer did not apply the changes'), resultMessage(result), 'danger');
					return result;
				}

				ui.addNotification(null, E('p', {}, result.output || _('Steer configuration applied.')), 'info');
				return result;
			});
	},

	refreshStatus: function(status) {
		const current = document.getElementById('steer-runtime-status');
		if (current)
			current.replaceWith(this.renderStatus(status));
	},

	renderStatus: function(status) {
		const running = status?.core_running && status?.network_loaded &&
			status?.dns_total > 0 && status?.dns_running == status?.dns_total;
		const conflicts = status?.conflicts || [];
		const conflictsMatter = status?.desired_enabled && conflicts.length > 0;
		let headline = _('Traffic steering is not fully active');
		let stateClass = 'is-stopped';
		let panelClass = '';

		if (status?.runtime_state == 'applying' || status?.runtime_state == 'stopping') {
			headline = status.runtime_state == 'applying' ? _('Steer is starting') : _('Steer is stopping');
			stateClass = 'is-starting';
			panelClass = ' steer-status--starting';
		}
		else if (status?.runtime_state == 'failed') {
			headline = _('The last apply failed');
			panelClass = ' steer-status--error';
		}
		else if (conflictsMatter) {
			headline = _('Steer is blocked by conflicting services');
			panelClass = ' steer-status--error';
		}
		else if (!status?.desired_enabled && running) {
			headline = _('Steer is still running while disabled');
			panelClass = ' steer-status--error';
		}
		else if (running) {
			headline = _('Traffic steering is active');
			stateClass = 'is-running';
		}
		else if (!status?.desired_enabled) {
			headline = _('Steer is disabled');
		}
		const showRuntimeMessage = status?.runtime_message && (
			status.runtime_state == 'failed' ||
			(!conflictsMatter && (
				[ 'applying', 'stopping' ].includes(status.runtime_state) ||
				(status.runtime_state == 'active' && running) ||
				(status.runtime_state == 'disabled' && !running))));

		const state = E('div', { 'class': 'steer-status' + panelClass }, [
			E('div', { 'class': 'steer-status__lead' }, [
				E('span', { 'class': 'steer-status__eyebrow' }, _('Current state')),
				E('strong', { 'class': stateClass }, headline),
				showRuntimeMessage ? E('span', { 'class': 'steer-status__message' }, status.runtime_message) : ''
			]),
			E('dl', { 'class': 'steer-status__facts' }, [
				E('div', {}, [ E('dt', {}, _('Configuration switch')), E('dd', {}, status?.desired_enabled ? _('Enabled') : _('Disabled')) ]),
				E('div', {}, [ E('dt', {}, _('Core')), E('dd', {}, status?.core_running ? _('Running') : _('Stopped')) ]),
				E('div', {}, [ E('dt', {}, _('DNS profiles')), E('dd', {}, '%d / %d'.format(status?.dns_running || 0, status?.dns_total || 0)) ]),
				E('div', {}, [ E('dt', {}, _('Network')), E('dd', {}, status?.network_loaded ? _('Running') : _('Stopped')) ]),
				E('div', {}, [ E('dt', {}, _('Recovery point')), E('dd', {}, status?.has_last_known_good ? _('Ready') : _('Not created')) ])
			])
		]);
		const conflictAlert = conflictsMatter ? E('div', { 'class': 'steer-conflict-alert' }, [
			E('strong', {}, _('Stop and disable conflicting services before starting Steer')),
			E('ul', {}, conflicts.map((conflict) => E('li', {}, conflictDescription(conflict)))),
			E('p', {}, _('Steer will not stop or disable another service automatically.'))
		]) : null;
		const alert = this.renderGeodataAlert(status);
		return E('div', { 'id': 'steer-runtime-status' },
			[ conflictAlert, alert, state ].filter((item) => item != null));
	}
});
