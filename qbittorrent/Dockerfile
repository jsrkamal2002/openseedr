# Build qBittorrent-nox (headless) from the source repo.
# Multi-stage: build deps → compile → minimal runtime.

# ── Stage 1: build environment ────────────────────────────────────────────────
FROM alpine:3.20 AS builder

RUN apk add --no-cache \
    alpine-sdk \
    boost-dev \
    cmake \
    libtorrent-rasterbar-dev \
    openssl-dev \
    qt6-qtbase-dev \
    qt6-qttools-dev \
    zlib-dev \
    ninja

WORKDIR /src
COPY src/ .

RUN cmake -B build -G Ninja \
      -DCMAKE_BUILD_TYPE=Release \
      -DGUI=OFF \
      -DWEBUI=ON \
      -DTESTING=OFF \
      -DSTACKTRACE=OFF \
      -DCMAKE_INSTALL_PREFIX=/usr/local && \
    cmake --build build --parallel $(nproc) && \
    cmake --install build

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache \
    boost-filesystem \
    boost-program_options \
    boost-system \
    libtorrent-rasterbar \
    qt6-qtbase \
    openssl \
    ca-certificates \
    python3 \
    tini

# Non-root user
RUN addgroup -S qbt && adduser -S -G qbt qbt

COPY --from=builder /usr/local/bin/qbittorrent-nox /usr/local/bin/qbittorrent-nox
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

# qBittorrent config and download directories.
# /data is world-writable (sticky bit) so other containers (e.g. the api,
# running as a different UID) can create per-user sub-directories inside it.
RUN mkdir -p /config /data && chown -R qbt:qbt /config /data && chmod 1777 /data

USER qbt

VOLUME ["/config", "/data"]
EXPOSE 8080 6881 6881/udp

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["qbittorrent-nox", \
     "--profile=/config", \
     "--webui-port=8080"]
