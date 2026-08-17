<script lang="ts">
  /*
   * Inline datapoint rows for a single timeseries. Nested under an
   * expanded series row in TimeseriesPanel. Selection, exemplar
   * expansion, and histogram snapshot sync go through MetricViewContext.
   *
   * Long lists are windowed rather than truncated. This is the view that
   * answers "what did my service actually send", so every datapoint has to be
   * reachable -- a cap with a "showing 500 of 17,076" note would be a smaller
   * lie than merged rows, but still a lie. Rendering a row per datapoint is not
   * an option either: a dense stream is hundreds of thousands of rows and the
   * DOM will not take it.
   *
   * So the rows exist in the array and only the visible slice exists in the
   * DOM, with spacer rows standing in for the rest. Heights are measured rather
   * than assumed, because an expanded exemplar row is several times the height
   * of a plain one; unmeasured rows use an estimate and correct themselves as
   * they scroll into view.
   */
  import { SvelteMap } from 'svelte/reactivity'
  import { formatDateTimeMs } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { formatMetricValuePlain } from '@/components/metrics/utils/format-metric-value'
  import { dedupeAttributes } from '@/components/metrics/utils/dedupe-attributes'
  import { itemHref, navigateToItem } from '@/route'
  import { SPAN_PARAM } from '@/route/query-params'
  import type { DataPoint, Exemplar } from '@/types/api-types'

  type Props = {
    datapoints: DataPoint[]
    /** Show a color swatch column (flat cross-series lists). */
    showSwatch?: boolean
    seriesColor?: string
    /** Drop horizontal inset (nested under series rows). */
    flush?: boolean
  }

  let { datapoints, showSwatch = false, seriesColor, flush = false }: Props =
    $props()

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  let metricUnit = $derived(ctx.metric?.unit ?? '')

  // Below this a list renders whole: no scroller, no spacers, no measurement.
  // Windowing a short list costs more than it saves and would put a second
  // scroll region inside the detail pane for twenty rows.
  const VIRTUALIZE_ABOVE = 200
  // Rows above and below the viewport, so a fast scroll does not show gaps
  // before the next frame fills them.
  const OVERSCAN = 10
  // Only used for rows that have never been on screen. Close to a plain row's
  // real height, so the scrollbar barely shifts as measurements replace it.
  const ESTIMATED_ROW_PX = 24

  let virtualize = $derived(datapoints.length > VIRTUALIZE_ABOVE)

  let scrollTop = $state(0)
  let viewportPx = $state(0)

  // Measured heights, keyed by datapoint id rather than index: the array
  // changes identity on every poll, and an id survives that.
  const rowPx = new SvelteMap<string, number>()
  const expansionPx = new SvelteMap<string, number>()

  function heightOf(dp: DataPoint): number {
    const base = rowPx.get(dp.id) ?? ESTIMATED_ROW_PX
    if (!ctx.expandedDatapoints.has(dp.id)) return base
    return base + (expansionPx.get(dp.id) ?? 0)
  }

  // Cumulative offsets, one longer than the list: offsets[i] is where row i
  // starts and offsets[n] is the total height. Recomputed when the list or a
  // measurement changes -- not on scroll, which only searches it.
  let offsets = $derived.by((): number[] => {
    if (!virtualize) return []
    const out = new Array<number>(datapoints.length + 1)
    out[0] = 0
    for (let i = 0; i < datapoints.length; i++) {
      out[i + 1] = out[i]! + heightOf(datapoints[i]!)
    }
    return out
  })

  /** First index whose row ends after `px`. */
  function indexAt(px: number): number {
    let lo = 0
    let hi = datapoints.length - 1
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (offsets[mid + 1]! <= px) lo = mid + 1
      else hi = mid
    }
    return lo
  }

  let range = $derived.by((): { start: number; end: number } => {
    if (!virtualize) return { start: 0, end: datapoints.length }
    // Before layout has measured the viewport, render a screenful rather than
    // nothing -- otherwise the list is empty until the first scroll event.
    const height = viewportPx || 400
    const start = Math.max(0, indexAt(scrollTop) - OVERSCAN)
    const end = Math.min(datapoints.length, indexAt(scrollTop + height) + 1 + OVERSCAN)
    return { start, end }
  })

  let visible = $derived(datapoints.slice(range.start, range.end))
  let padTopPx = $derived(virtualize ? (offsets[range.start] ?? 0) : 0)
  let padBottomPx = $derived(
    virtualize
      ? (offsets[datapoints.length] ?? 0) - (offsets[range.end] ?? 0)
      : 0
  )

  let columnCount = $derived(showSwatch ? 2 : 1)

  /** Report a rendered element's height, ignoring no-op changes so the
   *  ResizeObserver cannot drive a render loop. */
  function measure(
    node: HTMLElement,
    args: { id: string; into: SvelteMap<string, number> }
  ) {
    let current = args
    const report = () => {
      const px = node.getBoundingClientRect().height
      if (px <= 0) return
      const prev = current.into.get(current.id)
      if (prev !== undefined && Math.abs(prev - px) < 0.5) return
      current.into.set(current.id, px)
    }
    const observer = new ResizeObserver(report)
    observer.observe(node)
    report()
    return {
      update(next: { id: string; into: SvelteMap<string, number> }) {
        current = next
        report()
      },
      destroy() {
        observer.disconnect()
      },
    }
  }

  function displayUnit(unit: string): string | null {
    const u = unit.trim()
    if (!u || u === '1') return null
    return u
  }

  function formatDatapointTime(timestamp: bigint): string {
    return formatDateTimeMs(
      Number(timestamp / 1_000_000n),
      timeContext.tz
    ).dateTime
  }

  function exemplarSpanPatch(ex: Exemplar) {
    return ex.spanID ? { [SPAN_PARAM]: ex.spanID } : undefined
  }

  function goToExemplarTrace(e: MouseEvent, ex: Exemplar) {
    if (!ex.traceID) return
    e.preventDefault()
    navigateToItem('traces', ex.traceID, 'push', exemplarSpanPatch(ex))
  }

  function datapointValueParts(
    dp: DataPoint
  ): { number: string; unit: string | null } {
    const unit = displayUnit(metricUnit)
    if (dp.metricType === 'Gauge' || dp.metricType === 'Sum') {
      const raw = dp.doubleValue ?? dp.intValue
      if (raw === undefined || raw === null) {
        return { number: '—', unit: null }
      }
      return {
        number: formatMetricValuePlain(Number(raw)),
        unit,
      }
    }
    if (
      dp.metricType === 'Histogram' ||
      dp.metricType === 'ExponentialHistogram'
    ) {
      return {
        number: `count ${dp.count}, sum ${formatMetricValuePlain(dp.sum)}`,
        unit,
      }
    }
    return { number: '—', unit: null }
  }
