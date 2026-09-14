# vessel

**Your site has no route to a registry. Your deploy tool assumes it does.**

An air-gapped machine cannot pull an image, so the whole stack has to arrive as files. By hand that
means a `podman save` per image, a tarball, a copy of the units, and the site values on a sheet of
paper. `vessel` reads the descriptor you already have, pins every image to a digest, and writes one
executable that installs itself. Its exit code is the playbook's contract: `0` running, `3` the
machine is not ready, `4` refused, `5` failed partway.

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Moq77111113/vessel)](go.mod)

## Quick start

```sh
vessel build ./acme -o myapp   # pin every image to a digest, write one file
./myapp install                # on the other side; it is running when this returns
```

That is the whole delivery. `myapp` carries the images, the units, the files and a copy of vessel.

## Install

```sh
go install github.com/Moq77111113/vessel/cmd/vessel@latest
```

Go 1.26 or newer. Only the machine that builds needs vessel.

## Build

`./acme` holds a `vessel.yaml` and a [quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html)
directory, the systemd units that each run a container.

```sh
$ vessel build ./acme -o myapp
  Resolving registry.example.com/acme/web:1.0
    Pulling registry.example.com/acme/web@sha256:dcc94eaf6f8694…
   Finished myapp, 15 MB, run it on the target machine
```

Every `Image=` tag comes out a digest, patched in place. Comments and ordering survive:

```ini
[Container]
-Image=registry.example.com/acme/web:1.0
+Image=registry.example.com/acme/web@sha256:dcc94eaf6f86942cb6c1b93566b705177d1fd9a4c729e85de5dc430ef17aeb78
Network=app.network
```

`build` reads the credentials `podman login` already left.

`--layout ./bundle` also writes the OCI layout, for a CI job archiving deliveries on a registry.
`vessel install ./bundle` installs one on a machine that already carries vessel.

## vessel.yaml

```yaml
name: acme
version: 1.4.0
units: ./units
files:
  - source: realm.json
    target: /etc/acme/realm.json
variables:
  - name: PUBLIC_HOST
    description: the public address of this machine
  - name: DB_PASSWORD
    secret: true
    from: openssl rand -hex 32
actions:
  - mkdir -p /etc/acme/certs
```

| key | what it does |
|---|---|
| `name` | a plain lowercase identifier; the machine keeps this delivery under `/var/lib/vessel/<name>/` |
| `units` | the descriptor directory, relative to this file |
| `files` | plain files to carry; `source` relative to this file, `target` absolute on the machine |
| `variables` | values the machine supplies; `###NAME###` in any carried file is replaced |
| `actions` | commands run before anything is written, for their effect only; they run on every install |

A value comes from `--set`, from a previous install, or from `from:`. Missing all three stops the
install before the first byte:

```sh
$ ./myapp install
vessel: PUBLIC_HOST is not set: the public address of this machine
```


`install` runs `podman secret create` for a `secret: true` variable, and the unit reads it back
with `Secret=DB_PASSWORD,type=env,target=POSTGRES_PASSWORD`. `build` refuses a secret no unit reads
that way.

## On the machine

```sh
$ ./myapp inspect                  # reads and prints, writes nothing
acme 1.4.0, read by quadlet, resolved for linux/amd64
      Image registry.example.com/acme/web:1.0 sha256:dcc94eaf6f8694…
       File etc/containers/systemd/web.container 315 bytes

$ ./myapp install --set PUBLIC_HOST=203.0.113.10
   Finished acme 1.4.0 installed and running: 2 images, 4 of 4 files changed

$ ./myapp status
acme 1.4.0, installed 2026-09-12T16:53:46Z
previous 1.3.0, installed 2026-03-02T09:14:11Z
    Service web.service running
    Differs etc/acme/realm.json

$ ./myapp upgrade
   Removing etc/containers/systemd/cache.container
   Finished acme 1.5.0 installed and running: 2 images, 3 of 4 files changed

$ ./myapp uninstall
   Finished acme 1.5.0 removed: 4 files
```

| verb | what it does |
|---|---|
| `install` | writes the files, loads the images, starts the services, and returns once they are up |
| `status` | reads the record `install` wrote: `Differs` was edited since, `Absent` is gone, `Unreadable` could not be read. With no name, lists every delivery |
| `upgrade` | `install`, plus stopping and removing what the previous version put and this one does not carry |
| `uninstall` | removes exactly the files its record names, and names the secrets, images and site values it leaves |

vessel checks the machine first and names every problem at once:

```sh
$ ./myapp install
vessel: this machine is not ready:
  podman is too old for quadlet: found 4.3.1, want 5.0 or newer
  cannot write to /etc/containers/systemd
```

## Requirements

Debian 13+, RHEL 9+ or Rocky 9+, podman 5.0 or newer, systemd, root. The floor is podman 4.4, where
quadlet arrived, and Debian 12 ships 4.3.

## Signing

Every part of a bundle is pinned by sha256, and vessel refuses a file that does not match. Print
`sha256sum myapp` on the install sheet for the operator to compare. `vessel build --key vessel.key`
also writes `myapp.minisig`, for sites running a
[minisign](https://jedisct1.github.io/minisign/) step.

To have the binary check for itself: build vessel with your public key baked in
(`make build KEY=vessel.pub`), sign the layout with `vessel build --key --layout`, and ship the
layout.

## Maturity

v0: no tagged release, no CI, no production site. The command line, the bundle format and
`vessel.yaml` change without notice.

What exists is covered: `e2e/` drives the real command line, one test checks the digest a unit pins
against the manifest blob the bundle carries byte for byte, and one loads an image into a real
podman. Run it on a lab machine and tell me where it breaks.

MIT. See [LICENSE](LICENSE).
