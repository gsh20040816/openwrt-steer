/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require view';
'require steer as steer';

function fact(label, value) {
	return E('div', {}, [ E('dt', {}, label), E('dd', {}, value == null || value === '' ? '—' : String(value)) ]);
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
		return E([], [
			E('section', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Runtime')),
				E('dl', { 'class': 'steer-status__facts' }, [
					fact(_('Steer'), runtime.steer),
					fact('sing-box', singBox.version || singBox.error || runtime.sing_box_error),
					fact(_('Running status'), status.healthy ? _('Normal') : (status.generation ? _('Abnormal') : _('Stopped'))),
					fact(_('Last Apply'), lastApply ? (lastApply.result?.ok ? _('Succeeded') : _('Failed')) : null),
					fact(_('Geo seed'), runtime.geodata?.version || runtime.geodata_error),
					fact(_('Geo rules'), runtime.geodata?.rule_count)
				])
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
