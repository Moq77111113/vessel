# vessel

Ship a container stack to a machine with no network.

Vessel reads the descriptor you already have, pins every image to a digest, and writes one signed
OCI artifact. That artifact travels through your registry, or as a single executable on a USB
stick. No Kubernetes, no format to learn, nothing to install on the far machine.

```sh
vessel link ./units -o ./bundle          # pin every image to a digest
vessel pack ./bundle -o myapp            # one file to carry
./myapp install                          # on the other side
```

## Install

```sh
go install github.com/Moq77111113/vessel/cmd/vessel@latest
```

Go 1.26 or newer. Only the machine that *builds* needs vessel: `myapp` carries its own copy.

## Build a delivery

`./units` is your [quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html)
directory, the systemd units that each run a container.

```sh
$ vessel link ./units -o ./bundle --name acme --version 1.4.0
registry.example.com/acme/web:1.0            sha256:dcc94eaf6f8694…
registry.example.com/library/postgres:17.2   sha256:a7f2e782a04d50…
2 images, 3 files, bundle in ./bundle

$ vessel pack ./bundle -o myapp
myapp, 11 MB, run it on the target machine
```

Every `Image=` tag comes out a digest. Nothing else in your file moves, comments and ordering
included:

```ini
[Container]
-Image=registry.example.com/acme/web:1.0
+Image=registry.example.com/acme/web@sha256:dcc94eaf6f86942cb6c1b93566b705177d1fd9a4c729e85de5dc430ef17aeb78
Network=app.network
```

`myapp` is your whole delivery: copy it onto a stick and you are done. A bundle is also a plain OCI
layout, so a registry can carry it instead:

```sh
skopeo copy --all oci:./bundle:acme:1.4.0 docker://registry.example.com/deliveries/acme:1.4.0
```

## Install it on a machine

```sh
$ ./myapp inspect                        # reads and prints, writes nothing
acme 1.4.0, read by quadlet, resolved for linux/amd64 on 2026-09-05T15:11:56Z
image registry.example.com/acme/web:1.0   sha256:dcc94eaf6f8694…
file  etc/containers/systemd/web.container 315 bytes

$ ./myapp install
acme 1.4.0 installed: 2 images, 3 of 3 files changed

These secrets are not on this machine yet, the units need them:
  podman secret create db-password <file>

Start it:
  systemctl daemon-reload
  systemctl start db.service web.service
```

Secrets and certificates never travel inside `myapp`. When a unit asks for one, `install` names it
instead of letting the service fail later for no visible reason. Running `install` twice changes
nothing the second time.

If the machine is not ready, `install` says so **before writing anything**, and names every problem
at once:

```
vessel: this machine is not ready:
  podman is too old for quadlet: found 4.3.1, want 5.0 or newer
  cannot write to /etc/containers/systemd
```

## Requirements

The descriptor decides the runtime. Quadlet units install with podman and start with systemd, so
the machine needs Debian 13+, RHEL 9+ or Rocky 9+, podman 5.0 or newer, systemd, and root. The
floor is podman 4.4, where quadlet arrived, and Debian 12 ships 4.3.

## Signing

Integrity is always on: every part of a bundle is pinned by sha256, and vessel refuses a file that
does not match. Who *built* the file is a different question, and a binary cannot vouch for itself.
So print the checksum on the install sheet and have the operator compare it:

```sh
sha256sum myapp
```

`vessel pack --key vessel.key` also writes a `myapp.minisig`, for sites that run a
[minisign](https://jedisct1.github.io/minisign/) step.

Stronger, when you can put a binary in your machine image: build it with your public key baked in
(`make build KEY=vessel.pub`), sign the bundle with `vessel link --key`, and ship the bundle
directory rather than a packed file. That binary checks the signature itself, with nothing to
compare by hand.

## Contributing

```sh
go build ./cmd/vessel
go test ./...
```

Tests that need podman skip themselves, naming what is missing, so the suite is green on a laptop
with no container runtime.

MIT. See [LICENSE](LICENSE).
