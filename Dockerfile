# Multi-stage, multi-arch build for the five go-ansible CLI binaries.
# The final stage is FROM scratch: a CGO_ENABLED=0 Go binary needs no
# libc, no interpreter, nothing else — real Ansible fundamentally
# cannot reach this (it needs Python plus several pip-installed
# packages even just to be the controller). See BENCHMARKS.md in
# go-ansible/.github for the measured comparison.
#
# Known limitation, not hidden: command/shell (and any module that
# execs a shell on a "local" connection) need /bin/sh, which does not
# exist in a scratch image. Tasks using debug/copy/template/set_fact
# and friends work; a playbook that also needs a real target host over
# SSH is unaffected either way, since the shell requirement is the
# TARGET's, not this image's.
#
# Build with docker buildx for multi-arch:
#   docker buildx build --platform linux/amd64,linux/arm64,linux/riscv64,linux/loong64,linux/ppc64le,linux/s390x -t IMAGE .
# TARGETOS/TARGETARCH are supplied automatically by buildx per platform.
#
# The build stage is pinned to --platform=$BUILDPLATFORM (the runner's
# own native architecture, not the target one) deliberately: it cross-
# compiles via GOOS/GOARCH instead of running the Go toolchain itself
# under QEMU emulation for every target. This isn't just a speed
# optimization — it's required for linux/loong64, since the official
# golang image publishes no loong64 manifest at all (confirmed via
# `docker manifest inspect golang:1.26`), so buildx could never pull a
# native-loong64 build stage even with QEMU installed. Cross-compiling
# from the host's own architecture sidesteps that entirely: only the
# empty `scratch` final stage is tagged as the target platform, which
# has no architecture-specific content to need a manifest for.
FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir /out && \
    for bin in ansible ansible-playbook ansible-vault ansible-galaxy ansible-pull ansible-doc ansible-config ansible-console; do \
      CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
        go build -trimpath -ldflags="-s -w" -o /out/$bin ./cmd/$bin; \
    done

FROM scratch
COPY --from=build /out/ /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/ansible-playbook"]
