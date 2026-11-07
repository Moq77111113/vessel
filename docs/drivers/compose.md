# compose

Reads `compose.yaml`, installs it with `docker compose`.

Target: Docker with the containerd image store (default on a fresh Docker 29; on Docker 28 set `"features": {"containerd-snapshotter": true}` in `/etc/docker/daemon.json`), compose 2.21+, root.

```yaml
name: station
services:
  screen:
    image: docker.io/grafana/grafana:11.5.0
    volumes:
      - ./grafana:/etc/grafana/provisioning:ro   # carried
    secrets:
      - source: SCREEN_PASSWORD
        uid: "472"                               # the file goes to uid 472, mode 0400
        mode: 0400
secrets:
  SCREEN_PASSWORD:
    file: ./secrets/SCREEN_PASSWORD              # always this form
```

- Lands in `/etc/compose/<name>/`; secrets in `/etc/compose/.secrets/`.
- Refused: `build:`, an image through `${VAR}`, a symbolic link, a path outside the directory.
