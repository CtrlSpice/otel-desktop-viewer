# SDK configuration

Send application traces, metrics and logs to the viewer using OTLP, the
OpenTelemetry Protocol. Configure these settings in your application.
You need an instrumented application with an OpenTelemetry SDK and its OTLP
exporter; environment variables alone do not provide instrumentation.

[Start the viewer](../README.md#3-start-the-viewer) before using these examples.
They assume both processes run on the same machine with default ports.
For containers, see [Docker guide](docker-guide.md#use-docker-compose).

## Choose an OTLP receiver

| Export protocol | Default receiver address | Protocol setting |
| --- | --- | --- |
| OTLP HTTP with protobuf | `http://localhost:4318` | `http/protobuf` |
| OTLP gRPC | `http://localhost:4317` | `grpc` |

The web UI and viewer API use `http://localhost:8000`; exporters use the OTLP
receivers above. If you [change addresses or ports](command-line-options.md#set-addresses-and-browser-behaviour),
update the exporter settings to match.

## Configure with environment variables

Check which signals, protocols and configuration methods your SDK supports.
For [environment-variable configuration](https://opentelemetry.io/docs/concepts/sdk-configuration/otlp-exporter-configuration/),
choose one protocol and set its variables before starting the application.
These commands use POSIX shell syntax.

Signal-specific endpoint and protocol variables override `OTEL_EXPORTER_OTLP_ENDPOINT`
and `OTEL_EXPORTER_OTLP_PROTOCOL`. Check for existing overrides when changing
protocols. The [OTLP exporter specification](https://opentelemetry.io/docs/specs/otel/protocol/exporter/)
defines their precedence.

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

Signal-specific HTTP endpoints require the complete signal URL:

| Variable | Default OTLP HTTP endpoint |
| --- | --- |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | `http://localhost:4318/v1/traces` |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | `http://localhost:4318/v1/metrics` |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | `http://localhost:4318/v1/logs` |

### Send over gRPC

```sh
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export OTEL_TRACES_EXPORTER=otlp
export OTEL_METRICS_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=otlp
```

OTLP gRPC uses the receiver address without HTTP signal paths such as `/v1/traces`.

## Use declarative configuration

If your SDK supports [declarative configuration](https://opentelemetry.io/docs/languages/sdk-configuration/declarative-configuration/)
format `1.1`, save this as `otel-config.yaml`:

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

Replace `my-service` with your service name. The `otlp_http` exporters require
complete signal URLs. Set the file path before starting your application:

```sh
export OTEL_CONFIG_FILE="$PWD/otel-config.yaml"
```

Declarative configuration ignores other SDK environment variables unless the
file references them through substitution, such as `${OTEL_SERVICE_NAME}`.
Set endpoints in the file; `OTEL_EXPORTER_OTLP_ENDPOINT` does not override them.

## Check the result

Start your application, generate telemetry and open the matching Traces, Metrics
or Logs workspace at `http://localhost:8000` (or your configured web UI address).

If telemetry does not appear, check the application's exporter errors, endpoint,
protocol and signal-specific overrides.

### Send a test trace

Amy Tobey's [otel-cli](https://github.com/tobert/otel-cli) can test trace ingestion
without instrumenting an application. Amy is awesome, and we love this project.

With `otel-cli` installed and the viewer running, send a trace over OTLP HTTP:

```sh
otel-cli exec --endpoint http://localhost:4318 --protocol http/protobuf \
  --service viewer-test --name "test trace" --fail -- echo "Hello, telemetry"
```

Open `http://localhost:8000/traces` and look for `test trace` from `viewer-test`.
