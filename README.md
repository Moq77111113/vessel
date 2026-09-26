# vessel

**Your site has no route to a registry. Your deploy tool assumes it does.**

An air-gapped machine cannot pull an image, so the whole stack has to arrive as files. By hand that
means a `podman save` per image, a tarball, a copy of the units, and the site values on a sheet of
paper. `vessel` reads the descriptor you already have, pins every image to a digest, and writes one
signed executable that installs itself. Its exit code is the playbook's contract: `0` running, `3`
the machine is not ready, `4` refused, `5` failed partway.

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Moq77111113/vessel)](go.mod)

## Quick start

```sh
vessel build ./acme -o myapp                     # in CI: pin every image, sign, write two files
cosign verify-blob myapp --bundle myapp.sigstore \
  --certificate-identity <ci-identity> --certificate-oidc-issuer <issuer>
./myapp install                                  # on the other side; running when this returns
```

`myapp` carries the images, the units, the files and a copy of vessel. `myapp.sigstore` proves
which CI built it.

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
myapp.sigstore, ship it alongside
```

Every `Image=` tag comes out a digest, patched in place. Comments and ordering survive:

```ini
[Container]
-Image=registry.example.com/acme/web:1.0
+Image=registry.example.com/acme/web@sha256:dcc94eaf6f86942cb6c1b93566b705177d1fd9a4c729e85de5dc430ef17aeb78
Network=app.network
```

`build` reads the credentials `podman login` already left. `--platform linux/arm64` resolves for
another platform; the default is `linux/amd64`.

`--layout ./bundle` also keeps the OCI layout, signed as `bundle/index.json.sigstore`, for a CI job
that scans it or archives it on a registry. `vessel install ./bundle` installs one on a machine that
already carries vessel; verify `bundle/index.json` with cosign first.

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
| `actions` | commands run before the files are written; each install runs them, so keep them repeatable |

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
   Evidence sbom.spdx.json 48213 bytes

$ ./myapp install --set PUBLIC_HOST=203.0.113.10
   Finished acme 1.4.0 installed and running: 2 images, 4 of 4 files changed

$ ./myapp status
acme 1.4.0, installed 2026-09-12T16:53:46Z
previous 1.3.0, installed 2026-03-02T09:14:11Z
    Service web.service running
    Service backup.timer running
    Differs etc/acme/realm.json

$ ./myapp upgrade
   Removing etc/containers/systemd/cache.container
   Finished acme 1.5.0 installed and running: 2 images, 3 of 4 files changed

$ ./myapp uninstall
   Finished acme 1.5.0 removed: 4 files
```

| verb | what it does |
|---|---|
| `inspect` | prints what the executable carries; `--evidence DIR` writes out its SBOM and reports |
| `install` | writes the files, loads the images, starts the services, and returns once they are up |
| `status` | reads the record `install` wrote: `Differs` was edited since, `Absent` is gone, `Unreadable` could not be read. With no name, lists every delivery |
| `upgrade` | `install`, plus stopping and removing what the previous version put and this one does not carry |
| `resume` | finishes an install this executable started and never finished |
| `uninstall` | removes exactly the files its record names, and names the secrets, images and site values it leaves |

vessel checks the machine first and names every problem at once:

```sh
$ ./myapp install
vessel: this machine is not ready:
  podman is too old for quadlet: found 4.3.1, want 5.0 or newer
  cannot write to /etc/containers/systemd
```

## Dry run

`install`, `upgrade` and `resume` take `--dry-run`. Same checks and refusals as the real run, then
the plan. Nothing on the machine changes, no `from:` command runs, no secret value is printed.

```sh
$ ./myapp upgrade --dry-run
acme 1.5.0, dry run: nothing on this machine changed
    Checked the machine is ready
    Checked the bundle matches its digests
    Checked every variable has a value
    Checked no install is left unfinished
        Run mkdir -p /etc/acme/certs
    Replace etc/containers/systemd/web.container
     Remove etc/containers/systemd/cache.container
       Stop cache.service
       Load registry.example.com/acme/web:1.0 sha256:dcc94eaf6f8694…
      Start web.service
 Unverified the result of every action
 Unverified service health, known once started
```

