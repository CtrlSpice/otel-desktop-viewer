# Home

## Pixel Lulu

The welcome uses the front-view pixel Lulu from
[CtrlSpice/lulu](https://github.com/CtrlSpice/lulu/blob/06495f736a62c975584a4e9235c1d85aa9e7627b/views/front.txt),
dedicated to the public domain under
[CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/).
`assets/images/lulu-pixel.txt` retains the source pixel grid. `Lulu.svelte` draws
it as inline SVG, without the telescope, using fixed official Edo-murasaki,
Fuyu-gaki and Bishamonten swatches. Lulu keeps her soft pink identity across ink, season
and Day/Night changes. The outline, shading, body, gills and eyes retain separate
colour roles; the palest Edo-murasaki swatch supplies her eye highlights.
Lulu Axol'Otel was created for OTel Desktop Viewer. The original painting is by
[Chelsey Bateson](https://cbatesonart.artstation.com/); it is a separate artwork
and is not covered by the pixel sprite's CC0 dedication.

The welcome is an 80px-high horizontal lockup: Lulu and the complete product name,
vertically centred with a 24px gap. Lulu's column shrinks on narrow drawers to
keep the title on one line at the regular 16px Atkinson Mono size. It has no
tagline or decorative prompt.

## Discovery layout

Home's left pane contains the welcome, file-drop control, Getting started,
Storage and Ingestion issues. Getting started is the primary open section of the
shared resizable stack. The main pane contains the Service map, with each service
represented once and recorded cross-service parent/child connections drawn as
edges. Selecting a node keeps the map layout stable and shows matching records
in Details. The welcome has no decorative prompt before the app name.

Service cards use three shared card rows: service name and right-aligned Error
dot/count, then comma-separated trace, metric and log counts, then Last seen.
The lower rows align with the service name. The error total adds
error spans and error/fatal logs, with the breakdown on hover and in its accessible
label. Each service keeps a decorative assigned-colour dot before its name.
Attribute queries, typed values, sorting, filtering and exact Copy retain their
existing scope and behaviour. The header's filter icon opens and focuses the
attribute-key filter. Escape collapses the field and restores icon focus without
clearing the filter; an active filter remains marked on the icon.

Records is the number of distinct records carrying that attribute and type,
combining spans, logs and datapoints. Expanded rows count records carrying each
exact value. Those values can overlap on a record, so their counts are not
necessarily additive.

The table has four columns: Name, Records, %, and Copy. Received types appear
beside attribute names, with provenance on hover. The Attributes header offers
the same Collapse all / Expand all controls as traces. These apply across
attribute pages, including values still loading. Individual disclosures remain
independent until the next bulk action. Refreshing or changing the discovery
scope resets the disclosures.

Name uses the shared table's responsive pinned-prefix layout; Copy stays at
the far right. Narrow panes fall back to native horizontal scrolling when the
pinned prefix would leave too little room for another column. Resized widths
are saved.

Clicking an attribute or value row shows its matching Telemetry. Native name
and value buttons remain keyboard-accessible; a focused row also supports
Enter/Space selection and Up/Down/Home/End navigation. Copy, disclosures and
text selection do not activate the row.

Attributes offers 25, 50 or 100 entries per page, with each entry's expanded values
kept together. Filtering and sorting apply to all matching entries, not only the
current page. Changing pages preserves the selected attribute and expansion state.

The right-hand Details pane contains matching records. Its single header row
offers shared Traces, Metrics and Logs tabs with known matching-record counts.
The redundant visible Telemetry label is omitted; the section retains its
accessible name. An inspected attribute keeps its actual field name and metadata.
Choices switch immediately; arrow, Home and End keys navigate available signals.
Each signal retains independent pages and scroll positions. Attributes and
Telemetry each remember their own 25/50/100 page-size preference, defaulting to 25.
Selecting a service, attribute key or exact value updates the matching records;
record cards still navigate to their native signal workspace.
