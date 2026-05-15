<script lang="ts">
  /*
   * TimeseriesPanel: the bottom half of the metrics page main split.
   * One row per timeseries belonging to the currently-selected metric;
   * each row carries a color swatch (matches the chart line), a
   * checkbox for show/hide on the chart (replaces the old side
   * TimeseriesLegend's responsibility), the timeseries' attribute set,
   * a datapoint count badge, and a chevron column reserved for the
   * step-3 expand-to-datapoints behaviour (rendered but inert here).
   *
   * Reads everything through MetricViewContext: the panel is a pure
   * renderer of the context's per-metric state and a thin invoker of
   * its visibility-set mutations. Both Gauge/Sum and Histogram metrics
   * are supported by branching internally on ctx.isHistogramKind so
   * the panel itself stays a single component.
   *
   * Visibility-set rules (delegated to the context's SvelteSet writes):
   *   - Min 1: when only one row is checked, that row's checkbox is
   *     locked so the chart can't go fully blank. Communicated per-row
   *     via tooltip rather than a banner -- one-row situation, banner
   *     would be louder than warranted.
   *   - Max MAX_VISIBLE_TIMESERIES (10): when the cap is reached every
   *     unchecked row's checkbox is disabled so the palette never
   *     wraps. Checked rows stay enabled so the user can free a slot.
   */
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import type { Timeseries as PanelTimeseries } from '@/components/MetricCharts/TimeseriesLegend.svelte'
  import type { DataPoint, MetricTimeseries } from '@/types/api-types'
  import {
    MAX_VISIBLE_TIMESERIES,
    timeseriesColor,
    timeseriesForegroundColor,
  } from '@/utils/timeseries-palette'
  import { formatTimestamp } from '@/utils/time'
  import { ArrowDownIcon } from '@/icons'
  import type { SvelteSet } from 'svelte/reactivity'

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  // Per-key lookup into the metric's full Timeseries[] so a row can
  // render the underlying datapoints when expanded. Built from the
  // metric directly (not a context derivation) because no other
  // consumer needs it; cheap to recompute on metric change.
  let timeseriesByKey = $derived.by((): Map<string, MetricTimeseries> => {
    const map = new Map<string, MetricTimeseries>()
    const m = ctx.metric
    if (!m) return map
    for (const ts of m.timeseries) {
      map.set(ts.attributesKey, ts)
    }
    return map
  })

  /* Mirrors MetricDetailView's datapointValue: the at-a-glance scalar
     for the row. Histogram/ExpHistogram show count+sum (the per-
     bucket distribution lives in the chart's snapshot view, not in
     this inline table). */
  function datapointValue(dp: DataPoint): string {
    if (dp.metricType === 'Gauge' || dp.metricType === 'Sum') {
      return String(dp.doubleValue ?? dp.intValue ?? '—')
    }
    if (
      dp.metricType === 'Histogram' ||
      dp.metricType === 'ExponentialHistogram'
    ) {
      return `count: ${dp.count}, sum: ${dp.sum.toFixed(2)}`
    }
    return '—'
  }

  // Pick the right context state per metric kind. Both arms expose a
  // PanelTimeseries[]-shaped list and a SvelteSet<string> of visible
  // keys, so the rendering loop downstream is identical.
  let rows = $derived<PanelTimeseries[]>(
    ctx.isHistogramKind
      ? ctx.histogramLegendTimeseries
      : ctx.gaugeSumLegendTimeseries
  )
  let visibleKeys = $derived<SvelteSet<string>>(
    ctx.isHistogramKind ? ctx.histogramVisible : ctx.gaugeSumVisible
  )

  // Cap + floor predicates for the visibility rules above. Cheap;
  // re-derived on every visibleKeys mutation so the disabled state
  // stays in lockstep with row checks.
  let capReached = $derived(visibleKeys.size >= MAX_VISIBLE_TIMESERIES)
  let isLastChecked = $derived(visibleKeys.size === 1)

  function toggle(key: string, checked: boolean) {
    if (checked) {
      visibleKeys.add(key)
    } else {
      visibleKeys.delete(key)
    }
  }

  function attrsTooltip(attrs: PanelTimeseries['attributes']): string {
    if (attrs.length === 0) return 'default timeseries'
    return attrs.map((a) => `${a.key}=${a.value}`).join(', ')
  }

  // -- Histogram sync (step 4) -------------------------------------
  // When the user clicks a snapshot on the histogram heatmap, the
  // context sets selectedDatapointId AND expands the owning
  // timeseries (see onHeatmapSelect). This panel watches that
  // selection and scrolls the matching datapoint row into view +
  // briefly highlights it so the user can see what they picked.
  // The highlight is transient (~1.4s) -- a permanent selected
  // class would compete with the row's own hover/expanded states.
  let listEl = $state<HTMLUListElement | null>(null)
  let highlightedDpId = $state<string | null>(null)
  let highlightTimer: ReturnType<typeof setTimeout> | null = null

  $effect(() => {
    const id = ctx.selectedDatapointId
    if (!id || !listEl) return
    // Defer to the microtask queue so the row's expansion (added by
    // onHeatmapSelect via expandedTimeseries) is in the DOM by the
    // time we querySelect for the dp <tr>. Without this the inline
    // table doesn't exist yet on the same tick.
    queueMicrotask(() => {
      const row = listEl?.querySelector<HTMLElement>(
        `[data-dp-id="${CSS.escape(id)}"]`
      )
      if (!row) return
      row.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
      highlightedDpId = id
      if (highlightTimer) clearTimeout(highlightTimer)
      highlightTimer = setTimeout(() => {
        highlightedDpId = null
        highlightTimer = null
      }, 1400)
    })
  })

  $effect(() => {
    return () => {
      if (highlightTimer) clearTimeout(highlightTimer)
    }
  })
