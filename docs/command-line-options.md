# Command-line options

`otel-desktop-viewer [flags]` starts the server in the foreground.

Client commands such as `query`, `traces` and `logs` connect to a running viewer
and have their own options. The [Agent guide](../skills/otel-desktop-viewer/SKILL.md)
covers client commands and process management.

Use `--help` (`-h`) for command help, or `--version` (`-v`) on the root command:

```sh
otel-desktop-viewer --help
otel-desktop-viewer query --help
otel-desktop-viewer --version
```

## Set addresses and browser behaviour

| Option | Default | Behaviour |
| --- | --- | --- |
| `--host <address>` | `localhost` | Hostname or IP address for the viewer HTTP server and both OTLP receivers. |
| `--browser-port <port>` | `8000` | Web UI and viewer API port. |
| `--http <port>` | `4318` | OTLP HTTP receiver port. |
| `--grpc <port>` | `4317` | OTLP gRPC receiver port. |
| `--open-browser=<bool>` | `true` | Open the web UI after startup. `--open-browser=false` leaves it available without opening a browser. |

Application exporters use the OTLP addresses; see [SDK configuration](sdk-configuration.md).
Client commands use `--endpoint` with the viewer HTTP address.

## Choose storage and retention

| Option | Default | Behaviour |
| --- | --- | --- |
| `--db <path>` | Omitted | Save telemetry in a DuckDB file. Without this option, storage is in memory and is lost on shutdown. |
| `--db-max-size <size>` | `512MB` in memory, `2GB` with `--db` | Set the size threshold for automatic deletion of the oldest telemetry; `0` disables automatic deletion. |

To save telemetry with a custom threshold:

```sh
otel-desktop-viewer --db ./telemetry.duckdb --db-max-size 4GB
```

Reuse the file on later runs to read and add telemetry. Relative paths use the
working directory. See [Database upgrades](database-upgrades.md) before opening
the file with a different viewer version.

Sizes accept non-negative whole numbers with optional, case-insensitive `B`,
`KB`, `MB`, `GB` or `TB` suffixes. Units use powers of 1024; a bare number means bytes.

With a nonzero threshold, the viewer checks store size every 30 seconds. Usage can
exceed the threshold. The measurement covers telemetry storage, not total
process memory.

## Observe the viewer

| Option | Default | Behaviour |
| --- | --- | --- |
| `--self-telemetry-endpoint <url>` | Omitted | Send the viewer's own traces and metrics to a separate OTLP/gRPC receiver. Omit to keep self-telemetry off. |

Start a monitoring viewer in one terminal:

```sh
otel-desktop-viewer --browser-port 8001 --http 4328 --grpc 4327
```

In another terminal, start the viewer to observe:

```sh
otel-desktop-viewer --self-telemetry-endpoint http://localhost:4327
```

The monitoring UI is at `http://localhost:8001`; the observed viewer's UI is at
`http://localhost:8000`. Stop the observed viewer and wait for it to exit before
stopping the monitoring viewer.

## Build-specific option

`--feature-flags-file <path>` reads local feature settings from a flagd JSON
file in `EVALS=1` builds. It is omitted by default and can configure only
features included in the build. Public builds reject a supplied file.
