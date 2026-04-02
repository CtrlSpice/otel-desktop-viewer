---
name: TraceDetailPage Overhaul
overview: "Overhaul TraceDetailPage.svelte: functional cleanup, detail view redesign, time axis utility, waterfall (Honeycomb-style color rules; Rosé Pine Moon/Dawn palette), span selection, refinements, span search, layout polish, back-nav."
todos:
  - id: functional-cleanup
    content: Reorganize TraceDetailPage script into canonical sections, add fetch sequencing, clean up effects
    status: completed
  - id: detail-view-redesign
    content: "Redesign DetailView as a single-column property table: merged span/resource/scope fields with color-coded origin badges, collapsible events + links sections"
    status: completed
  - id: time-axis-util
    content: "Write pure niceTimeAxis utility (1-2-5 algorithm): given duration in ns + target tick count, returns unit, interval, and positioned/labeled ticks"
    status: completed
  - id: waterfall-view
    content: "Build waterfall view: header ruler (time axis), per-span duration bars, span hierarchy, Honeycomb-style bar colors (§4 Span bar coloring)"
    status: completed
  - id: span-selection
    content: Add selectedSpan on TraceDetailPage, wire waterfall row click + keyboard to DetailView (replace hardcoded first span)
    status: completed
  - id: waterfall-refinements
    content: "Waterfall refinements pass (see plan section): UX, edge cases, performance, accessibility — enumerate and implement in batches"
    status: completed
  - id: span-search
    content: "Wire up span search after waterfall: backend RPC, telemetry-service method, SearchEditor integration, handleSearchResults"
    status: completed
  - id: layout-polish
    content: Loading/error states, panel height, visual consistency with rest of app
    status: completed
  - id: back-nav-refinement
    content: Optionally improve history.back() to be app-aware
    status: completed
  - id: investigate-exceptions-summary
    content: Check why trace summaries may not surface exception counts properly (investigation only; no fixes yet)
    status: completed
isProject: false
---

# TraceDetailPage Overhaul

## Current state

`[TraceDetailPage.svelte](desktopexporter/internal/frontend/src/pages/TraceDetailPage.svelte)` is a minimal stub:

- Fetches a trace by ID via `telemetryAPI.getTraceByID(traceID)`
- Left panel: placeholder "Waterfall view coming soon..."
- Right panel: `DetailView` showing only `data.spans[0]`
- No search integration, no span selection, no cancellation/sequencing
- Route parsing and fetch are separate effects but not cleanly structured
- Empty `<style>` block

`[DetailView.svelte](desktopexporter/internal/frontend/src/components/TraceDetails/DetailView.svelte)` uses tab buttons (Fields / Events / Links) with a fixed `w-96` container. Sub-panels:

- `FieldsPanel` -- span attributes only
- `EventsPanel` -- span events
- `LinksPanel` -- span links
- `ResourcePanel`, `ScopePanel` -- exist but are unused (not imported anywhere)

Backend RPC methods available: `getTraceByID`, `searchTraces`, `deleteSpansByTraceID`, `deleteSpanByID`. There is **no** `searchTraceSpans` method yet.

## Plan

### 1. Functional cleanup of TraceDetailPage script

Reorganize `<script>` into canonical sections (imports, types, pure helpers, context, state, derived, effects, handlers, lifecycle) matching the pattern in `TracesPage` and `SearchEditor`.

- Add fetch sequencing (`loadSeq` pattern from `MetricsDetailPage`) so stale responses are discarded
- Merge the two effects (route parsing + fetch) into a single pipeline or make the dependency chain explicit
- Remove the empty `<style>` block
- Use `const` for `handleBack` (arrow, edge effect)

### 2. Redesign DetailView (right panel)

Replace the current tab-based layout with a **single-column property table** that merges all span data into one scrollable view.

**Structure:**

- Single-column table styled like the traces list (reuse `app.css` table classes)
- Each field rendered as: **compact header row** (field key + origin badge) / **data row** (value)
- Merge span attributes, resource attributes, and scope attributes into one list
- Color-coded origin badges on each field header:
  - Span fields/attributes -- no badge (default) or a subtle neutral one
  - Resource attributes -- e.g., a teal/cyan "resource" pill
  - Scope attributes -- e.g., a violet "scope" pill
