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
		return E([], [
			E('section', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Runtime')),
				E('dl', { 'class': 'steer-status__facts' }, [
					fact('Steer', runtime.steer),
					fact('sing-box', runtime.sing_box || runtime.sing_box_error),
					fact(_('Canonical schema'), runtime.canonical_schema),
					fact('Generation', status?.last_apply?.result?.generation),
					fact(_('Last Apply'), status?.last_apply?.sequence),
					fact('Geo seed', runtime.geodata?.version || runtime.geodata_error),
					fact('Geo rules', runtime.geodata?.rule_count)
				])
			])
		]);
	}
});
