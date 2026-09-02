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
#   docker buildx build --platform linux/amd64,linux/arm64,linux/riscv64,linux/ppc64le,linux/s390x -t IMAGE .
# TARGETOS/TARGETARCH are supplied automatically by buildx per platform.

FROM golang:1.26 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir /out && \
    for bin in ansible ansible-playbook ansible-vault ansible-galaxy ansible-pull ansible-doc ansible-config; do \
      CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
        go build -trimpath -ldflags="-s -w" -o /out/$bin ./cmd/$bin; \
    done

FROM scratch
COPY --from=build /out/ /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/ansible-playbook"]
