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
			.then(() => callApply())
			.then((result) => {
				if (!result?.ok) {
					ui.addNotification(_('Steer did not apply the changes'), resultMessage(result), 'danger');
					return result;
				}

				ui.addNotification(null, E('p', {}, result.output || _('Steer configuration applied.')), 'info');
				return result;
			});
	},

	renderStatus: function(status) {
		const running = status?.core_running && status?.network_loaded &&
			status?.dns_total > 0 && status?.dns_running == status?.dns_total;
		const state = E('div', { 'class': 'steer-status' }, [
			E('div', { 'class': 'steer-status__lead' }, [
				E('span', { 'class': 'steer-status__eyebrow' }, _('Current state')),
				E('strong', { 'class': running ? 'is-running' : 'is-stopped' },
					running ? _('Traffic steering is active') : _('Traffic steering is not fully active'))
			]),
			E('dl', { 'class': 'steer-status__facts' }, [
				E('div', {}, [ E('dt', {}, _('Core')), E('dd', {}, status?.core_running ? _('Running') : _('Stopped')) ]),
				E('div', {}, [ E('dt', {}, _('DNS profiles')), E('dd', {}, '%d / %d'.format(status?.dns_running || 0, status?.dns_total || 0)) ]),
				E('div', {}, [ E('dt', {}, _('Recovery point')), E('dd', {}, status?.has_last_known_good ? _('Ready') : _('Not created')) ])
			])
		]);
		const alert = this.renderGeodataAlert(status);
		return E([], alert ? [ alert, state ] : [ state ]);
	}
});
