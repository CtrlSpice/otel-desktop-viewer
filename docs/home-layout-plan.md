# Home layout plan

Status: proposed layout, not implemented. Home is part of the release scope.

Home should answer three questions: **What data is here? How do I add data?
Where do I investigate it?**

## Where things belong

| Location | Content |
| --- | --- |
| Global header | Existing Home, Traces, Metrics and Logs navigation; Evals only when enabled. Keep shared appearance controls here. |
| Home: main column, first section | **Explore data**: compact links to Traces, Metrics and Logs, with their own counts and last-received dates. A small Evals entry appears only when enabled. |
| Home: main column, second section | **Add data**: the existing OTLP file importer and **Connect an exporter** instructions, with HTTP/gRPC tabs and exact Copy. |
| Home: side column | **This viewer**: available storage information, last successful status refresh, and ingestion/import issues. Preserve the `#ingestion-issues` destination used by other pages. |
| Home: lower section | **Help and examples**: SDK/Collector setup, CLI guidance and a small example. Keep longer code examples collapsed until requested. |
| Home: identity/footer | Keep Lulu and the artwork credit; use a compact identity area so data and actions appear first. |
| Signal workspaces | Queries, time ranges, sorting, charts/tables, record Details and scoped Copy/Download stay with the signal they operate on. |
| Evals workspace | Database-wide Run list, producer-native file import, Run Results/Tool calls/Cost, row Details and Telemetry links. Runtime execution remains in the explicit eval CLI workflow. |

Home uses a two-column content layout that stacks on narrow windows. It does
not need the empty signal-list drawer or a record inspector. Remove the disabled
Home search and the “more coming soon” placeholder when implementing this layout.

```text
Shared navigation and appearance controls
Home · compact Lulu identity
┌──────────────────────────────────┬────────────────────────┐
│ Explore data                     │ This viewer            │
│ Traces · Metrics · Logs · Evals*  │ Storage · last refresh │
│ Counts and last received         │ Ingestion/import issues│
├──────────────────────────────────┤                        │
│ Add data                         │                        │
│ Import file · Connect exporter   │                        │
├──────────────────────────────────┴────────────────────────┤
│ Help and examples · artwork credit                        │
└───────────────────────────────────────────────────────────┘
* Only when enabled
```

Narrow order: identity, Explore data, Add data, This viewer, Help. For an empty
store, Add data precedes Explore data in both layouts.

## States and meanings

- **Empty store:** put Add data first. Keep the empty signal destinations visible
  with genuine zero counts after a successful status response.
- **Populated store:** put Explore data first; keep Add data immediately below.
- **Loading or failed status:** show unavailable values, not zero. If a previous
  response exists, identify it as the last successful reading.
- **Ingestion issues:** show a concise visible summary and expandable recorded
  details. A rejected record is distinct from an error span or error log.
- **Scope:** Home's store totals are labelled as such. They do not imply the
  selected time range or query of another workspace.
- **Dates:** distinguish last received by this viewer from the telemetry's
  recorded timestamp. Recent receipt alone is not proof of a live connection.
- **Evals disabled:** omit its Home entry and requests, using `evals.enabled`.
  Use the database-wide Run summary total when Evals is enabled.

Use the existing regular 14px working text, 16px headings, plain bracketed
counts, small meaningful dots and shared hover/selection surfaces. Buttons
remain still on press/release. No palette changes are part of this plan.

## Existing wiring and remaining decisions

- `HomePage.svelte` already receives the importer and import failures, polls
  `getStats`, and renders `IngestionIssues`.
- `getStats` supplies signal counts, last-received values and storage statistics.
  Use those fields without inferring health, persistence or service availability.
- `getImportConfig` supplies the OTLP HTTP port for file import. The current Home
  exporter snippets hardcode default ports. Accurate custom HTTP/gRPC instructions
  need an explicit runtime configuration contract for advertised endpoints.
  Decide that contract before adding connection-status controls.
- Keep telemetry-file import and producer-native eval import explicit. They have
  different formats and storage contracts; Evals retains authoritative source
  JSON and builds disposable indexes over it.
- Saved workspace/view revisions remain separate work: dataset binding, retention
  and portable-bundle contents still need decisions in
  `eval-integration-roadmap.md`. Add a saved-view Home section when that contract
  and navigation are implemented.

## Implementation order

1. Agree on the section order and content ownership above.
2. Replace the current Home shell using the existing importer, stats and issues;
   implement the empty/populated/loading/error states.
3. Settle advertised receiver configuration and make copied setup snippets match
   it. Preserve custom-port operation and exact Copy.
4. Verify navigation, import/partial failure, issue anchors, feature-off behavior,
   narrow layout and keyboard access; run the application quality gate.

A passing backend/Evals gate is not completion of this Home work.
