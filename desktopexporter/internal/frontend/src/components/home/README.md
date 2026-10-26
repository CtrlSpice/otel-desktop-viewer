# Home

## Pixel Lulu

The retained pixel Lulu artwork comes from
[CtrlSpice/lulu](https://github.com/CtrlSpice/lulu/blob/06495f736a62c975584a4e9235c1d85aa9e7627b/views/front.txt),
dedicated to the public domain under
[CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/).
`assets/images/lulu-pixel.txt` retains the source pixel grid. `Lulu.svelte` draws
it as inline SVG, without the telescope, using Edo-murasaki and Bishamonten inks.
Her body blends Edo-murasaki shades 6 and 7 equally; the gills blend Bishamonten
shades 4 and 3 with 25% of that skin colour to soften their saturation.
The original pixel palette guides the warm blush skin and rose gills, with the
retained plum outline. Lulu keeps her soft pink identity across ink, season
and Day/Night changes. The outline, shading, body, gills and eyes retain separate
colour roles; the palest Edo-murasaki swatch supplies her eye highlights.
Lulu Axol'Otel was created for OTel Desktop Viewer. The original painting is by
[Chelsey Bateson](https://cbatesonart.artstation.com/); it is a separate artwork
and is not covered by the pixel sprite's CC0 dedication.

## Discovery layout

Home's left pane contains Getting started, the file-drop box, Storage and
Ingestion issues. The dotted box matches the terminals' width and neutral border
colour. It has a fixed 96px height, centred content and a 2px dotted outline.
Pixel Lulu and the OTel Desktop Viewer wordmark form the sidebar header.
One of four humorous subtitles is selected on each Home visit and
shown beneath the wordmark, aligned with its left edge, in regular 12px Subtle text
with a 16px line-height. It stays fixed during
the visit and wraps naturally. The title and subtitle form one closely spaced
block, with 56px-wide Lulu vertically centred beside both lines and a 12px gap.
The import box sits 32px below the subtitle, followed by the exporter/test-trace
instructions in the same scrollable guide. Major groups have 32px gaps; each
instruction section groups its heading, description and code with 8px gaps.
The scrollable guide has no horizontal padding, so the import box and setup
instructions use the pane's full content width.
Shell snippets use explicit backslash continuations and native horizontal
scrolling rather than soft wrapping. Copy retains the executable command text.
The box opens the multi-file JSON/JSONL picker and says “Drop telemetry JSON
files here or anywhere on the screen.” File dropping works across the app.
Getting started retains its accessible section name and stays open as the primary
section of the resizable stack. The main pane contains the Service map, with each service
represented once and recorded cross-service parent/child connections drawn as
edges. Selecting a node keeps the map layout stable and shows matching records
in Details.

The HTTP and gRPC setup snippets use the receiver ports advertised by the running
viewer. If configuration is unavailable, the guide labels the standard fallback
port explicitly. The test-trace command uses the configured HTTP receiver port.

Storage identifies DuckDB's in-memory or on-disk mode, current usage and configured
retention limit. The default cap is 512 MiB in memory or 2 GiB on disk;
`--db-max-size 0` explicitly disables size-based retention.

Service cards use three shared card rows: service name and right-aligned Error
dot/count, then comma-separated trace, metric and log counts, then Last seen.
The lower rows align with the service name. The error total adds
error spans and error/fatal logs, with the breakdown on hover and in its accessible
label. Each service keeps a decorative assigned-colour dot before its name.
The right-hand Details pane contains matching records. Its single header row
offers shared Traces, Metrics and Logs tabs with known matching-record counts.
These tabs share the full header width evenly.
Tabs use the shared 4px choice gap. Service map view buttons use 32px targets
with the shared 2px icon-control gap.
The redundant visible Telemetry label is omitted; the section retains its
accessible name.
Choices switch immediately; arrow, Home and End keys navigate available signals.
Each signal retains independent pages and scroll positions. Telemetry remembers
its 25/50/100 page-size preference, defaulting to 25.
Selecting a service updates the matching records;
record cards still navigate to their native signal workspace.
