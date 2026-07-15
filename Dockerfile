# syntax=docker/dockerfile:1
FROM gcr.io/distroless/static-debian13:nonroot
# TARGETPLATFORM is set automatically by buildx (e.g. linux/amd64). GoReleaser's
# dockers_v2 stages each binary under <os>/<arch>/, and `just oci` mirrors that
# layout, so the same COPY works for local and release builds.
ARG TARGETPLATFORM
WORKDIR /
COPY ${TARGETPLATFORM}/ys /ys
ENTRYPOINT [ "/ys" ]
