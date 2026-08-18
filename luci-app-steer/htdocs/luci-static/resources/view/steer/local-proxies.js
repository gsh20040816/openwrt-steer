/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 */

'use strict';
'require form';
'require uci';
'require view';
'require steer as steer';

return view.extend({
	load: function() {
		return uci.load('steer');
	},

	render: function() {
		let m, s, o;
		steer.loadStyle();

		m = new form.Map('steer', _('Local proxies'),
			_('Create named SOCKS, HTTP or mixed entry points. An entry point does not own a node: ordered rules choose its DNS profile and route, just like transparent traffic. Existing service ports are not imported automatically.'));

		s = m.section(form.GridSection, 'local_proxy', _('Named proxy entry points'));
		s.anonymous = true;
		s.addremove = true;
		s.nodescriptions = true;
		s.addbtntitle = _('Add local proxy');
		s.sectiontitle = function(sectionId) {
			return uci.get('steer', sectionId, 'name') || _('Unnamed');
		};

		o = s.option(form.Flag, 'enabled', _('Enabled'));
		o.default = '1';
		o.editable = true;

		o = s.option(form.Value, 'name', _('Name'));
		o.rmempty = false;
		o.modalonly = true;

		o = s.option(form.ListValue, 'protocol', _('Protocol'));
		o.value('mixed', _('Mixed (SOCKS + HTTP)'));
		o.value('socks', 'SOCKS');
		o.value('http', 'HTTP');
		o.rmempty = false;
		o.editable = true;

		o = s.option(form.Value, 'listen', _('Listen address'));
		o.datatype = 'ipaddr';
		o.placeholder = '127.0.0.1';
		o.rmempty = false;
		o.editable = true;
		o.description = _('Use a loopback address unless LAN clients must connect directly. Non-loopback listeners require authentication.');

		o = s.option(form.Value, 'listen_port', _('Port'));
		o.datatype = 'port';
		o.rmempty = false;
		o.editable = true;

		o = s.option(form.Value, 'username', _('Username'));
		o.modalonly = true;

		o = s.option(form.Value, 'password', _('Password'));
		o.password = true;
		o.modalonly = true;
		o.description = _('Username and password must either both be set or both be empty.');

		return m.render();
	},

	handleSaveApply: function(ev, mode) {
		return steer.apply(this, ev, mode);
	}
});
