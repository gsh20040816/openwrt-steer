/* SPDX-License-Identifier: GPL-3.0-or-later */

'use strict';
'require view';
'require steer as steer';

return view.extend({
	load: function() {
		return steer.intent();
	},

	render: function(intent) {
		steer.loadStyle();
		return E('section', { 'class': 'cbi-section' }, [
			E('h3', {}, _('Canonical Intent preview')),
			E('p', {}, _('OpenWrt keeps UCI as its only configuration source. This read-only Canonical preview shows exactly what the shared compiler receives.')),
			E('pre', { 'class': 'steer-canonical-preview' }, JSON.stringify(intent || {}, null, 2))
		]);
	}
});
