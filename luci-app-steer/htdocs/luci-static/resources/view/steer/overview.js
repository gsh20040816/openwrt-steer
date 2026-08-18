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
		return Promise.all([
			uci.load('steer'),
			uci.load('firewall'),
			steer.status()
		]);
	},

	render: function(data) {
		let m, s, o;
		const status = data[2];
		steer.loadStyle();

		m = new form.Map('steer', _('Steer'),
			_('Choose which local firewall zones Steer manages. Rules, DNS profiles and routes remain explicit and are applied as one checked transaction.'));

		s = m.section(form.NamedSection, 'main', 'steer', _('Traffic steering'));
		s.tab('general', _('General'));
		s.tab('advanced', _('Advanced runtime'));

		o = s.taboption('general', form.Flag, 'enabled', _('Enable Steer'));
		o.rmempty = false;

		o = s.taboption('general', form.Flag, 'router_proxy', _('Proxy router traffic'));
		o.default = '1';
		o.rmempty = false;
		o.description = _('When enabled, the router’s own public TCP/UDP traffic and traditional DNS use the same ordered rules as managed clients.');

		o = s.taboption('general', form.DynamicList, 'managed_zone', _('Managed firewall zones'));
		o.rmempty = false;
		o.description = _('Only traffic entering these zones is managed. Other zones, including WAN unless explicitly selected, are untouched.');
		uci.sections('firewall', 'zone').forEach((zone) => {
			if (zone.name)
				o.value(zone.name, zone.name);
		});

		o = s.taboption('general', form.ListValue, 'log_level', _('Log level'));
		o.value('error', _('Error'));
		o.value('warn', _('Warning'));
		o.value('info', _('Info'));
		o.default = 'warn';

		o = s.taboption('general', form.Button, '_geodata_update', _('GeoSite / GeoIP data'));
		o.inputtitle = _('Check now');
		o.inputstyle = 'apply';
		o.description = _('Downloads use the router traffic setting and ordinary Rules. Each source file keeps its own last-known-good version.');
		o.onclick = function() {
			return steer.updateGeodata();
		};

		o = s.taboption('advanced', form.Value, 'tproxy_port', _('TPROXY listen port'));
		o.datatype = 'port';
		o.rmempty = false;

		o = s.taboption('advanced', form.Value, 'dns_port', _('DNS interception port'));
		o.datatype = 'port';
		o.rmempty = false;

		o = s.taboption('advanced', form.Value, 'dns_upstream_mark', _('SmartDNS upstream loop-guard mark'));
		o.datatype = 'uinteger';
		o.rmempty = false;
		o.description = _('This mark only prevents SmartDNS UDP/TCP 53 upstream packets from being captured as new local DNS queries. The packets still enter TPROXY and use ordinary Rules. It must not overlap the TPROXY mark mask.');

		o = s.taboption('advanced', form.Value, 'routing_mark', _('Core bypass mark'));
		o.datatype = 'uinteger';
		o.rmempty = false;

		o = s.taboption('advanced', form.Value, 'tproxy_mark', _('TPROXY mark'));
		o.datatype = 'uinteger';
		o.rmempty = false;

		o = s.taboption('advanced', form.Value, 'mark_mask', _('Mark mask'));
		o.datatype = 'uinteger';
		o.rmempty = false;

		o = s.taboption('advanced', form.Value, 'route_table', _('Policy route table'));
		o.datatype = 'range(1,252)';
		o.rmempty = false;

		o = s.taboption('advanced', form.Value, 'rule_priority', _('Policy rule priority'));
		o.datatype = 'range(1,32765)';
		o.rmempty = false;

		s = m.section(form.NamedSection, 'bootstrap', 'bootstrap', _('Bootstrap DNS'));
		s.description = _('The startup root for proxy-node and encrypted-DNS hostnames. It is always contacted directly by IP with the core bypass mark, so startup never depends on SmartDNS or a proxy node.');

		o = s.option(form.ListValue, 'protocol', _('Protocol'));
		o.value('udp', 'UDP');
		o.value('tcp', 'TCP');
		o.rmempty = false;

		o = s.option(form.Value, 'server', _('Server IP'));
		o.datatype = 'ipaddr';
		o.rmempty = false;

		o = s.option(form.Value, 'server_port', _('Port'));
		o.datatype = 'port';
		o.rmempty = false;

		o = s.option(form.ListValue, 'strategy', _('Address strategy'));
		o.value('prefer_ipv4', _('Prefer IPv4'));
		o.value('prefer_ipv6', _('Prefer IPv6'));
		o.value('ipv4_only', _('IPv4 only'));
		o.value('ipv6_only', _('IPv6 only'));
		o.rmempty = false;

		return m.render().then((formNode) => E([], [ steer.renderStatus(status), formNode ]));
	},

	handleSaveApply: function(ev, mode) {
		return steer.apply(this, ev, mode);
	}
});
