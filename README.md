# vessel

**Ship a podman stack to a machine with no network, as one signed file.**

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Moq77111113/vessel)](go.mod)

vessel reads the [quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html) units you already have, pins every image to a digest, and writes one executable that carries the images, the units and the files. On the target machine, that executable installs, upgrades, repairs and removes the stack.

```sh
# CI
vessel build ./acme --key vessel.key -o myapp   # writes myapp and its signature, myapp.sig

# air-gapped machine
./myapp install                       # returns once every service is up
```

## Install

```sh
go install github.com/Moq77111113/vessel/cmd/vessel@latest
```

Go 1.26+. Only the machine that builds needs vessel. The target needs podman 5.0+, systemd and root (Debian 13+, RHEL 9+, Rocky 9+).

## Describe a delivery

`vessel.yaml` sits beside the units:

```yaml
name: acme
version: 1.4.0
files:
  - source: realm.json
    target: /etc/acme/realm.json      # ###PUBLIC_HOST### inside is replaced at install
variables:
  - name: PUBLIC_HOST
    description: the public address of this machine
  - name: DB_PASSWORD
    secret: true                      # becomes a podman secret
    from: openssl rand -hex 32
actions:
  - mkdir -p /etc/acme/certs          # runs on every install: keep it repeatable
```

A value comes from `--set NAME=value`, from the previous install, or from `from:`. A secret never goes on a command line: `--set-file NAME=path` reads it from a file only its owner can read (`chmod 600`). vessel never prompts: a missing value, or a plain value holding a control character, stops the install before anything is written.

Image tags are rewritten to digests in place. Comments and ordering survive:

```diff
-Image=registry.example.com/acme/web:1.0
+Image=registry.example.com/acme/web@sha256:dcc94eaf6f86942cb6c1b93566b705177d1fd9a4c729e85de5dc430ef17aeb78
```

## Commands

On the build machine:

| command | does |
|---|---|
| `vessel build <dir> -o myapp` | pins, packs and signs; `--layout DIR` also keeps the OCI layout |
| `vessel build <layout> --evidence sbom.json -o myapp` | packs a kept layout with SBOMs and scan reports |

On the target, through the executable:

| command | does |
|---|---|
| `./myapp inspect` | prints what it carries; `--evidence DIR` writes out its SBOMs and reports |
| `./myapp install` | writes the files, loads the images, starts the services |
| `./myapp upgrade` | installs a newer version and removes what it no longer carries |
| `./myapp resume` | finishes an install that was cut |
| `./myapp status` | shows what is installed, and any file edited since |
| `./myapp uninstall` | removes exactly what the install put down |

`install`, `upgrade` and `resume` take `--dry-run`: same checks, then the plan, and nothing changes.

Exit codes: `0` done, `3` machine not ready, `4` refused and nothing touched, `5` failed partway.

## When an install is cut

vessel records every step as it goes. After a power cut or a failed service, `status` says where it stopped, and `install` refuses to run over it:

```sh
$ ./myapp status
acme 1.5.0, installed 2026-09-12T16:53:46Z
this install never finished, it stopped after files: run resume, or install with the 1.4.0 installer
```

- `./myapp resume` finishes it, skipping what is done.
- `./myapp-1.4.0 install` rolls back to the previous version.
- `./myapp uninstall` takes it all off.

An action cut or killed while it ran blocks `resume`: check it by hand, then `resume --skip-action`. Once the values are stored, `resume` refuses `--set`: it finishes with the values the install started with. A record left by an older vessel cannot be resumed: `uninstall`, then install again.

## Signing

`vessel build --key vessel.key` signs the executable, and the kept layout's `index.json`, with an ECDSA P-256 key you provide: a CI file variable, Vault or a KMS export, vessel does not care. Without a key the build stops before it pulls anything. `--insecure-unsigned` builds for development, and every command then shows the delivery as insecure.

```sh
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out vessel.key
openssl pkey -in vessel.key -pubout -out vessel.pub   # put this one on every site, once
```

Check a release on site before running it, with no network:

```sh
base64 -d myapp.sig > myapp.der
openssl dgst -sha256 -verify vessel.pub -signature myapp.der myapp
```

`cosign verify-blob myapp --key vessel.pub --signature myapp.sig --insecure-ignore-tlog` checks the same signature.

## SBOM and scan reports

Scan the exact layout you ship, then pack the reports inside the release:

```sh
vessel build ./acme --key vessel.key --layout ./bundle -o scratch
syft ./bundle -o spdx-json > sbom.spdx.json
vessel build ./bundle --key vessel.key --evidence sbom.spdx.json -o myapp
```

The second build signs the layout as it finds it: check `bundle/index.json.sig` against `vessel.pub` between the two.

vessel carries the reports and never reads them. On site, `./myapp inspect --evidence ./audit` writes them out.

## Status

v0: no tagged release, no production site yet. The command line and the bundle format may still change. Run it on a lab machine and [open an issue](https://github.com/Moq77111113/vessel/issues) where it breaks.

## License

MIT. See [LICENSE](LICENSE).