`Add`, `Replace`, `Keep` and `Remove` name each file. A file whose content waits on a `from:`
value reads `Depends etc/acme/realm.json on PUBLIC_HOST`.

## When an install is cut

The record opens before the first change and notes every step and every action as it finishes.
A power cut, a kill or a failed service leaves it open, and `install` and `upgrade` refuse to
hide it (exit `4`):

```sh
$ ./myapp status
acme 1.5.0, installed 2026-09-12T16:53:46Z
this install never finished, it stopped after files: run resume, or install with the 1.4.0 installer
```

- **Finish it:** `./myapp resume` skips every step and action already done. A failed action runs again.
- **Go back:** run `install` with the previous version's executable. It puts its files back,
  removes what the new version added, and runs its own actions again.
- **Take it off:** `./myapp uninstall` removes both versions' files.

An action cut while it ran has an unknown result, so `resume` refuses:

```sh
$ ./myapp resume
vessel: "mkdir -p /etc/acme/certs": an action may have stopped halfway, check it by hand, then resume --skip-action
```

Check what the command did, then `./myapp resume --skip-action` marks it finished without running it.
vessel never removes an image or a secret while it recovers.

## Requirements

Debian 13+, RHEL 9+ or Rocky 9+, podman 5.0 or newer, systemd, root. The floor is podman 4.4, where
quadlet arrived, and Debian 12 ships 4.3.

## Signing

`vessel build` signs with [Sigstore](https://www.sigstore.dev/), using the CI's own identity. There
is no key to generate or store. A build with no CI identity fails before it resolves a single image.

GitLab CI:

```yaml
build:
  id_tokens:
    VESSEL_SIGSTORE_ID_TOKEN:
      aud: sigstore
  script:
    - vessel build ./acme -o myapp
```

GitHub Actions: give the job `permissions: id-token: write`.

Verify before running, on a machine that already has [cosign](https://github.com/sigstore/cosign):

```sh
# GitLab
cosign verify-blob myapp --bundle myapp.sigstore \
  --certificate-identity-regexp '^https://gitlab.com/<group>/<project>/-/jobs/[0-9]+$' \
  --certificate-oidc-issuer https://gitlab.com

# GitHub
cosign verify-blob myapp --bundle myapp.sigstore \
  --certificate-identity 'https://github.com/<owner>/<repo>/.github/workflows/<file>@<ref>' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Each signature is logged in [Rekor](https://docs.sigstore.dev/logging/overview/), Sigstore's
public log: it holds the CI identity (the project URL) and the hash of `myapp`, never the file.

For a local build, `--insecure-unsigned` writes no signature. The bundle, the record, `inspect` and
`status` all say so: `insecure: built with --insecure-unsigned, for development use only`.

## SBOM and scan reports

vessel carries them and never reads them. CI scans the kept layout, then builds from it:

```sh
vessel build ./acme --layout ./bundle -o scratch
cosign verify-blob bundle/index.json --bundle bundle/index.json.sigstore ...
syft ./bundle -o spdx-json > sbom.spdx.json
trivy fs ./bundle --format json > trivy.json
vessel build ./bundle --evidence sbom.spdx.json --evidence trivy.json -o myapp
```

Built from a layout, `build` packs those exact bytes, so a report describes what ships. Each file
goes in as an OCI artifact bound to the bundle root, inside the signed executable. On site,
`./myapp inspect --evidence ./audit` writes them out.

## Maturity

v0: no tagged release, no production site. The command line, the bundle format and `vessel.yaml`
change without notice.

What exists is covered: `e2e/` drives the real command line, one test checks the digest a unit pins
against the manifest blob the bundle carries byte for byte, and one loads an image into a real
podman. CI checks a signed build with cosign on every push to `main`. Run it on a lab machine and
tell me where it breaks.

MIT. See [LICENSE](LICENSE).