- **Field ordering**: group by origin (core span fields first, then span attributes, then resource, then scope) with thin visual dividers between groups
- **Collapsible sections** at the bottom for Events (with count badge) and Links (with count badge), collapsed by default
- Remove the fixed `w-96` width so it flexes within `ResizablePanels`

**Files affected:**

- Rework `[DetailView.svelte](desktopexporter/internal/frontend/src/components/TraceDetails/DetailView.svelte)` -- new layout, remove tab buttons
- Rework `[FieldsPanel.svelte](desktopexporter/internal/frontend/src/components/TraceDetails/FieldsPanel.svelte)` -- or replace entirely; merge resource/scope fields in
- Keep `[EventsPanel.svelte](desktopexporter/internal/frontend/src/components/TraceDetails/EventsPanel.svelte)` and `[LinksPanel.svelte](desktopexporter/internal/frontend/src/components/TraceDetails/LinksPanel.svelte)` as collapsible sub-sections
- `[ResourcePanel.svelte](desktopexporter/internal/frontend/src/components/TraceDetails/ResourcePanel.svelte)` and `[ScopePanel.svelte](desktopexporter/internal/frontend/src/components/TraceDetails/ScopePanel.svelte)` can be removed (their data folds into the main view)
- Add badge styles to `[app.css](desktopexporter/internal/frontend/src/app.css)` (e.g., `.badge-origin-resource`, `.badge-origin-scope`)

### 3. Time axis utility (`niceTimeAxis`)

A pure, zero-dependency function that produces human-friendly tick marks for a time ruler. Lives in `[utils/time-axis.ts](desktopexporter/internal/frontend/src/utils/time-axis.ts)`.

**The legacy problem**: the old `DurationIndicator` divided the trace into equal slices, producing ugly labels like `154.321ms`, `308.642ms`. We want ticks at round values like `0`, `200ms`, `400ms`.

**Algorithm (1-2-5 sequence):**

1. **Pick the time unit** based on total duration (ns/us/ms/s/min)
2. **Compute a raw interval**: `duration / targetTickCount`
3. **Snap to the nearest "nice" step**: walk the 1-2-5 sequence (`1, 2, 5, 10, 20, 50, ...`) scaled to the chosen unit, pick the step closest to the raw interval
4. **Generate ticks**: start at 0, step by the nice interval, stop at or just past the duration
5. **Format labels**: consistent unit across all ticks (e.g., all in `ms`, never mixing `ms` and `s`)

**Signature:**

```typescript
type TimeAxisTick = { offsetPercent: number; label: string };
type TimeAxisResult = {
  unit: string;
  intervalNs: bigint;
  ticks: TimeAxisTick[];
};

function niceTimeAxis(
  durationNs: bigint,
  targetTickCount: number,
): TimeAxisResult;
```

**Consumers**: the waterfall header ruler and the per-span duration bars both use the tick array for positioning.

Also refactor the legacy `[formatDuration](desktopexporter/internal/frontend/src/utils/duration.ts)` (if it exists in the new codebase) or port it from the legacy code into `[utils/time.ts](desktopexporter/internal/frontend/src/utils/time.ts)` alongside `niceTimeAxis`.

### 4. Waterfall view (left panel) — first pass

Replace the placeholder with a full time-scaled waterfall. New components under `components/TraceDetails/`:

**Sub-components:**

- `WaterfallView.svelte` -- outer container, accepts spans + trace bounds (selection wiring comes in step 5)
- `WaterfallHeader.svelte` -- time ruler consuming `niceTimeAxis` ticks, span name / service name column headers
- `WaterfallRow.svelte` -- one row per span: indented name, service name, duration bar positioned against trace bounds

**Key concerns (baseline):**

