# SPDX-License-Identifier: Apache-2.0

include $(TOPDIR)/rules.mk

PKG_NAME:=geoview
PKG_VERSION:=0.2.6
PKG_RELEASE:=2

PKG_SOURCE_PROTO:=git
PKG_SOURCE_URL:=https://github.com/snowie2000/geoview.git
PKG_SOURCE_VERSION:=3c91926d360b8f49d47520639e574608318baf12
PKG_MIRROR_HASH:=6a5325e3390e9a9061abedc9bddd196c9182b1827b817471bf8d36d0f9715e45

PKG_LICENSE:=Apache-2.0
PKG_MAINTAINER:=snowie2000

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_USE_MIPS16:=0
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=github.com/snowie2000/geoview
GO_PKG_BUILD_PKG:=$(GO_PKG)
GO_PKG_LDFLAGS:=-s -w

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/geoview
  TITLE:=A geofile toolkit
  URL:=https://github.com/snowie2000/geoview
  SECTION:=net
  CATEGORY:=Network
  SUBMENU:=IP Addresses and Names
  DEPENDS+= $(GO_ARCH_DEPENDS)
endef

define Package/geoview/description
  geoview is a handy tool to extract useful information from geo* files.
endef

define Package/geoview/install
	$(call GoPackage/Package/Install/Bin,$(PKG_INSTALL_DIR))
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_INSTALL_DIR)/usr/bin/geoview $(1)/usr/bin/
endef

$(eval $(call GoBinPackage,geoview))
$(eval $(call BuildPackage,geoview))
