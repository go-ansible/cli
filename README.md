# cli

CLI binaries: ansible-playbook, ansible, ansible-vault, ansible-galaxy,
ansible-pull, ansible-doc, ansible-config, ansible-console.

Part of [go-ansible](https://github.com/go-ansible) — a pure-Go (CGO=0),
functional-parity port of [Ansible](https://www.ansible.com/).

[![CI](https://github.com/go-ansible/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/go-ansible/cli/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-ansible/cli.svg)](https://pkg.go.dev/github.com/go-ansible/cli)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)

## Docker / OCI image

Every tagged release publishes a multi-arch image bundling all eight
binaries, `FROM scratch` — no libc, no interpreter, nothing but the
static binaries themselves (real Ansible cannot do this: it needs
Python plus several pip-installed packages just to be the controller;
see [BENCHMARKS.md](https://github.com/go-ansible/.github/blob/main/BENCHMARKS.md)
for the measured comparison).

```sh
docker run --rm -v "$PWD:/pb" ghcr.io/go-ansible/cli \
  -i /pb/inventory.yml /pb/site.yml
```

The entrypoint is `ansible-playbook`; run any other binary with
`--entrypoint`:

```sh
docker run --rm --entrypoint ansible-doc ghcr.io/go-ansible/cli setup
```

Published for `linux/amd64`, `linux/arm64`, `linux/riscv64`,
`linux/ppc64le`, and `linux/s390x`. `linux/loong64` is not included —
it isn't a platform `docker buildx` recognizes without additional
QEMU/binfmt setup this project's CI doesn't do, unlike the Go test
matrix elsewhere in this org, which does cover it directly via
`qemu-user-static`.

**Known limitation**: `command`/`shell` tasks against a `local`
connection need `/bin/sh`, which doesn't exist in a scratch image —
`debug`/`copy`/`template`/`set_fact` and similar work fine. A playbook
targeting a real remote host over SSH is unaffected either way, since
the shell requirement is the *target's*, not this image's.
