include $(TOPDIR)/rules.mk

PKG_NAME:=steer
PKG_VERSION:=0.1.0
PKG_RELEASE:=10

PKG_LICENSE:=GPL-3.0-or-later
PKG_MAINTAINER:=Steer contributors

include $(INCLUDE_DIR)/package.mk

define Package/steer
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Intent-driven transparent proxy for OpenWrt
  DEPENDS:=+steer-geodata
  EXTRA_DEPENDS:=ca-bundle (>=0), firewall4 (>=0), geoview (>=0.2.6-r2), ip-full (>=0), \
	kmod-nft-socket (>=0), kmod-nft-tproxy (>=0), sing-box (>=0), \
	smartdns (>=0), ucode (>=0), ucode-mod-fs (>=0), ucode-mod-uci (>=0)
endef

define Package/steer/description
 Steer compiles explicit UCI routing and DNS intent into a controlled
 sing-box transparent proxy configuration.
endef

define Package/steer/conffiles
/etc/config/steer
endef

define Package/steer/install
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/etc/config/steer $(1)/etc/config/steer

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/etc/init.d/steer $(1)/etc/init.d/steer
	$(INSTALL_BIN) ./files/etc/init.d/steer-geodata $(1)/etc/init.d/steer-geodata

	$(INSTALL_DIR) $(1)/etc/uci-defaults
	$(INSTALL_BIN) ./files/etc/uci-defaults/99-steer-firewall $(1)/etc/uci-defaults/99-steer-firewall

	$(INSTALL_DIR) $(1)/lib/upgrade/keep.d
	$(INSTALL_DATA) ./files/lib/upgrade/keep.d/steer $(1)/lib/upgrade/keep.d/steer

	$(INSTALL_DIR) $(1)/usr/share/steer
	$(INSTALL_DATA) ./files/usr/share/steer/model.uc $(1)/usr/share/steer/model.uc
	$(INSTALL_DATA) ./files/usr/share/steer/firewall.uc $(1)/usr/share/steer/firewall.uc
	$(INSTALL_BIN) ./files/usr/share/steer/firewall.include $(1)/usr/share/steer/firewall.include

	$(INSTALL_DIR) $(1)/usr/libexec/steer
	$(INSTALL_BIN) ./files/usr/libexec/steer/runtime $(1)/usr/libexec/steer/runtime
	$(INSTALL_BIN) ./files/usr/libexec/steer/geodata $(1)/usr/libexec/steer/geodata

	$(INSTALL_DIR) $(1)/usr/sbin
	$(INSTALL_BIN) ./files/usr/sbin/steerctl $(1)/usr/sbin/steerctl
endef

define Package/steer/postrm
#!/bin/sh
if [ -z "$${IPKG_INSTROOT}" ]; then
	uci -q delete firewall.steer
	uci -q commit firewall
fi
endef

define Build/Compile
endef

$(eval $(call BuildPackage,steer))