- Span hierarchy (parent-child nesting via `parentSpanID`, indentation by depth)
- Duration bars: left offset + width as percentages of trace duration, with inline duration label (inside or beside bar depending on width)
- Event dots on the duration bar (like legacy, but optional / deferred)
- Handling rootless traces gracefully (missing parent placeholder rows)
- Virtualized rendering if span count is large (consider later; start without)

**Reuse from legacy** (ported to Svelte + Tailwind, not copy-pasted):

- `getTraceBounds(spans)` -- find min start / max end across all spans
- `getOffset(traceStart, traceEnd, point)` -- percentage offset for positioning
- Zero-width space trick for span name line-breaking

**Span bar coloring (Honeycomb-style, documented behavior):**

Honeycomb’s waterfall colors duration bars dynamically; we mirror the rules, not their exact palette.

- **Palette**: Use **Rosé Pine Moon** accents in dark theme and **Rosé Pine Dawn** accents in light theme (not Honeycomb’s).
  - **Reserve reds/pinks for errors**: do **not** use `love` (and optionally `rose`) for categorical span coloring; keep them for error/exception emphasis.
  - Candidate set (categorical tokens): `gold`, `pine`, `foam`, `iris` (optionally add `rose` only if you want a warmer non-error accent).
  - neutrals for lines/labels: `muted`, `subtle`, `text`
- **Theme mapping**: keep the same token order + stable hash → index. Only the underlying hex changes per theme (Moon vs Dawn).
- **Mapping rule**: stable hash of the chosen field value → index into the accent list above (so the same `service.name` always gets the same color).
- **High-cardinality fallback (gel pen mode)**: if distinct values exceed 6, keep the same 4 hues but add a second, subtle channel so duplicates stay distinguishable.
  - **Naming**: in code, this is officially **gelPenMode** (feature flag / mode name / helper naming).
  - option A: alternate **lightness / tone** (two or three preset mix-with-base levels) per repeated hue
  - option B: alternate **bar style** (ring/outline strength or faint pattern) per repeated hue
  - ensure label contrast stays readable (dark text on light bars, light text on dark bars)
- **Default grouping field**: If the trace has **more than one distinct `service.name`** (from resource attributes), color bars by `**service.name**`. If only one service (or missing), color by **span `name`** instead.
- **User override**: Later, allow **“color rows by values in this field”** (equivalent to Honeycomb’s column-header menu) — pick any displayed attribute; hash stable string → hue in a fixed saturation/lightness ramp so the same value always maps to the same color.
- **Errors**: Spans with error status (and optionally exception events on the bar) use **error/red** styling; can stack with or override the categorical color (Honeycomb emphasizes red for errors).
- **Selection**: Selected span row / bar gets **primary/blue** highlight (Honeycomb: blue) on top of base color or as ring/outline.
- **Event dots**: Small markers on the bar; exception/error-related events can use **red** like Honeycomb.

**Error definition (for red styling):**

- A span is considered an error span if `**statusCode === "ERROR"` OR it has one or more exception events.

Implementation sketch: pure helper `spanBarColorKey(span, traceSpans, colorByField?: string)` → discriminated union `{ kind: 'error' } | { kind: 'categorical'; key: string }` then map to CSS variables or Tailwind-safe classes in the component.

### 5. Span selection state

Add `selectedSpan` (or `selectedSpanID`) on `TraceDetailPage`, pass callbacks / bind into `WaterfallView`, and drive `DetailView` from the selection. Until this step, the right panel can stay on first span or a placeholder. Include row click, optional keyboard navigation (arrow up/down), and scroll-into-view when selection changes.

### 6. Waterfall refinements (iterative)

Dedicated pass **after** waterfall first pass and span selection wiring, **before** span search. You have a long list of refinements to add here; we will break them into small batches during implementation.

**Buckets to fill in as you enumerate:**

- **Ruler + scale**: tick density vs width, label collision, RTL, very short / very long traces
- **Rows**: column widths, truncation, hover/focus, selection styling, striped rows vs list parity
- **Bars**: min width, zero-duration spans, spans outside trace bounds, overlapping labels
- **Hierarchy**: sort order, orphan handling, expand/collapse subtrees (if desired)
- **Interaction**: keyboard (full matrix), scroll-into-view on select, copy UX
- **Performance**: virtualization, memoization of layout, resize observers
- **A11y**: roles, labels, live regions for selection changes
- **Color-by-field UX**: column header menu, persistence (optional), contrast checks for bar + label

