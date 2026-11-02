# cli

CLI binaries: ansible-playbook, ansible, ansible-inventory, ansible-vault,
ansible-galaxy, ansible-pull, ansible-doc, ansible-config, ansible-console.

Nine of the ten console scripts real `ansible-core` 2.21.4 declares in its own
`entry_points.txt`. The tenth is `ansible-test`, which drives collection test
suites against collections this port does not host.

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

## `ansible-playbook --compat-report`

This port's own flag, not one real has. It answers the question that makes
people keep old Python virtualenvs around — not "does this playbook run"
but "what in it stopped running, and what replaces it":

```sh
$ ansible-playbook --compat-report legacy.yml
playbook: legacy.yml
  2:3: play keyword "sudo": use `become: true`
  3:3: play keyword "sudo_user": use `become_user:`
  17:7: task keyword "always_run": use `check_mode: false` — it is the same
        meaning with the polarity flipped
  19:7: task keyword "include": use `import_tasks:` for a file to pull in at
        parse time, or `include_tasks:` for one chosen while the play runs

4 item(s) will not run as written.
$ echo $?
4
```

It exits 4 when it finds anything — the same status both implementations
give a playbook that will not run — so a pipeline can gate on it, 0 when
there is nothing, and 1 when the playbook could not be read at all,
because "I could not look" must not read as "there is nothing wrong".

It runs before the parse, which is the point: a playbook carrying a
retired keyword does not parse, here or in real, so a report that needed
a parsed playbook would be silent in the one case it is for. It reaches
no host, loads no inventory, and decrypts a vaulted playbook first.

The replacement advice comes from `playbook.CompatScan`, whose table was
measured against ansible-core 2.21.4 rather than taken from a changelog.
What it deliberately does not say is the version each keyword went away
in: the reference does not ship that history.

## `ansible-doc` names the module's own collection

A page header and a `-l` line carry the collection the module really comes
from, as real prints it:

```sh
$ ansible-doc rocketchat
> MODULE community.general.rocketchat
$ ansible-doc synchronize
> MODULE ansible.posix.synchronize
```

It printed `ansible.builtin.` for every module until v0.143.0, on a premise
that was true when written and wrong for 514 of the 576 registered modules.
Worth stating because it was not cosmetic: `modules.NormalizeName` strips any
known collection prefix, so `ansible.builtin.rocketchat` *ran here* and real
Ansible rejects it — this port's own documentation was handing out a name that
works nowhere else, and nothing local could catch that.

**Known limitation**: `command`/`shell` tasks against a `local`
connection need `/bin/sh`, which doesn't exist in a scratch image —
`debug`/`copy`/`template`/`set_fact` and similar work fine. A playbook
targeting a real remote host over SSH is unaffected either way, since
the shell requirement is the *target's*, not this image's.
