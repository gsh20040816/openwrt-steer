/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require view';
'require steer as steer';
'require steer.ui-spec as uiSpec';

function fact(label, value) {
	return E('div', {}, [ E('dt', {}, label), E('dd', {}, value == null || value === '' ? '—' : String(value)) ]);
}

function boundaryModeLabel(mode) {
	return {
		dedicated_shim: _('Dedicated DNS shim'),
		tun_port53_hijack: _('TUN port-53 hijack')
	}[mode] || mode;
}

return view.extend({
	load: function() {
		return Promise.all([ steer.runtime(), steer.status() ]);
	},

	render: function(data) {
		steer.loadStyle();
		const runtime = data[0] || {};
		const status = data[1] || {};
		const singBox = runtime.sing_box || {};
		const lastApply = status.last_apply;
		const boundary = uiSpec.dns_boundaries.openwrt;
		return E([], [
			E('section', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Runtime')),
				E('dl', { 'class': 'steer-status__facts' }, [
					fact(_('Steer'), runtime.steer),
					fact('sing-box', singBox.version || singBox.error || runtime.sing_box_error),
					fact(_('Build tags'), Array.isArray(singBox.tags) ? singBox.tags.join(' / ') : null),
					fact(_('Canonical schema'), runtime.canonical_schema),
					fact(_('Generation'), status.generation),
					fact(_('Last Apply'), lastApply ? '%s · %s'.format(lastApply.sequence, lastApply.result?.ok ? _('Succeeded') : _('Failed')) : null),
					fact(_('Geo seed'), runtime.geodata?.version || runtime.geodata_error),
					fact(_('Geo rules'), runtime.geodata?.rule_count)
				])
			]),
			E('section', { 'class': 'cbi-section' }, [
				E('h3', {}, _('DNS capture boundary')),
				E('dl', { 'class': 'steer-status__facts' }, [
					fact(_('Mode'), boundaryModeLabel(boundary.capture_mode)),
					fact(_('Capture scope'), _(boundary.capture_scope)),
					fact(_('Exclusions'), boundary.exclusions.map((item) => _(item)).join(' · '))
				]),
				E('p', {}, _(boundary.encrypted_dns_boundary))
			]),
			E('section', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Platform components and paths')),
				E('dl', { 'class': 'steer-status__facts' }, [
					fact(_('Service'), 'procd · steer'),
					fact(_('Configuration'), '/etc/config/steer'),
					fact(_('Runtime directory'), '/run/steer'),
					fact(_('State directory'), '/var/lib/steer'),
					fact(_('Geo seed path'), '/usr/share/steer/geodata-seed')
				])
			])
		]);
	}
});
