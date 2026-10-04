# cli

CLI binaries: ansible-playbook, ansible, ansible-inventory, ansible-vault,
ansible-galaxy, ansible-pull, ansible-doc, ansible-config, ansible-console.

Part of [go-ansible](https://github.com/go-ansible) — a pure-Go (CGO=0),
functional-parity port of [Ansible](https://www.ansible.com/).

All nine build for `windows/amd64` and `windows/arm64` as well as the six
64-bit Unix targets, so the control node can be Windows — which real
ansible-core does not support at all. Whether a *target* can be Windows is
a separate question: `ansible_connection: winrm` reaches one, but the
`ansible.windows` module family is not ported, so what runs over it today
is whatever needs no POSIX shell.

[![CI](https://github.com/go-ansible/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/go-ansible/cli/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-ansible/cli.svg)](https://pkg.go.dev/github.com/go-ansible/cli)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)

## Docker / OCI image

Every tagged release publishes a multi-arch image bundling all nine
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

Published for all six 64-bit Linux targets — `linux/amd64`,
`linux/arm64`, `linux/riscv64`, `linux/loong64`, `linux/ppc64le` and
`linux/s390x`. This paragraph used to say loong64 was excluded for want
of QEMU/binfmt setup; it is in the workflow's platform list and in the
published manifest:

```sh
$ docker manifest inspect ghcr.io/go-ansible/cli | grep architecture
amd64  arm64  loong64  ppc64le  riscv64  s390x
```

**Known limitation**: `command`/`shell` tasks against a `local`
connection need `/bin/sh`, which doesn't exist in a scratch image —
`debug`/`copy`/`template`/`set_fact` and similar work fine. A playbook
targeting a real remote host over SSH is unaffected either way, since
the shell requirement is the *target's*, not this image's.