</script>

<div class="timeseries-panel" role="region" aria-label="Timeseries">
  <div class="timeseries-panel__header">
    <span class="timeseries-panel__title">Timeseries</span>
    <span
      class="timeseries-panel__count"
      class:timeseries-panel__count--cap={capReached}
    >
      {visibleKeys.size} / {Math.min(rows.length, MAX_VISIBLE_TIMESERIES)} visible
    </span>
  </div>

  <ul class="timeseries-panel__list" role="list" bind:this={listEl}>
    {#each rows as ts, i (ts.key)}
      {@const checked = visibleKeys.has(ts.key)}
      {@const disabledByCap = !checked && capReached}
      {@const disabledByFloor = checked && isLastChecked}
      {@const disabled = disabledByCap || disabledByFloor}
      {@const colorIdx = ctx.timeseriesColorIndex.get(ts.key) ?? i}
      {@const color = timeseriesColor(colorIdx)}
      {@const fg = timeseriesForegroundColor(colorIdx)}
      {@const tooltip = disabledByFloor
        ? 'At least one timeseries must remain selected'
        : attrsTooltip(ts.attributes)}
      {@const expanded = ctx.expandedTimeseries.has(ts.key)}
      {@const tsRecord = timeseriesByKey.get(ts.key)}
      {@const datapoints = tsRecord?.datapoints ?? []}
      <li
        class="timeseries-panel__row"
        class:timeseries-panel__row--disabled={disabled}
        class:timeseries-panel__row--expanded={expanded}
      >
        <label class="timeseries-panel__cell timeseries-panel__cell--check" title={tooltip}>
          <input
            type="checkbox"
            class="checkbox checkbox-xs timeseries-panel__checkbox"
            style:--input-color={color}
            style:color={fg}
            {checked}
            {disabled}
            onchange={(e) =>
              toggle(ts.key, (e.currentTarget as HTMLInputElement).checked)}
          />
        </label>

        <span
          class="timeseries-panel__swatch"
          style:background-color={color}
          aria-hidden="true"
        ></span>

        <span class="timeseries-panel__attrs" title={tooltip}>
          {#if ts.attributes.length === 0}
            <span class="timeseries-panel__attrs-empty">default timeseries</span>
          {:else}
            {#each ts.attributes as attr (attr.key)}
              <span class="timeseries-panel__attr">
                <span class="timeseries-panel__attr-key">{attr.key}</span>
                <span class="timeseries-panel__attr-eq">=</span>
                <span class="timeseries-panel__attr-value">{attr.value}</span>
              </span>
            {/each}
          {/if}
        </span>

        {#if ts.badge}
          <span class="timeseries-panel__badge">{ts.badge}</span>
        {/if}

        <!-- Chevron toggles expansion to an inline datapoints table
             below the row. The arrow rotates 180° when open via the
             --open modifier; a 200ms transform transition keeps the
             rotation perceptibly tied to the click. -->
        <button
          type="button"
          class="timeseries-panel__chevron"
          class:timeseries-panel__chevron--open={expanded}
          aria-label={expanded ? 'Collapse datapoints' : 'Expand datapoints'}
          aria-expanded={expanded}
          onclick={() => ctx.toggleTimeseriesExpanded(ts.key)}
          disabled={datapoints.length === 0}
        >
          <ArrowDownIcon class="h-3.5 w-3.5" aria-hidden="true" />
        </button>

        {#if expanded && datapoints.length > 0}
          <!-- Inline datapoints sub-row. Spans the full row width via
               grid-column on the table wrapper (the row is a flex row;
               this child wraps below by being a flex item with full
               basis and flex-wrap on the parent). -->
          <div class="timeseries-panel__expansion">
            <table class="timeseries-panel__dp-table" aria-label="Datapoints for this timeseries">
              <thead>
                <tr>
                  <th class="timeseries-panel__dp-th timeseries-panel__dp-th--time">time</th>
                  <th class="timeseries-panel__dp-th timeseries-panel__dp-th--value">value</th>
                  <th class="timeseries-panel__dp-th timeseries-panel__dp-th--meta">flags</th>
                </tr>
              </thead>
              <tbody>
                {#each datapoints as dp (dp.id)}
                  <tr
                    class="timeseries-panel__dp-row"
                    class:timeseries-panel__dp-row--highlight={highlightedDpId === dp.id}
                    data-dp-id={dp.id}
                  >
                    <td class="timeseries-panel__dp-td timeseries-panel__dp-td--time">
                      <span class="tabular-nums">
                        {formatTimestamp(
                          dp.timestamp,
                          timeContext.timezone,
                          'milliseconds'
                        )}
                      </span>
                    </td>
                    <td class="timeseries-panel__dp-td timeseries-panel__dp-td--value">
                      {datapointValue(dp)}
                    </td>
                    <td class="timeseries-panel__dp-td timeseries-panel__dp-td--meta">
                      {#if dp.exemplars.length > 0}
                        <span
                          class="badge badge-xs badge-soft badge-neutral"
                          title="{dp.exemplars.length} exemplar{dp.exemplars.length === 1 ? '' : 's'}"
                        >
                          {dp.exemplars.length} ex
                        </span>
                      {/if}
                      {#if dp.flags > 0}
                        <span
                          class="badge badge-xs badge-soft badge-warning ml-1"
                          title="data point flags = {dp.flags}"
                        >
                          flags: {dp.flags}
                        </span>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </li>
    {/each}
  </ul>

  {#if capReached}
    <p class="timeseries-panel__cap-note">
      Cap of {MAX_VISIBLE_TIMESERIES} timeseries reached. Uncheck one to enable
      another.
    </p>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .timeseries-panel {
    @apply flex h-full min-h-0 min-w-0 flex-col gap-2 overflow-hidden p-2;
  }

  .timeseries-panel__header {
    @apply flex shrink-0 items-baseline justify-between px-1;
  }

  .timeseries-panel__title {
    @apply text-xs font-semibold uppercase tracking-wide text-base-content/60;
  }

  .timeseries-panel__count {
    @apply text-xs tabular-nums text-base-content/60;
  }

  .timeseries-panel__count--cap {
    @apply font-semibold text-warning;
  }

  /* Scrollable list region; the panel itself is bounded by the parent
     ResizablePanels. min-h-0 + overflow-y-auto = the rows scroll
     internally without forcing the panel to grow. */
  .timeseries-panel__list {
    @apply m-0 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-0;
    list-style: none;
  }

  /* Row layout: [checkbox][swatch][attrs grow][badge][chevron]
     swatch is a thin colored bar so the row's identity is glanceable
     even when the checkbox is unchecked (chart-line color is what the
     user already mentally associates this timeseries with). */
  /* flex-wrap so the expansion sub-row (flex-basis: 100%) can break
     onto a new line below the row's main controls. items-center on
     the row would force the expansion vertically-center within the
     row's height; using items-start keeps the controls top-aligned
     within their first wrap line and lets the expansion sit flush
     under them. */
  .timeseries-panel__row {
    @apply flex flex-wrap items-center gap-x-2 gap-y-0 rounded px-1 py-1 hover:bg-base-200/50;
    min-height: 1.75rem;
  }

  .timeseries-panel__row--expanded {
    @apply bg-base-200/30 hover:bg-base-200/40;
  }

  .timeseries-panel__row--disabled {
    @apply opacity-60;
  }

  .timeseries-panel__row--disabled:hover {
    @apply bg-transparent;
  }

  .timeseries-panel__cell--check {
    @apply flex shrink-0 cursor-pointer items-center;
  }

  .timeseries-panel__row--disabled .timeseries-panel__cell--check {
    @apply cursor-not-allowed;
  }

  .timeseries-panel__checkbox {
    @apply shrink-0 cursor-pointer;
  }

  .timeseries-panel__row--disabled .timeseries-panel__checkbox {
    @apply cursor-not-allowed;
  }

  /* Color swatch: 3px-wide vertical chip. Wider than a dot, narrower
     than a chip; reads as "color identity" without claiming row real
     estate the attrs need. */
  .timeseries-panel__swatch {
    @apply shrink-0 rounded-sm;
    width: 3px;
    align-self: stretch;
    margin-block: 2px;
  }

  /* Attrs claim the row's flex space and truncate on overflow. The
     <label> tooltip on the parent label already surfaces the full
     attribute string on hover. */
  .timeseries-panel__attrs {
    @apply flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2 gap-y-0.5 overflow-hidden font-mono text-xs;
  }

  .timeseries-panel__attrs-empty {
    @apply italic text-base-content/50;
  }

  .timeseries-panel__attr {
    @apply inline-flex min-w-0 max-w-full items-baseline;
  }

  .timeseries-panel__attr-key {
    @apply shrink-0 text-base-content/70;
  }

  .timeseries-panel__attr-eq {
    @apply mx-0.5 shrink-0 text-base-content/40;
  }

  .timeseries-panel__attr-value {
    @apply truncate text-base-content;
  }

  .timeseries-panel__badge {
    @apply shrink-0 text-[0.65rem] tabular-nums text-base-content/50;
  }

  /* Chevron toggle. The transform transition keeps the rotation
     perceptibly tied to click; disabled state (timeseries with no
     datapoints) drops opacity and forbids hover wash. */
  .timeseries-panel__chevron {
    @apply inline-flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-base-content/55 transition-[background-color,color,transform] duration-200 hover:bg-base-200 hover:text-base-content;
  }

  .timeseries-panel__chevron--open {
    @apply rotate-180;
  }

  .timeseries-panel__chevron:disabled {
    @apply cursor-not-allowed opacity-40;
  }

  .timeseries-panel__chevron:disabled:hover {
    @apply bg-transparent text-base-content/55;
  }

  /* Expansion sub-row: full-width below the row controls via
     flex-basis 100% on a flex-wrap parent. The inner padding gives
     the table breathing room from the row's edges and from the
     swatch + checkbox column to the left. */
  .timeseries-panel__expansion {
    @apply mt-1 w-full pl-7 pr-2 pb-1;
    flex-basis: 100%;
  }

  /* Inline datapoints table -- 3 columns (time, value, flags). Tight
     row height so a long timeseries doesn't force the bottom panel
     to scroll for one expanded row. tabular-nums on time so they
     left-align under each other. */
  .timeseries-panel__dp-table {
    @apply w-full text-xs;
    border-collapse: collapse;
  }

  .timeseries-panel__dp-th {
    @apply px-2 py-1 text-left text-[0.65rem] font-semibold uppercase tracking-wide text-base-content/55;
    border-bottom: 1px solid
      color-mix(in oklab, var(--color-base-300) 25%, transparent);
  }

  .timeseries-panel__dp-th--time {
    width: 14rem;
  }

  .timeseries-panel__dp-th--meta {
    width: 8rem;
  }

  .timeseries-panel__dp-row {
    @apply transition-colors hover:bg-base-200/40;
  }

  /* Transient highlight for the row that was just selected via the
     histogram heatmap. The class is removed by a timer (~1.4s) so
     the highlight feels like a brief "look here" pulse rather than
     a sticky selected state. */
  .timeseries-panel__dp-row--highlight {
    background-color: color-mix(
      in oklab,
      var(--color-primary) 22%,
      transparent
    );
  }

  .timeseries-panel__dp-td {
    @apply px-2 py-0.5 align-middle;
  }

  .timeseries-panel__dp-td--time {
    @apply text-base-content/80;
  }

  .timeseries-panel__dp-td--value {
    @apply font-mono text-base-content;
  }

  .timeseries-panel__dp-td--meta {
    @apply text-right;
  }

  .timeseries-panel__cap-note {
    @apply m-0 px-1 text-xs text-warning/80;
  }
</style>
