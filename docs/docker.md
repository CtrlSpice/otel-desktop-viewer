# Docker guide

Docker runs the viewer without a local Go installation. Install Docker with
Compose support before using these examples.

## Run the published image

```sh
docker run -p 8000:8000 -p 4317:4317 -p 4318:4318 ghcr.io/ctrlspice/otel-desktop-viewer:latest
```

Docker downloads the image if it is missing locally. The image supports Linux
amd64 and arm64. Docker selects the image for your architecture.

Open `http://localhost:8000` in your host browser. The viewer runs in the
foreground. Press Ctrl+C to stop it.

To download an updated image before starting a new container:

```sh
docker pull ghcr.io/ctrlspice/otel-desktop-viewer:latest
```

## Connect to the container

The launch command publishes these ports on the Docker host:

| Purpose | Container port | Host address |
| --- | --- | --- |
| Web UI and viewer APIs | `8000` | `http://localhost:8000` |
| OTLP gRPC | `4317` | `http://localhost:4317` |
| OTLP HTTP | `4318` | `http://localhost:4318` |

Each `-p` option has the form `HOST_PORT:CONTAINER_PORT`. For example,
`-p 8001:8000` serves the web UI at `http://localhost:8001` on the host.

Applications on the host use the published OTLP ports.
Applications in the same Compose network use the viewer's service name and
container ports, as shown in [Use Docker Compose](#use-docker-compose).
Inside a container, `localhost` refers to that container.

See the [SDK configuration guide](sdk-configuration.md) for exporter settings.

## Save telemetry between runs

By default, telemetry stays in memory and is lost when the viewer stops.
Mount a Docker volume and use `--db` to save telemetry outside the container:

```sh
docker run \
  -p 8000:8000 -p 4317:4317 -p 4318:4318 \
  --mount type=volume,source=otel-viewer-data,target=/data \
  ghcr.io/ctrlspice/otel-desktop-viewer:latest \
  --host 0.0.0.0 --db /data/telemetry.duckdb
```

Docker creates the named volume `otel-viewer-data` if it does not exist.
Start subsequent containers with the same volume and database path to reuse the
saved telemetry. Stop the previous viewer container before starting its replacement.

The published image runs the viewer as its entrypoint. Arguments after the image
name replace its default arguments. Include `--host 0.0.0.0` when supplying viewer
flags so the published ports remain reachable.

File storage defaults to a `2GB` limit. Old telemetry is deleted when the store
exceeds the limit. See [Command-line options](command-line-options.md) to change it.

## Use Docker Compose

Save this as `compose.yaml`, or add the service and volume to your existing
Compose file:

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

This example saves telemetry in a named volume. Start the viewer service:

```sh
docker compose up otel-desktop-viewer
```

For an application in the same Compose network, use one of these exporter endpoints:

| Export protocol | Endpoint |
| --- | --- |
| OTLP HTTP with protobuf | `http://otel-desktop-viewer:4318` |
| OTLP gRPC | `http://otel-desktop-viewer:4317` |

The browser on your host still uses `http://localhost:8000`.

## Choose an image version or architecture

Use a version tag instead of `latest` to select a specific release. For example:

```sh
docker pull ghcr.io/ctrlspice/otel-desktop-viewer:v0.7.0
```

Use that same tag in your `docker run` command or Compose `image` field.
See [Releases](https://github.com/CtrlSpice/otel-desktop-viewer/releases) for available versions.

Explicit architecture tags are also available:

```sh
docker pull ghcr.io/ctrlspice/otel-desktop-viewer:latest-amd64
docker pull ghcr.io/ctrlspice/otel-desktop-viewer:latest-arm64
```

## Build from source

Run these commands from the repository root. The build compiles the frontend
and Go binary inside Docker:

```sh
docker build --tag otel-desktop-viewer:latest .
docker run -p 8000:8000 -p 4317:4317 -p 4318:4318 otel-desktop-viewer:latest
```

The source-build image uses `CMD` rather than the published image's `ENTRYPOINT`.
To supply viewer flags to a source-build image, include the executable path:

```sh
docker run \
  -p 8000:8000 -p 4317:4317 -p 4318:4318 \
  --mount type=volume,source=otel-viewer-data,target=/data \
  otel-desktop-viewer:latest \
  /root/otel-desktop-viewer --host 0.0.0.0 --db /data/telemetry.duckdb
```

See the [documentation index](README.md) for other configuration and usage guides.
