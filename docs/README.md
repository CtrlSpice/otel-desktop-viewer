# Documentation

## Start here

- [Install the viewer](../README.md#1-install-the-viewer) — download and installation options.
- [Start the viewer](../README.md#3-start-the-viewer) — start the viewer and choose in-memory or file storage.
- [Use the viewer](../README.md#4-use-the-viewer) — choose the human or agent workflow.
- [Command-line options](command-line-options.md) — viewer flags, defaults, storage and ports.
- [SDK configuration guide](sdk-configuration.md) — send application telemetry over HTTP or gRPC, using environment variables or YAML.

## Use the viewer

- [Agent guide](../skills/otel-desktop-viewer/SKILL.md) — inspect telemetry through the CLI.
- [Docker guide](docker.md) — images, ports, persistent storage and Docker Compose.
- [OTLP JSON export and import](otlp-json-export.md) — file formats, downloads and browser imports.
- [Schema upgrades](schema-migration.md) — database compatibility and migrations.
- [Collector secret redaction](redaction.md) — data covered by ingestion redaction and its limits.

## Understand and develop the viewer

- [Architecture](ARCHITECTURE.md) — components, storage, APIs and frontend behaviour.
- [Metric resolution](metric-resolution.md) — how the viewer calculates metric chart data.
- [Contributor guide](CONTRIBUTING.md) — build, test and contribution instructions.
- [Code of Conduct](CODE_OF_CONDUCT.md).
