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
- `Secret=` becomes a podman secret: the app reads an environment variable, and podman stores the value unencrypted.
- `LoadCredentialEncrypted=` becomes a credential sealed by `systemd-creds`, bound to the TPM2 when there is one. It holds the value alone; mount it as a file and it never lands on disk in clear:

```ini
# database.container
[Container]
Volume=%d/DB_PASSWORD:/run/secrets/db_password:ro
Environment=POSTGRES_PASSWORD_FILE=/run/secrets/db_password

[Service]
LoadCredentialEncrypted=DB_PASSWORD
```

- Only root reads the mounted credential: the app reads it at start, then drops root, as the postgres image does.
- `EnvironmentFile=%d/…` copies the value into the container config on disk.
