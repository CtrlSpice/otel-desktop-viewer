# Docker guide

Install Docker with Compose support to use these examples. The published image
supports Linux amd64 and arm64; Docker selects your architecture.

## Run the viewer

```sh
docker run -p 8000:8000 -p 4317:4317 -p 4318:4318 ghcr.io/ctrlspice/otel-desktop-viewer:latest
```

Open <http://localhost:8000>. The viewer runs in the foreground; Ctrl+C stops it.
To fetch an updated image before starting a new container:

```sh
docker pull ghcr.io/ctrlspice/otel-desktop-viewer:latest
```

## Connect an application

| Purpose | Container port | Host address |
| --- | --- | --- |
| Web UI and viewer API | `8000` | `http://localhost:8000` |
| OTLP gRPC receiver | `4317` | `http://localhost:4317` |
| OTLP HTTP receiver | `4318` | `http://localhost:4318` |

Port mappings use `HOST_PORT:CONTAINER_PORT`. For example, `-p 8001:8000` serves
the web UI and viewer API at `http://localhost:8001`.

Applications on the host use the published OTLP receiver addresses. Applications
in the same Compose network use the service name and OTLP receiver ports.
Inside a container, `localhost` refers to that container. See
[SDK configuration](sdk-configuration.md#choose-an-otlp-receiver) for exporter settings.

## Save telemetry between runs

Telemetry defaults to memory and is lost on shutdown. Mount a volume and set
`--db` to retain it:

```sh
docker run \
  -p 8000:8000 -p 4317:4317 -p 4318:4318 \
  --mount type=volume,source=otel-viewer-data,target=/data \
  ghcr.io/ctrlspice/otel-desktop-viewer:latest \
  --host 0.0.0.0 --db /data/telemetry.duckdb
```

Docker creates the named volume if needed. Reuse the volume and database path
on later runs, stopping the previous container before its replacement starts.

Arguments after the published image name replace its defaults. Include
`--host 0.0.0.0` when supplying flags so published ports remain reachable.
With file storage, periodic checks delete the oldest telemetry above a `2GB`
default threshold. See [Command-line options](command-line-options.md#choose-storage-and-retention)
for size syntax and retention behaviour.

## Use Docker Compose

Save as `compose.yaml`, or add the service and volume to an existing file:

```yaml
services:
  otel-desktop-viewer:
    image: ghcr.io/ctrlspice/otel-desktop-viewer:latest
    command: ["--host", "0.0.0.0", "--db", "/data/telemetry.duckdb"]
    ports:
      - "8000:8000"
      - "4317:4317"
      - "4318:4318"
    volumes:
      - otel-viewer-data:/data

volumes:
  otel-viewer-data:
```

Start it with:

```sh
docker compose up otel-desktop-viewer
```

Applications in the same Compose network use `http://otel-desktop-viewer:4318`
for OTLP HTTP or `http://otel-desktop-viewer:4317` for OTLP gRPC.
Open the web UI at `http://localhost:8000` in the host browser.

## Choose an image version

Use a [release tag](https://github.com/CtrlSpice/otel-desktop-viewer/releases)
instead of `latest` in the run command or Compose file:

```sh
docker run -p 8000:8000 -p 4317:4317 -p 4318:4318 ghcr.io/ctrlspice/otel-desktop-viewer:v0.7.0
```

Explicit architecture tags are available as `latest-amd64` and `latest-arm64`.

## Build from source

From the repository root, build the frontend and Go binary inside Docker:

```sh
docker build --tag otel-desktop-viewer:latest .
docker run -p 8000:8000 -p 4317:4317 -p 4318:4318 otel-desktop-viewer:latest
```

The source-build image uses `CMD`; the published image uses `ENTRYPOINT`.
For a source-build image, include the executable path when supplying flags:

```sh
docker run \
  -p 8000:8000 -p 4317:4317 -p 4318:4318 \
  --mount type=volume,source=otel-viewer-data,target=/data \
  otel-desktop-viewer:latest \
  /root/otel-desktop-viewer --host 0.0.0.0 --db /data/telemetry.duckdb
```
