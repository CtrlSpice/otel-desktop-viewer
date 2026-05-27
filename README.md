# otel-desktop-viewer

`otel-desktop-viewer` is a CLI tool for receiving OpenTelemetry **traces, metrics, and logs** on your local machine. It helps you visualize and explore telemetry without sending it to a vendor. Its goals are to be easy to install, have minimal dependencies, and stay fast.

It is written in Go as a custom exporter on top of the [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector).

~~Also, it has a dark mode~~ Y'all. I added another dark mode. It has **two** dark modes now.

<p align="center">
  <img src="docs/lulu.png" alt="Lulu the First — a pink axolotl striking a heroic pose while gazing at a field of stars through a telescope" width="480">
</p>

## Getting started

#### via Homebrew Cask

```bash
brew install --cask ctrlspice/tap/otel-desktop-viewer
```

#### via `go install`

Make sure you have [go](https://go.dev/) installed.

**Note**: This requires CGO compilation due to DuckDB dependencies.

**On Windows**: You'll need MSYS2 for CGO compilation:

1. **Install MSYS2**: Download and install from https://www.msys2.org/
2. **Open MSYS2 UCRT64 terminal**:
   - After installing MSYS2, you'll see multiple terminal options in the Start Menu
   - Choose **"MSYS2 UCRT64"** (not "MSYS2 MinGW 64-bit" or "MSYS2 MSYS")
   - Or run: `C:\msys64\ucrt64.exe`
3. **Install required packages**:
   ```bash
   pacman -S mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-toolchain
   ```
4. **Add MSYS2 to your PATH** (choose one):

   **Command Prompt (permanent)**:

   ```cmd
   setx PATH "%PATH%;C:\msys64\ucrt64\bin"
   ```

   **PowerShell (permanent)**:

   ```powershell
   [Environment]::SetEnvironmentVariable("PATH", [Environment]::GetEnvironmentVariable("PATH", "User") + ";C:\msys64\ucrt64\bin", "User")
   ```

   **PowerShell (current session only)**:

   ```powershell
   $env:PATH += ";C:\msys64\ucrt64\bin"
   ```

5. **Restart your terminal** for PATH changes to take effect
6. **Test the setup**:
   ```cmd
   gcc --version
   g++ --version
   ```

**On Linux/macOS**: CGO should work out of the box.

```bash
# install the CLI tool
go install github.com/CtrlSpice/otel-desktop-viewer@latest

# run it!
$(go env GOPATH)/bin/otel-desktop-viewer

# if you have $GOPATH/bin added to your $PATH you can call it directly!
otel-desktop-viewer

# if not you can add it to your $PATH by running this or adding it to
# your startup script (usually ~/.bashrc or ~/.zshrc)
export PATH="$(go env GOPATH)/bin:$PATH"
```

Running the CLI opens a browser tab to `localhost:8000` and starts OTLP receivers on `localhost:4318` (HTTP) and `localhost:4317` (gRPC).

#### via Docker

You can run otel-desktop-viewer using Docker without installing Go or building locally.

Pull from GitHub Container Registry:

```bash
# For AMD64 (most common)
docker pull ghcr.io/ctrlspice/otel-desktop-viewer:latest-amd64
docker run -p 8000:8000 -p 4317:4317 -p 4318:4318 ghcr.io/ctrlspice/otel-desktop-viewer:latest-amd64
```

```bash
# For ARM64 (Apple Silicon, etc.)
docker pull ghcr.io/ctrlspice/otel-desktop-viewer:latest-arm64
docker run -p 8000:8000 -p 4317:4317 -p 4318:4318 ghcr.io/ctrlspice/otel-desktop-viewer:latest-arm64
```

Or build locally:

```bash
docker build --tag otel-desktop-viewer:latest .
docker run -p 8000:8000 -p 4317:4317 -p 4318:4318 otel-desktop-viewer:latest
```

## Docker Compose

If your application is also running in Docker:

```yaml
services:
  app:
    image: your-apps-image-tag
    # Add your app configuration here

  otel-desktop-viewer:
    image: ghcr.io/ctrlspice/otel-desktop-viewer:latest-amd64 # Use latest-arm64 for ARM64 systems
    ports:
      - "8000:8000"
      - "4317:4317"
      - "4318:4318"
```

Your app can export to `otel-desktop-viewer:4318` (HTTP) or `otel-desktop-viewer:4317` (gRPC).

## Command line options

```bash
Flags:
      --browser-port int   Port for the web UI and JSON-RPC API (default 8000)
      --db string          DuckDB file path (default: in-memory)
      --grpc int           OTLP gRPC listen port (default 4317)
      --host string        Host for OTLP receivers and the web UI (default localhost)
      --http int           OTLP HTTP listen port (default 4318)
      --open-browser       Open the browser on launch (default true)
  -h, --help               help for otel-desktop-viewer
  -v, --version            version for otel-desktop-viewer
```

Persist telemetry to disk:

```bash
otel-desktop-viewer --db ./telemetry.duckdb
```

## Configuring your OpenTelemetry SDK

Configure an OTLP exporter to send to `http://localhost:4318` (HTTP) or `http://localhost:4317` (gRPC).

If your SDK supports [configuration via environment variables](https://opentelemetry.io/docs/concepts/sdk-configuration/otlp-exporter-configuration/), you can use:

```bash
# HTTP
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
export OTEL_TRACES_EXPORTER="otlp"
export OTEL_METRICS_EXPORTER="otlp"
export OTEL_LOGS_EXPORTER="otlp"
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"

# gRPC
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"
export OTEL_TRACES_EXPORTER="otlp"
export OTEL_METRICS_EXPORTER="otlp"
export OTEL_LOGS_EXPORTER="otlp"
export OTEL_EXPORTER_OTLP_PROTOCOL="grpc"
```

## Screenshots

![Traces view](docs/screenshots/traces.png)

![Metrics view](docs/screenshots/metrics.png)

![Logs view](docs/screenshots/logs.png)

## Example with `otel-cli`

If you have [`otel-cli`](https://github.com/equinix-labs/otel-cli) installed, it is a great way to send rich test traces from shell scripts. otel-cli supports span kinds, attributes, events, trace propagation, and background spans—much more than a single `exec` wrapper.

Start the desktop viewer in one terminal:

```bash
otel-desktop-viewer
```

In another terminal, point otel-cli at the viewer:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
```

**Quick span** — wrap any command:

```bash
otel-cli exec --service my-service --name "check the archive" curl -s -o /dev/null https://archive.org/
```

**Chained spans** — otel-cli propagates context automatically:

```bash
otel-cli exec --kind producer --service demo --name produce -- \
  otel-cli exec --kind consumer --service demo --name consume sleep 0.2
```

**Rich trace** — background span, events, attributes, and linked child spans:

```bash
sockdir=$(mktemp -d)
carrier=$(mktemp)

otel-cli span background \
  --service "otel-cli-example" \
  --name "script runtime" \
  --attrs "deployment.environment=local,team=platform" \
  --tp-carrier "$carrier" \
  --sockdir "$sockdir" &
sleep 0.1

otel-cli span event --name "starting work" --attrs "phase=setup,attempt=1" --sockdir "$sockdir"

otel-cli exec --service "otel-cli-example" --name "fetch example" --kind client \
  --attrs "http.url=https://example.com" \
  --tp-carrier "$carrier" \
  curl -s -o /dev/null https://example.com

otel-cli exec --kind producer --service "otel-cli-example" --name "hand off" \
  --tp-carrier "$carrier" -- \
  otel-cli exec --kind consumer --service "otel-cli-example" --name "process" sleep 0.1

otel-cli span event --name "finished" --attrs "phase=teardown,status=ok" --sockdir "$sockdir"
otel-cli span end --sockdir "$sockdir"
```

Open `http://localhost:8000/traces` to explore the result. For more otel-cli features (custom span times, `{{traceparent}}` in command args, config files, and a built-in TUI server), see the [otel-cli README](https://github.com/equinix-labs/otel-cli).

![otel-cli example trace](docs/screenshots/otel-cli-example.png)

## Implementation

The CLI is a custom OpenTelemetry Collector distribution. A `desktop` exporter:

- ingests traces, metrics, and logs into **DuckDB** (in-memory by default, optional on-disk persistence via `--db`)
- exposes data through a **JSON-RPC** API at `POST /rpc`
- serves a **Svelte** web UI embedded in the binary via [`go:embed`](https://go.dev/embed/)

See [ARCHITECTURE.md](ARCHITECTURE.md) for a full system overview.

## What's with the axolotl??

Her name is **Lulu Axol'Otel**. She is very pink, and I love her.

More seriously, I like to give my [side projects](https://github.com/CtrlSpice/bumblebee-consolematch) an [animal theme](https://github.com/CtrlSpice/yak-vs-yak) to add a little aesthetic interest on what otherwise might be fairly plain applications.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before participating.

## License

Apache 2.0, see LICENSE
