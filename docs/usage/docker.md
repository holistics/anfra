# Running anfra with Docker

`ghcr.io/holistics/anfra` runs `anfra serve` for a repo at `/repo`: the core API, MCP and the Data Apps, on port 7878. How the image is built, and why, is in [designs/release.md](../designs/release.md#the-docker-image).

## Run it on a repo

Mount the repo at `/repo` and publish the port:

```sh
docker run -p 7878:7878 -v "$PWD:/repo" ghcr.io/holistics/anfra
```

Then `http://localhost:7878/` lists the repo's Data Apps; the core API is under `/api` and MCP at `/mcp`. The repo's `.anfra/data_sources.yml` comes with the mount, credentials included.

The image's command is `serve --addr 0.0.0.0:7878`: every interface, so a published port reaches it. `serve` has no authentication ([designs/security.md](../designs/security.md)): publish the port only where everyone who reaches it may query the repo's data.

**Behind a proxy** (TLS, another host name, a path prefix): add `-e ANFRA_SITE_URL=https://data.example.com/anfra`, so the links the server makes (an export's download) point where callers reach it. Without it, a link uses the host the caller's request named, which is right when they reach the published port directly.

**Other `serve` flags** replace the whole command, so repeat the address:

```sh
docker run -p 7878:7878 -v "$PWD:/repo" ghcr.io/holistics/anfra serve --addr 0.0.0.0:7878 --no-watch
```

`--no-watch` turns off live reload, for hosting Data Apps whose files do not change. (While no Data App is open, live reload watches nothing anyway.)

**Serving other people:** add `-e ANFRA_HIDE_ERROR_CAUSES=1`. By default anfra shows an error's cause to whoever made the request, which helps the person running it and can show others SQL, database errors and file paths ([designs/security.md](../designs/security.md)).

**Other commands** run the same way, in the mounted repo:

```sh
docker run --rm -v "$PWD:/repo" ghcr.io/holistics/anfra validate
docker run --rm -v "$PWD:/repo" ghcr.io/holistics/anfra query --dataset sales '…'
```

anfra only reads the repo, so the mount may be read-only (`-v "$PWD:/repo:ro"`), as long as its files are readable by UID 1000.

## Build an image with a repo in it

To deploy a fixed version of a repo, copy it onto the image:

```dockerfile
FROM ghcr.io/holistics/anfra:<version>@sha256:<digest>
COPY --chown=anfra:anfra . /repo
```

- **Pin a version and its digest,** so a rebuild gets the same anfra. `docker buildx imagetools inspect ghcr.io/holistics/anfra:<version>` prints the digest.
- **Avoid root steps.** The image sets `HOME=/home/anfra`, and that applies to a step run as root too: a tool it runs may create root-owned folders in anfra's home, which anfra then cannot write to, and it fails at startup with `permission denied`. (Rosetta does this when building amd64 on Apple silicon.) If a step needs root, run it with `HOME=/root`, and switch back with `USER anfra`.
- **Mind the credentials.** The copied `.anfra/data_sources.yml` is in the image: push it only to a private registry.

## Users

Run the image as its own user, `anfra` (UID 1000): don't override it with `--user` or a different `runAsUser`. A mount does not need it either: anfra writes only to its home, not the repo.

Another UID is not supported today. anfra's home is `anfra`'s, and canal-query looks up the process's user and exits when it has no passwd entry. In Kubernetes, if the cluster requires a different UID, this is a blocker for now.

## Logs

anfra writes its log, with both sidecars', to `/home/anfra/.anfra/repos/<repo id>/logs/anfra.log` in the container. To see it in `docker logs`, set `ANFRA_LOG_STDERR=1`:

```sh
docker run -p 7878:7878 -v "$PWD:/repo" -e ANFRA_LOG_STDERR=1 ghcr.io/holistics/anfra
```

`LOG_LEVEL` sets the level. To keep state across container restarts (logs, compile caches), mount a volume at `/home/anfra/.anfra`.

## Telemetry

Set `OTEL_EXPORTER_OTLP_ENDPOINT` to an OTLP/HTTP collector the container can reach, and anfra exports traces, its sidecars' included. See [configs.md](configs.md#telemetry).

## Platforms

One image name serves every platform: each tag holds a linux/amd64 and a linux/arm64 build, and Docker pulls the one matching the machine, Apple silicon included. Docker on macOS and Windows runs these Linux images in its VM. `--platform` forces the other one, under emulation.
