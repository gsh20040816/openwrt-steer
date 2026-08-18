ARG SDK_IMAGE
FROM ${SDK_IMAGE}

LABEL org.opencontainers.image.source="https://github.com/gsh20040816/openwrt-steer"
LABEL org.opencontainers.image.description="Pinned OpenWrt SDK for Steer release builds"

# The pinned image already contains the extracted SDK under its /builder
# volume. The custom entrypoint deliberately does not invoke the leftover
# setup.sh downloader.

COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["bash", "/entrypoint.sh"]
