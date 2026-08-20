include $(TOPDIR)/rules.mk

PKG_NAME:=steer
PKG_VERSION:=0.1.0
PKG_RELEASE:=18

PKG_LICENSE:=GPL-3.0-or-later
PKG_MAINTAINER:=Steer contributors

include $(INCLUDE_DIR)/package.mk

define Package/steer
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Intent-driven transparent proxy for OpenWrt
  DEPENDS:=+steer-geodata +bind-dig +ca-bundle +firewall4 +geoview +ip \
	+kmod-nft-socket +kmod-nft-tproxy +smartdns +ucode +ucode-mod-fs \
	+ucode-mod-uci
  # Selecting curl/libcurl and sing-box/sing-box-tiny in the 25.12 SDK creates
  # upstream Kconfig dependency cycles. Keep them as explicit APK dependencies
  # without turning their runtime variants into Steer's build-time options.
  EXTRA_DEPENDS:=curl (>=0), sing-box (>=0)
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
