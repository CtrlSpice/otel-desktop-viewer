# SDK configuration guide

Configure your application's OpenTelemetry exporter to send traces, metrics and
logs to `otel-desktop-viewer` using OTLP, the OpenTelemetry Protocol.

These settings belong to your application, not the viewer process.

## Prerequisites

- Install and start the viewer. See [Start the viewer](../README.md#3-start-the-viewer).
- Instrument your application with an OpenTelemetry SDK and install its OTLP exporter.
- Check which signals, protocols and configuration methods your SDK supports.
  Environment variables alone do not instrument an application.

The examples assume the application and viewer run on the same machine with the
viewer's default ports. In a container, `localhost` refers to that container.
For applications in Docker Compose, see [Use Docker Compose](docker.md#use-docker-compose).

## Choose an endpoint

| Export protocol | Default endpoint | Protocol setting |
| --- | --- | --- |
| OTLP HTTP with protobuf | `http://localhost:4318` | `http/protobuf` |
| OTLP gRPC | `http://localhost:4317` | `grpc` |

Port `8000` serves the web UI and viewer APIs. It does not receive OTLP.
If you changed the viewer's host or receiver ports, use those addresses instead.
See [Command-line options](command-line-options.md).

## Configure with environment variables

Use these examples if your SDK supports
[environment-variable configuration](https://opentelemetry.io/docs/concepts/sdk-configuration/otlp-exporter-configuration/).
Choose one protocol. Set its variables in the application's environment before
starting the application. The commands below use POSIX shell syntax.

### Send over HTTP

```sh
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_TRACES_EXPORTER=otlp
export OTEL_METRICS_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=otlp
```

The shared endpoint is a base URL. The SDK appends `/v1/traces`, `/v1/metrics`
or `/v1/logs` for each signal.

Signal-specific endpoint variables override the shared endpoint. They require
the complete HTTP path:

| Variable | Default viewer URL |
| --- | --- |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | `http://localhost:4318/v1/traces` |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | `http://localhost:4318/v1/metrics` |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | `http://localhost:4318/v1/logs` |

Signal-specific protocol variables also override `OTEL_EXPORTER_OTLP_PROTOCOL`.
Check for existing overrides when changing protocols.
See the [OTLP exporter specification](https://opentelemetry.io/docs/specs/otel/protocol/exporter/).

### Send over gRPC

```sh
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export OTEL_TRACES_EXPORTER=otlp
export OTEL_METRICS_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=otlp
```

For gRPC, use the receiver address without HTTP signal paths such as `/v1/traces`.

## Use declarative configuration

SDKs that support
[declarative configuration](https://opentelemetry.io/docs/languages/sdk-configuration/declarative-configuration/)
can read a YAML file instead. Check that your SDK supports the file format used
below (`1.1`).

Save this as `otel-config.yaml`:

```yaml
file_format: "1.1"

resource:
  attributes:
    - name: service.name
      value: my-service

tracer_provider:
  processors:
    - batch:
        exporter:
          otlp_http:
            endpoint: http://localhost:4318/v1/traces

meter_provider:
  readers:
    - periodic:
        exporter:
          otlp_http:
            endpoint: http://localhost:4318/v1/metrics

logger_provider:
  processors:
    - batch:
        exporter:
          otlp_http:
            endpoint: http://localhost:4318/v1/logs
```

Replace `my-service` with your application's service name.
The `otlp_http` exporters require complete signal URLs, as shown above.

Set `OTEL_CONFIG_FILE` to the file's path before starting your application:

```sh
export OTEL_CONFIG_FILE=/path/to/otel-config.yaml
```

With declarative configuration, SDKs ignore other SDK environment variables
unless the file explicitly references them through substitution, such as `${OTEL_SERVICE_NAME}`.
Set endpoints in the file, rather than expecting `OTEL_EXPORTER_OTLP_ENDPOINT`
to override them.

## Check the result

Start your application with the chosen configuration and generate telemetry.
Open the viewer at `http://localhost:8000`, or use your configured viewer HTTP address.
Inspect the corresponding Traces, Metrics or Logs workspace.

If telemetry does not appear, check the application's exporter errors, endpoint,
protocol and signal-specific overrides.

### Send a test trace

**Optional:** Use Amy Tobey's [otel-cli](https://github.com/tobert/otel-cli) for a
quick test. Amy is awesome, and we love this project.

With `otel-cli` installed and the viewer running, send a trace over OTLP HTTP:

```sh
otel-cli exec --endpoint http://localhost:4318 --protocol http/protobuf \
  --service viewer-test --name "test trace" --fail -- echo "Hello, telemetry"
```

Open `http://localhost:8000/traces` and look for `test trace` from `viewer-test`.
This checks trace ingestion without instrumenting an application.

## Related documentation

- [Documentation index](README.md).
