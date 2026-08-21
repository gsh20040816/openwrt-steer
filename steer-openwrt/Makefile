include $(TOPDIR)/rules.mk

PKG_NAME:=steer-openwrt
PKG_VERSION:=0.2.0
PKG_RELEASE:=3

PKG_LICENSE:=GPL-3.0-or-later
PKG_LICENSE_FILES:=LICENSE
PKG_MAINTAINER:=Steer contributors

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=github.com/gsh20040816/openwrt-steer/steer-openwrt
GO_PKG_BUILD_PKG:=$(GO_PKG)/cmd/steer-openwrt
GO_PKG_LDFLAGS_X:=$(GO_PKG)/cmd/steer-openwrt.version=$(PKG_VERSION)-r$(PKG_RELEASE)

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
# One release transition only: remove this block entirely when schema 6 ships.
[ -n "$${IPKG_INSTROOT}" ] && exit 0

config=/etc/config/steer
[ -f "$$config" ] || exit 0
schema="$$(uci -q get steer.main.schema_version 2>/dev/null)"
case "$$schema" in
	4)
		uci -q delete steer.main.managed_zone
		uci set steer.main.schema_version='5'
		uci commit steer
		;;
	5)
		;;
	*)
		echo "steer: unsupported existing schema '$$schema'; refusing automatic migration" >&2
		exit 1
		;;
esac
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
