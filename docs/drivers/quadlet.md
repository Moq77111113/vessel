# quadlet

Reads [podman quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html) units, installs them with podman and systemd.

Target: podman 5.0+, systemd, root (Debian 13+, RHEL 9+, Rocky 9+).

```ini
# web.container
[Container]
Image=docker.io/library/nginx:1.27
Secret=WEB_TOKEN,type=env,target=TOKEN
```

- Lands in `/etc/containers/systemd/`; `.timer`, `.socket` and `.path` in `/etc/systemd/system/`.
- `Secret=` becomes a podman secret; `LoadCredentialEncrypted=` a credential sealed by `systemd-creds`, bound to the TPM2 when there is one.
