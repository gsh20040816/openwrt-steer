include $(TOPDIR)/rules.mk

PKG_NAME:=steer-openwrt
PKG_VERSION:=0.3.0
PKG_RELEASE:=2

PKG_LICENSE:=GPL-3.0-or-later
PKG_LICENSE_FILES:=LICENSE
PKG_MAINTAINER:=Steer contributors

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=github.com/gsh20040816/openwrt-steer/steer-openwrt
GO_PKG_BUILD_PKG:=$(GO_PKG)/cmd/steer-openwrt
GO_PKG_LDFLAGS_X:=main.version=$(PKG_VERSION)-r$(PKG_RELEASE)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/steer-openwrt
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Intent-driven transparent proxy controller for OpenWrt
  URL:=https://github.com/gsh20040816/openwrt-steer
	DEPENDS:=$(GO_ARCH_DEPENDS) +ca-bundle +firewall4 +geoview +ip \
	+kmod-nft-queue +kmod-nft-tproxy +kmod-tun +steer-geodata
  EXTRA_DEPENDS:=sing-box (>=1.13.18)
  PROVIDES:=steer
  CONFLICTS:=steer
  REPLACES:=steer
endef

define Package/steer-openwrt/description
 Steer compiles strict UCI routing and DNS intent into a verified sing-box
 execution plan and OpenWrt platform configuration.
endef

define Package/steer-openwrt/conffiles
/etc/config/steer
endef

define Package/steer-openwrt/postinst
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] && exit 0

config=/etc/config/steer
[ -f "$$config" ] || exit 0
schema="$$(uci -q get steer.main.schema_version 2>/dev/null)"
config_changed=0
case "$$schema" in
	6)
		for option in probe_direct probe_proxy speedtest_proxy; do
			first="$$(uci -q get steer.main.$$option 2>/dev/null | sed 's/ .*//')"
			uci -q delete steer.main.$$option
			[ -z "$$first" ] || uci set steer.main.$$option="$$first"
		done
		uci set steer.main.schema_version='7'
		config_changed=1
		;;
	7) ;;
	*)
		echo "steer: schema 6 or 7 is required, found '$$schema'" >&2
		exit 1
		;;
esac

# One release transition only: remove LuCI speed-test button values written by
# 0.2.0-r7 and older after all supported installations have upgraded.
for section in $$(uci -q show steer | sed -n "s/^steer\.\([a-z][a-z0-9_]*\)=node/\1/p"); do
	for option in _connect_speedtest _download_speedtest; do
		uci -q get steer.$$section.$$option >/dev/null || continue
		uci -q delete steer.$$section.$$option
		config_changed=1
	done
done
[ "$$config_changed" = 0 ] || uci commit steer

old_test_logs=/var/lib/steer/logs/speedtests
new_test_logs=/var/lib/steer/logs/tests/nodes
if [ -d "$$old_test_logs" ]; then
	mkdir -p "$$new_test_logs"
	chmod 0700 "$$new_test_logs"
	for old_node_directory in "$$old_test_logs"/*; do
		[ -d "$$old_node_directory" ] || continue
		node_id="$${old_node_directory##*/}"
		mkdir -p "$$new_test_logs/$$node_id"
		chmod 0700 "$$new_test_logs/$$node_id"
		for report in connect.json download.json; do
			[ -f "$$old_node_directory/$$report" ] || continue
			mv -f "$$old_node_directory/$$report" "$$new_test_logs/$$node_id/$$report"
		done
		rmdir "$$old_node_directory" 2>/dev/null || true
	done
	rmdir "$$old_test_logs" 2>/dev/null || true
fi

cron_file=/etc/crontabs/root
cron_line='*/15 * * * * /usr/sbin/steer subscription update 2>&1 | logger -t steer-subscription'
[ -x /etc/init.d/cron ] || { echo 'steer: BusyBox crond is required' >&2; exit 1; }
mkdir -p /etc/crontabs
grep -Fqx "$$cron_line" "$$cron_file" 2>/dev/null || echo "$$cron_line" >> "$$cron_file"
/etc/init.d/cron restart

# Switch to the migrated intent through Steer's verified generation
# transaction. PKG_UPGRADE must not leak into the init calls made by Apply.
apply_output="$$(PKG_UPGRADE=0 /usr/sbin/steer apply 2>&1)" || {
	echo "$$apply_output" >&2
	exit 1
}
endef

define Package/steer-openwrt/prerm
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] && exit 0
cron_file=/etc/crontabs/root
cron_line='*/15 * * * * /usr/sbin/steer subscription update 2>&1 | logger -t steer-subscription'
if [ -f "$$cron_file" ]; then
	tmp="$${cron_file}.steer.$$"
	grep -Fvx "$$cron_line" "$$cron_file" > "$$tmp" || true
	mv "$$tmp" "$$cron_file"
	/etc/init.d/cron restart
fi
endef

define Build/Prepare
	$(call Build/Prepare/Default)
	$(CP) ./src/. $(PKG_BUILD_DIR)/
	$(CP) $(CURDIR)/../LICENSE $(PKG_BUILD_DIR)/
endef

define Package/steer-openwrt/install
	$(call GoPackage/Package/Install/Bin,$(PKG_INSTALL_DIR))
	$(INSTALL_DIR) $(1)/usr/sbin
	$(INSTALL_BIN) $(PKG_INSTALL_DIR)/usr/bin/steer-openwrt $(1)/usr/sbin/steer

	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/etc/config/steer $(1)/etc/config/steer

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/etc/init.d/steer $(1)/etc/init.d/steer

	$(INSTALL_DIR) $(1)/lib/upgrade/keep.d
	$(INSTALL_DATA) ./files/lib/upgrade/keep.d/steer $(1)/lib/upgrade/keep.d/steer
endef

$(eval $(call GoBinPackage,steer-openwrt))
$(eval $(call BuildPackage,steer-openwrt))
