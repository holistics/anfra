# syntax=docker/dockerfile:1
#
# anfra serve, for a repo mounted (or copied) at /repo:
#
#   docker run -p 7878:7878 -v "$PWD:/repo" ghcr.io/holistics/anfra
#
# Then http://localhost:7878/ lists the repo's Data Apps; the core API is at /api, MCP at
# /mcp. Any other command runs the same way, in the mounted repo:
#
#   docker run --rm -v "$PWD:/repo" ghcr.io/holistics/anfra query --dataset sales '...'
#
# The repo's .anfra/data_sources.yml, credentials included, comes with the mount. anfra runs
# as uid 1000: it reads the repo, and writes only its own state, under /home/anfra.
#
# One image name for every platform. Each tag (0.4.0, 0.4, latest) is a multi-platform
# index of linux/amd64 and linux/arm64, from which Docker pulls the one matching the machine,
# Apple silicon included; Docker on macOS and Windows runs these Linux images in its VM.
# --platform overrides the choice, at the cost of emulation.
#
# The image packages the release's own Linux binaries, sidecars embedded, which
# build_release.yml lays out in the build context as <os>/<arch>/anfra. So it is
# the same anfra as the release's download, for amd64 and arm64 alike, and the
# final stage runs nothing: building it for another architecture needs no
# emulation. To build it locally, put a Linux binary built with -tags embed_sidecar
# at <dir>/linux/amd64/anfra, then:
#
#   docker buildx build --platform linux/amd64 -f Dockerfile -t anfra <dir>
#
# Debian trixie: the sidecars anfra unpacks at startup link glibc 2.38 or newer.

# What the final stage needs from a shell, prepared on the builder's own platform. tini is
# the target's: Debian's static build, downloaded for TARGETARCH and unpacked, not installed.
FROM --platform=$BUILDPLATFORM debian:trixie-slim AS prepare
ARG TARGETARCH
RUN dpkg --add-architecture "$TARGETARCH" \
 && apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && cd /tmp && apt-get download "tini:$TARGETARCH" && dpkg-deb -x tini_*.deb /tmp/tini \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 1000 --user-group --home-dir /home/anfra --no-create-home anfra \
 && mkdir -p /out/home/anfra /out/repo

FROM debian:trixie-slim
ARG TARGETPLATFORM
# Warehouses are reached over TLS: the system's roots.
COPY --from=prepare /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# A named user, not a bare uid: the sidecars look their user up, and fail without one. The
# account files are text, the same on every architecture.
COPY --from=prepare /etc/passwd /etc/group /etc/
# anfra's state (~/.anfra: logs, caches, the unpacked sidecars) and the repo, the user's own.
COPY --from=prepare --chown=1000:1000 /out/ /
COPY --chmod=755 ${TARGETPLATFORM}/anfra /usr/local/bin/anfra
COPY --from=prepare /tmp/tini/usr/bin/tini-static /usr/bin/tini

USER anfra
ENV HOME=/home/anfra \
    ANFRA_NO_UPDATE_NOTIFIER=1
WORKDIR /repo
EXPOSE 7878

# tini is PID 1, as `docker run --init` would make it: it reaps orphans and forwards signals,
# and anfra is not PID 1, which a process it spawns could read as its host having died.
#
# Every interface, so a published port reaches it. anfra serve has no
# authentication: publish it only where its users may query the repo's data.
ENTRYPOINT ["/usr/bin/tini", "--", "anfra"]
CMD ["serve", "--addr", "0.0.0.0:7878"]

LABEL org.opencontainers.image.source=https://github.com/holistics/anfra \
      org.opencontainers.image.description="Anfra: anfra serve for a repo mounted at /repo" \
      org.opencontainers.image.licenses=Apache-2.0
