# Command-line options

These options configure the `otel-desktop-viewer` server: its network addresses,
telemetry storage and browser launch behaviour.

For installation, see [Install the viewer](../README.md#1-install-the-viewer).
For application exporters, see the [SDK configuration guide](sdk-configuration.md).

## Usage

```sh
otel-desktop-viewer [flags]
```

The command starts the viewer in the foreground. Without flags, it opens the
browser at `http://localhost:8000` and stores telemetry in memory.

Read the help for your installed version:

```sh
otel-desktop-viewer --help
```

Client commands such as `query`, `traces` and `logs` connect to an existing viewer.
They have separate options. Read a client's help with its command name:

```sh
otel-desktop-viewer query --help
```

## Viewer flags

| Flag | Value | Default | Behaviour |
| --- | --- | --- | --- |
| `--host` | Hostname or IP address | `localhost` | Address used by the viewer HTTP server and both OTLP receivers. |
| `--http` | Integer port | `4318` | OTLP HTTP receiver port. |
| `--grpc` | Integer port | `4317` | OTLP gRPC receiver port. |
| `--browser-port` | Integer port | `8000` | HTTP port for the web UI and viewer APIs. |
| `--open-browser` | `true` or `false` | `true` | Open the web UI in a browser after startup. Use `--open-browser=false` to disable this. |
| `--db` | Database file path | No file | Save telemetry in a DuckDB file. Without a file, telemetry stays in memory. |
| `--db-max-size` | Byte size, such as `512MB`, `2GB` or `0` | `512MB` in memory, `2GB` with `--db` | Delete the oldest telemetry when the store exceeds this size. `0` disables this deletion. |
| `--self-telemetry-endpoint` | OTLP/gRPC endpoint URL | Unset | Send the viewer's own traces and metrics to this endpoint. Unset keeps self-telemetry off. See [Observe the viewer](#observe-the-viewer). |
| `--feature-flags-file` | Local flagd JSON file path | Unset | Configure experimental features included in the build. Public builds cannot enable Evals through this flag. |

## Help and version

| Flag | Behaviour |
| --- | --- |
| `-h`, `--help` | Print help for the selected command. |
| `-v`, `--version` | Print the installed viewer version. Use this flag on the root command. |

## Save telemetry between runs

By default, telemetry is kept in memory and lost when the viewer stops.
Use `--db` to save it to a file instead:

```sh
otel-desktop-viewer --db ./telemetry.duckdb
```

Start the viewer with the same file to read the saved telemetry and receive new
telemetry into that database. Relative paths use the viewer's working directory.

Storage limits apply in both modes. Saving to a file does not disable deletion
of old telemetry when the store exceeds its limit.

## Set a storage limit

This command saves telemetry to a file and sets a `4GB` storage limit:

```sh
otel-desktop-viewer --db ./telemetry.duckdb --db-max-size 4GB
```

Size values accept whole numbers with `B`, `KB`, `MB`, `GB` or `TB` suffixes.
Suffixes are case-insensitive. Units use powers of 1024: `1KB` means 1024 bytes.
A number without a suffix means bytes. Negative values are invalid.

Use `--db-max-size 0` to disable automatic deletion of old telemetry.
The limit applies to the telemetry store, not the viewer's total process memory.

## Change the ports

```sh
otel-desktop-viewer --browser-port 8001 --http 4328 --grpc 4327
```

This command uses these addresses:

| Purpose | Address |
| --- | --- |
| Web UI and viewer APIs | `http://localhost:8001` |
| OTLP HTTP | `http://localhost:4328` |
| OTLP gRPC | `http://localhost:4327` |

Configure application exporters to use the matching OTLP address.
Client commands use the viewer HTTP address through their `--endpoint` option.

## Start without opening a browser

```sh
otel-desktop-viewer --open-browser=false
```

This flag disables automatic browser launch. The web UI remains available.

For agents, read the bundled guide before managing a viewer process:

```sh
otel-desktop-viewer skills
```

Reuse the intended viewer if it is running. If you start a viewer child process,
retain its process handle and check that its HTTP endpoint responds before sending commands.
Terminate and wait for only the child you started. Do not stop another viewer.

## Observe the viewer

To debug the viewer itself, send its internal traces and metrics to a separate
monitoring viewer.

Start the monitoring viewer in one terminal:

```sh
otel-desktop-viewer --grpc 4327 --http 4328 --browser-port 8001
```

Start the viewer you want to observe in another terminal:

```sh
otel-desktop-viewer --self-telemetry-endpoint http://localhost:4327
```

Open `http://localhost:8001` to inspect the observed viewer's telemetry.
The observed viewer's own UI remains at `http://localhost:8000`.

Omitting `--self-telemetry-endpoint` keeps self-telemetry off.
The endpoint must receive OTLP over gRPC outside the observed viewer.
Keep the monitoring viewer running until the observed viewer finishes shutting down.
For automation, retain both child process handles. Stop and wait for the observed
viewer before stopping and waiting for the monitoring viewer. Stop only processes
you started.

## Related documentation

- [Schema upgrades](schema-migration.md) — open databases across viewer versions.
- [Documentation index](README.md).