</script>

{#snippet datapointRows(dp: DataPoint)}
  {@const selected = ctx.selectedDatapointId === dp.id}
  {@const hasExtra = dp.flags > 0 || dp.exemplars.length > 0}
  {@const expanded = hasExtra && ctx.expandedDatapoints.has(dp.id)}
  {@const valueParts = datapointValueParts(dp)}
  <tr
    class="dp-list__row"
    class:dp-list__row--selected={selected}
    class:dp-list__row--expandable={hasExtra}
    data-dp-id={dp.id}
    onclick={() => ctx.onDatapointClick(dp)}
    use:measure={{ id: dp.id, into: rowPx }}
  >
    {#if showSwatch}
      <td class="dp-list__td dp-list__td--swatch">
        <span
          class="dp-list__swatch"
          style:background-color={seriesColor}
          aria-hidden="true"
        ></span>
      </td>
    {/if}
    <td
      class="dp-list__td dp-list__td--content"
      colspan={showSwatch ? undefined : 1}
    >
      <div class="dp-list__row-main">
        <span class="dp-list__time tabular-nums"
          >{formatDatapointTime(dp.timestamp)}</span
        >
        <div class="dp-list__trail">
          <span class="dp-list__value-group">
            <span class="dp-list__value tabular-nums">{valueParts.number}</span>
            {#if valueParts.unit}
              <span class="dp-list__unit">{valueParts.unit}</span>
            {/if}
          </span>
          {#if hasExtra}
            {#if dp.exemplars.length > 0}
              <span class="badge-count">
                {#if dp.exemplarCount > dp.exemplars.length}
                  {dp.exemplars.length} of {dp.exemplarCount} ex
                {:else}
                  {dp.exemplars.length} ex
                {/if}
              </span>
            {/if}
            {#if dp.flags > 0}
              <span class="badge badge-xs badge-soft badge-warning">flags</span>
            {/if}
          {/if}
        </div>
      </div>
    </td>
  </tr>
  {#if expanded}
    <tr
      class="dp-list__expansion-row"
      use:measure={{ id: dp.id, into: expansionPx }}
    >
      <td colspan={showSwatch ? 2 : 1} class="dp-list__expansion-cell">
        <div class="dp-list__expansion">
          {#if dp.flags > 0}
            <div class="dp-list__detail">
              <span class="dp-list__detail-label">flags</span>
              <span class="dp-list__detail-value">{dp.flags}</span>
            </div>
          {/if}
          {#if dp.exemplarCount > dp.exemplars.length}
            <div class="dp-list__detail">
              <span class="dp-list__detail-label">exemplars</span>
              <span class="dp-list__detail-value">
                showing {dp.exemplars.length} of {dp.exemplarCount} — the store
                caps the list so one densely sampled stream cannot decide the
                size of every response
              </span>
            </div>
          {/if}
          {#each dp.exemplars as ex, i}
            <div class="dp-list__exemplar">
              <span class="dp-list__detail-label">exemplar {i + 1}</span>
              <div class="dp-list__exemplar-fields">
                <span class="dp-list__detail-value">value: {ex.value}</span>
                <span class="dp-list__detail-value tabular-nums">
                  time: {formatDatapointTime(ex.timestamp)}
                </span>
                {#if ex.traceID}
                  {@const patch = exemplarSpanPatch(ex)}
                  <a
                    class="dp-list__detail-value link link-primary font-mono"
                    href={itemHref('traces', ex.traceID, patch)}
                    onclick={e => goToExemplarTrace(e, ex)}
                  >trace: {ex.traceID}</a>
                {/if}
                {#if ex.spanID && ex.traceID}
                  <a
                    class="dp-list__detail-value link link-primary font-mono"
                    href={itemHref('traces', ex.traceID, exemplarSpanPatch(ex))}
                    onclick={e => goToExemplarTrace(e, ex)}
                  >span: {ex.spanID}</a>
                {:else if ex.spanID}
                  <span class="dp-list__detail-value">span: {ex.spanID}</span>
                {/if}
                {#each dedupeAttributes(ex.filteredAttributes) as attr (attr.key)}
                  <span class="dp-list__detail-value">{attr.key}: {attr.value}</span>
                {/each}
              </div>
            </div>
          {/each}
        </div>
      </td>
    </tr>
  {/if}
{/snippet}

{#if datapoints.length === 0}
  <p class="dp-list__empty" class:dp-list__empty--flush={flush}>No datapoints</p>
{:else if !virtualize}
  <table class="dp-list" class:dp-list--flush={flush} aria-label="Datapoints">
    <tbody>
      {#each datapoints as dp (dp.id)}
        {@render datapointRows(dp)}
      {/each}
    </tbody>
  </table>
{:else}
  <div
    class="dp-list__scroller"
    bind:clientHeight={viewportPx}
    onscroll={e => (scrollTop = e.currentTarget.scrollTop)}
  >
    <table
      class="dp-list"
      class:dp-list--flush={flush}
      aria-label="Datapoints"
      aria-rowcount={datapoints.length}
    >
      <tbody>
        <!-- Spacers stand in for the rows that are not in the DOM, so the
             scrollbar reflects the whole list rather than the window. -->
        {#if padTopPx > 0}
          <tr aria-hidden="true" style:height="{padTopPx}px">
            <td colspan={columnCount}></td>
          </tr>
        {/if}
        {#each visible as dp (dp.id)}
          {@render datapointRows(dp)}
        {/each}
        {#if padBottomPx > 0}
          <tr aria-hidden="true" style:height="{padBottomPx}px">
            <td colspan={columnCount}></td>
          </tr>
        {/if}
      </tbody>
    </table>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .dp-list__empty--flush {
    @apply px-0;
  }

  .dp-list--flush .dp-list__td--swatch {
    @apply pl-0;
  }

  .dp-list--flush .dp-list__td--content {
    @apply pl-0 pr-2;
  }

  .dp-list__empty {
    @apply m-0 px-3 py-2 text-center text-xs italic;
    color: var(--color-muted);
  }

  /* Its own scroll region, so a long series does not push the rest of the
     detail pane off screen. Only applied when the list is windowed. */
  .dp-list__scroller {
    @apply overflow-y-auto;
    max-height: 24rem;
    overscroll-behavior: contain;
  }

  .dp-list {
    @apply w-full text-xs;
    border-collapse: collapse;
  }

  .dp-list__row {
    @apply cursor-pointer transition-colors hover:bg-base-300/30;
  }

  .dp-list__row--selected {
    background-color: color-mix(
      in oklab,
      var(--color-primary) 18%,
      transparent
    );
  }

  .dp-list__td {
    @apply py-1 align-middle;
  }

  .dp-list__td--swatch {
    @apply pl-3 pr-1;
    width: 1.25rem;
  }

  .dp-list__swatch {
    @apply inline-block rounded-full;
    width: 6px;
    height: 6px;
  }

  .dp-list__td--content {
    @apply w-full pl-1 pr-4;
  }

  .dp-list__row-main {
    @apply flex w-full min-w-0 items-baseline justify-between gap-3;
  }

  .dp-list__trail {
    @apply flex min-w-0 shrink items-baseline justify-end gap-2;
  }

  .dp-list__value-group {
    @apply inline-flex min-w-0 items-baseline justify-end gap-1;
  }

  .dp-list__time {
    @apply shrink-0;
    color: var(--color-subtle);
  }

  .dp-list__value {
    @apply min-w-0 truncate font-mono;
    color: var(--color-base-content);
  }

  .dp-list__unit {
    @apply shrink-0;
    color: var(--color-subtle);
  }

  .dp-list__expansion-row {
    @apply bg-base-200/50;
  }

  .dp-list__expansion-cell {
    @apply p-0;
  }

  .dp-list__expansion {
    @apply flex flex-col gap-2 px-4 py-2;
    border-bottom: 1px solid
      color-mix(in oklab, var(--color-base-300) 30%, transparent);
  }

  .dp-list__detail {
    @apply flex items-baseline gap-2 text-xs;
  }

  .dp-list__detail-label {
    @apply shrink-0 text-xs font-medium;
    color: var(--color-subtle);
  }

  .dp-list__detail-value {
    @apply font-mono text-xs;
    color: var(--color-base-content);
  }

  .dp-list__exemplar {
    @apply flex flex-col gap-0.5;
  }

  .dp-list__exemplar-fields {
    @apply flex flex-col gap-0.5 pl-3;
  }
</style>
