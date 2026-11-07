# vessel.yaml

```yaml
name: acme
version: 1.4.0
source: ./stack                  # optional: where the stack is
files:
  - source: realm.json
    target: /etc/acme/realm.json # ###PUBLIC_HOST### inside is replaced at install
variables:
  - name: PUBLIC_HOST            # --set PUBLIC_HOST=acme.example.com
  - name: DB_PASSWORD
    secret: true                 # --set-file DB_PASSWORD=/root/db, never on the command line
    from: openssl rand -hex 32   # or computed at install
actions:
  - mkdir -p /etc/acme/certs     # before the files
after:
  - cmd: echo "open https://$(hostname -f)/"   # once every service runs
```

- vessel never prompts: a missing value stops the install before anything changes.
- Write images with their registry: `docker.io/library/postgres:17.2`.
