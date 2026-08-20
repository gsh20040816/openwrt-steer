ARG TOOLCHAIN_IMAGE
FROM ${TOOLCHAIN_IMAGE}

ARG OPENWRT_REF

LABEL org.opencontainers.image.source="https://github.com/gsh20040816/openwrt-steer"
LABEL org.opencontainers.image.description="Pinned OpenWrt source and official external toolchain for Steer releases"

USER root
RUN git init /work/openwrt \
	&& git -C /work/openwrt remote add origin https://github.com/openwrt/openwrt.git \
	&& git -C /work/openwrt fetch --depth=1 origin "${OPENWRT_REF}" \
	&& git -C /work/openwrt checkout --detach FETCH_HEAD \
	&& chown -R buildbot:buildbot /work/openwrt

USER buildbot
WORKDIR /work/openwrt
RUN mkdir -p staging_dir build_dir \
	&& ln -s /prebuilt_tools/staging_dir/host staging_dir/host \
	&& ln -s /prebuilt_tools/build_dir/host build_dir/host \
	&& ./scripts/ext-tools.sh --refresh

COPY --chown=buildbot:buildbot entrypoint.sh /entrypoint.sh

ENTRYPOINT ["bash", "/entrypoint.sh"]
