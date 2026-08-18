include $(TOPDIR)/rules.mk

PKG_NAME:=steer-geodata
PKG_VERSION:=202608162214
PKG_RELEASE:=1

PKG_LICENSE:=GPL-3.0-or-later
PKG_MAINTAINER:=Steer contributors

include $(INCLUDE_DIR)/package.mk

GEOSITE_FILE:=steer-geosite-$(PKG_VERSION).dat
GEOIP_FILE:=steer-geoip-$(PKG_VERSION).dat
RELEASE_URL:=https://github.com/Loyalsoldier/v2ray-rules-dat/releases/download/$(PKG_VERSION)

define Download/geosite
  URL:=$(RELEASE_URL)
  FILE:=$(GEOSITE_FILE)
  URL_FILE:=geosite.dat
  HASH:=e0f0f3f07dee391d757174e3645f3284facfe57a1f9932388252e6b4f7a67dab
endef

define Download/geoip
  URL:=$(RELEASE_URL)
  FILE:=$(GEOIP_FILE)
  URL_FILE:=geoip.dat
  HASH:=b406dd3759037188b0674b110dcaf33664a699c0518152d0ca0d9023fc774c6b
endef

$(eval $(call Download,geosite))
$(eval $(call Download,geoip))

define Package/steer-geodata
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Versioned GeoSite and GeoIP seed data for Steer
  PKGARCH:=all
endef

define Package/steer-geodata/description
 Complete, version-locked Loyalsoldier GeoSite and GeoIP seed files used to
 initialize Steer before its proxy core is available.
endef

define Build/Compile
endef

define Package/steer-geodata/install
	$(INSTALL_DIR) $(1)/usr/share/steer/geodata-seed
	$(INSTALL_DATA) $(DL_DIR)/$(GEOSITE_FILE) $(1)/usr/share/steer/geodata-seed/geosite.dat
	$(INSTALL_DATA) $(DL_DIR)/$(GEOIP_FILE) $(1)/usr/share/steer/geodata-seed/geoip.dat
	$(INSTALL_DATA) ./files/release $(1)/usr/share/steer/geodata-seed/release
endef

$(eval $(call BuildPackage,steer-geodata))
