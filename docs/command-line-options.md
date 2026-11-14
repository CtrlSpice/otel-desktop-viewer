# Command-line options

`otel-desktop-viewer [flags]` starts the server in the foreground. By default,
it opens <http://localhost:8000> and stores telemetry in memory.

Client commands such as `query`, `traces` and `logs` connect to a running viewer
and have their own options. Check help for the installed version:

```sh
otel-desktop-viewer --help
otel-desktop-viewer query --help
```

## Viewer flags

| Flag | Value | Default | Behaviour |
| --- | --- | --- | --- |
| `--host` | Hostname or IP address | `localhost` | Address for the viewer HTTP server and both OTLP receivers. |
| `--http` | Integer port | `4318` | OTLP HTTP receiver port. |
| `--grpc` | Integer port | `4317` | OTLP gRPC receiver port. |
| `--browser-port` | Integer port | `8000` | Web UI and viewer API port. |
| `--open-browser` | `true` or `false` | `true` | Open the web UI after startup. |
| `--db` | File path | No file | Save telemetry in a DuckDB file instead of memory. |
| `--db-max-size` | Byte size or `0` | `512MB` in memory, `2GB` with `--db` | Delete the oldest telemetry above this size; `0` disables deletion. |
| `--self-telemetry-endpoint` | OTLP/gRPC endpoint URL | Unset | Send the viewer's own traces and metrics to a separate receiver. |
| `--feature-flags-file` | Local flagd JSON file | Unset | Configure features included at build time; cannot enable omitted features. |
| `-h`, `--help` | None | | Print help for the selected command. |
| `-v`, `--version` | None | | Print the viewer version; use on the root command. |

## Save telemetry between runs

```sh
otel-desktop-viewer --db ./telemetry.duckdb
```

Use the same file on later runs to read and add to that database. Relative paths
use the working directory. Without `--db`, telemetry is lost when the viewer
stops. See [Database upgrades](database-upgrades.md) before opening a file with
a different viewer version.

## Set a storage limit

Storage limits apply to both file and memory storage. For a file with a `4GB` cap:

```sh
otel-desktop-viewer --db ./telemetry.duckdb --db-max-size 4GB
```

Sizes accept whole numbers with case-insensitive `B`, `KB`, `MB`, `GB` or `TB`
suffixes. Units use powers of 1024; a bare number means bytes. Negative values
are invalid. `--db-max-size 0` disables automatic deletion.

The cap measures the telemetry store, not total process memory.

## Change the ports

```sh
otel-desktop-viewer --browser-port 8001 --http 4328 --grpc 4327
```

This serves the UI and APIs at `http://localhost:8001`, OTLP HTTP at
`http://localhost:4328`, and OTLP gRPC at `http://localhost:4327`.
Application exporters use the OTLP addresses; client commands use the viewer
HTTP address through `--endpoint`. See [SDK configuration](sdk-configuration.md).

## Start without opening a browser

```sh
otel-desktop-viewer --open-browser=false
```

The web UI remains available. For automation, reuse the intended running viewer.
If you start a child process, retain its handle, wait for its HTTP endpoint to
respond, and stop and wait for only that child. The
[Agent guide](../skills/otel-desktop-viewer/SKILL.md) covers process management.

## Observe the viewer

Start a monitoring viewer in one terminal:

```sh
otel-desktop-viewer --grpc 4327 --http 4328 --browser-port 8001
```

In another terminal, start the viewer whose internal traces and metrics you
want to inspect:

```sh
otel-desktop-viewer --self-telemetry-endpoint http://localhost:4327
```

The monitoring UI is at `http://localhost:8001`; the observed viewer is at
`http://localhost:8000`. Omitting the endpoint keeps self-telemetry off.
The receiver must be outside the observed viewer and accept OTLP over gRPC.

Keep the monitoring viewer running through the observed viewer's shutdown.
In automation, retain both process handles and stop and wait for the observed
viewer first. Stop only processes you started.