_(Replace or extend this list with your specific items when you are ready.)_

**Refinements captured so far (incremental checklist):**

- **Tree gutter connectors**: add subtle vertical guide lines per ancestor depth + short horizontal connector into the current span row (VS Code-style tree guides; no ASCII).
- **Child-count badge**: render a small badge near the node showing direct children count (Honeycomb-ish dependency count, but visually cleaner).
- **Tree-connector correctness metadata**: compute `childrenCount` and `ancestorHasNextSibling[]` per row so vertical guides continue/terminate correctly.
  - **Decision**: spans are **preordered** and each row already has `depth` set, so we can derive connector metadata in a single linear pass (no explicit tree rebuild).
  - **Algorithm sketch** (pure helper, O(n), stack-based):
    - Input: flat ordered spans `[ { spanData, depth }, ... ]`
    - Output: enriched rows with `{ childrenCount, ancestorHasNextSibling: boolean[] }`
    - Maintain a stack of open ancestors by depth. For each row:
      - Pop stack until `stack.length === depth`
      - If `depth > 0`, increment `childrenCount` of `stack[depth-1]` (direct parent)
      - Compute `ancestorHasNextSibling` from stack state (which depth columns should keep drawing a vertical guide)
      - Update branch termination by peeking at the next row’s depth (<= current depth closes branches)

### 7. Wire up span search from SignalHeader (after waterfall + refinements)

`SearchEditor` already has a `{ signal: 'traces', view: 'detail', traceID }` variant and `SearchResultEvent` already defines `{ signal: 'traces'; view: 'detail'; results: SpanData[] }`.

Two sub-steps:

- **Backend**: add a `searchTraceSpans` RPC method in `[jsonrpc_handler.go](desktopexporter/internal/server/jsonrpc_handler.go)` (accepts traceID + query filters, returns filtered spans)
- **Frontend**: add `searchTraceSpans` to `[telemetry-service.ts](desktopexporter/internal/frontend/src/services/telemetry-service.ts)`, wire `buildSearchFn` in `SearchEditor` for the `traces/detail` case, and add `handleSearchResults` to `TraceDetailPage`

**Why after waterfall**: the waterfall and ruler behavior should stabilize first; search can then filter/highlight rows and drive selection without fighting ongoing layout changes.

### 8. Layout and UI polish

- Loading/error states consistent with `TracesPage` (rounded cards, spinner)
- Panel height: currently `h-[calc(100vh-12rem)]` which may not account for the new header layout
- Visual consistency across detail and list views

### 9. "Back within app" navigation refinement

Current `history.back()` with fallback is fine for now. Future improvement: track whether the previous history entry is within the app (stash an app marker in `nav-state`) to avoid backing out to an external site.

## Decisions made

- **Field ordering**: group by origin (span, resource, scope) with visual dividers
- **Waterfall**: full time-scaled waterfall (not simplified indented list)
- **Backend search**: Go RPC method (`searchTraceSpans`), integrated **after** waterfall + refinements
- **Time axis**: hand-rolled 1-2-5 algorithm, no external deps
- **Execution order**: waterfall first pass, then span selection wiring, then waterfall refinements, then search
- **Span colors**: Honeycomb-style defaults (`service.name` vs span `name` by distinct service count), error red, selected blue, optional user-chosen field + stable hash → hue; see §4 **Span bar coloring**

## Open questions

- **Waterfall refinements**: paste or enumerate your full list so we can turn section 6 into a checklist
- **Badge colors**: any specific palette preferences for resource/scope origin badges?
- **Anything else** to add to this list?

## Investigation notes (do not implement yet)

- **Trace summaries exceptions**: check why exception counts aren’t surfacing properly in trace list summaries / stats. Scope: investigation only (confirm where exception counts are derived, and whether exceptions are being recorded/queried/aggregated consistently).
