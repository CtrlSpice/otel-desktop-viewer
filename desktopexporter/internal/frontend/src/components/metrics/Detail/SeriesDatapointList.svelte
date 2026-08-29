<script lang="ts">
  /*
   * Paginated datapoint rows for a single timeseries. Nested under an expanded
   * series row in TimeseriesPanel. Selection, flags/exemplar expansion, and
   * histogram snapshot sync go through MetricViewContext.
   */
  import { tick } from 'svelte'
  import { formatDateTimeMs } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { formatMetricValuePlain } from '@/components/metrics/utils/format-metric-value'
  import { dedupeAttributes } from '@/components/metrics/utils/dedupe-attributes'
  import DetailNav from '@/components/shared/DetailNav.svelte'
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

  let {
    datapoints,
    showSwatch = false,
    seriesColor,
    flush = false,
  }: Props = $props()

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  let metricUnit = $derived(ctx.metric?.unit ?? '')

  const PAGE_SIZES = [25, 50, 100] as const
  type PageSize = (typeof PAGE_SIZES)[number]

  let pageSize = $state<PageSize>(25)
  let pageIndex = $state(0)
  let listRoot = $state<HTMLDivElement>()
  let keyboardFocusTarget: number | null = null

  let pageCount = $derived(Math.max(1, Math.ceil(datapoints.length / pageSize)))
  let rangeStart = $derived(pageIndex * pageSize)
  let rangeEnd = $derived(Math.min(datapoints.length, rangeStart + pageSize))
  let visible = $derived(datapoints.slice(rangeStart, rangeEnd))
  let indexedDatapoints: DataPoint[] | undefined
  let cachedDatapointIndexByID = new Map<string, number>()
  let revealedDatapoints: DataPoint[] | undefined
  let revealedSelectedID: string | null | undefined

  function datapointIndexByID(points: DataPoint[]): Map<string, number> {
    if (points === indexedDatapoints) return cachedDatapointIndexByID

    const indexByID = new Map<string, number>()
    for (let index = 0; index < points.length; index++) {
      indexByID.set(points[index]!.id, index)
    }
    indexedDatapoints = points
    cachedDatapointIndexByID = indexByID
    return cachedDatapointIndexByID
  }

  function focusedInspectButton(): {
    element: HTMLButtonElement
    id: string
    index: number
  } | null {
    const active = document.activeElement
    if (
      !(active instanceof HTMLButtonElement) ||
      !listRoot?.contains(active) ||
      active.dataset.dpId === undefined ||
      active.dataset.dpIndex === undefined
    ) {
      return null
    }
    return {
      element: active,
      id: active.dataset.dpId,
      index: Number(active.dataset.dpIndex),
    }
  }

  async function restoreNearestInspectFocus(
    preferredIndex: number,
    previous: HTMLButtonElement
  ): Promise<void> {
    await tick()
    const active = document.activeElement
    // A pointer or keyboard action that focused something else after the data
    // change wins. Body means the focused button was removed from the DOM.
    if (active !== previous && active !== document.body) return

    let nearest: HTMLButtonElement | undefined
    let nearestDistance = Number.POSITIVE_INFINITY
    for (const button of listRoot?.querySelectorAll<HTMLButtonElement>(
      'button[data-dp-index]'
    ) ?? []) {
      const distance = Math.abs(Number(button.dataset.dpIndex) - preferredIndex)
      if (distance < nearestDistance) {
        nearest = button
        nearestDistance = distance
      }
    }
    nearest?.focus()
  }

  // Reconcile before Svelte removes the old page so a focused Inspect button
  // can be identified. External focus is never moved; keyboard traversal owns
  // its own explicit target below.
  $effect.pre(() => {
    const points = datapoints
    const size = pageSize
    const selectedID = ctx.selectedDatapointID
    const focused = focusedInspectButton()
    const indexByID = datapointIndexByID(points)
    const selectedIndex = selectedID ? (indexByID.get(selectedID) ?? -1) : -1
    const focusedIndex = focused ? (indexByID.get(focused.id) ?? -1) : -1
    // Reveal is edge-triggered. Once handled, pagination belongs to the user
    // until either the selected identity or the datapoints array changes.
    const revealSelection =
      points !== revealedDatapoints || selectedID !== revealedSelectedID
    revealedDatapoints = points
    revealedSelectedID = selectedID

    const lastPage = Math.max(0, pageCount - 1)
    let nextPage = Math.min(pageIndex, lastPage)
    if (revealSelection && selectedIndex >= 0) {
      nextPage = Math.floor(selectedIndex / size)
    }

    if (focused && keyboardFocusTarget === null) {
      const start = nextPage * size
      const end = Math.min(points.length, start + size)
      const remainsMounted = focusedIndex >= start && focusedIndex < end
      if (!remainsMounted && end > start) {
        const preferred =
          selectedIndex >= 0
            ? selectedIndex
            : Math.max(
                start,
                Math.min(
                  focusedIndex >= 0 ? focusedIndex : focused.index,
                  end - 1
                )
              )
        void restoreNearestInspectFocus(preferred, focused.element)
      }
    }

    if (nextPage !== pageIndex) pageIndex = nextPage
  })

  function setPage(next: number): void {
    pageIndex = Math.max(0, Math.min(next, pageCount - 1))
  }

  function changePageSize(e: Event): void {
    const next = Number((e.currentTarget as HTMLSelectElement).value)
    if (next !== 25 && next !== 50 && next !== 100) return
    pageIndex = 0
    pageSize = next
  }

  async function focusDatapointAt(index: number): Promise<void> {
    const target = Math.max(0, Math.min(index, datapoints.length - 1))
    keyboardFocusTarget = target
    pageIndex = Math.floor(target / pageSize)
    await tick()
    listRoot
      ?.querySelector<HTMLButtonElement>(`button[data-dp-index="${target}"]`)
      ?.focus()
    if (keyboardFocusTarget === target) keyboardFocusTarget = null
  }

  function moveInspectFocus(e: KeyboardEvent, index: number): void {
    let next: number | null = null
    switch (e.key) {
      case 'ArrowDown':
      case 'j':
        next = index + 1
        break
      case 'ArrowUp':
      case 'k':
        next = index - 1
        break
      case 'PageDown':
        next = index + pageSize
        break
      case 'PageUp':
        next = index - pageSize
        break
      case 'Home':
        next = 0
        break
      case 'End':
        next = datapoints.length - 1
        break
    }
    if (next === null) return

    e.preventDefault()
    const clamped = Math.max(0, Math.min(next, datapoints.length - 1))
    if (clamped !== index) void focusDatapointAt(clamped)
  }

  function displayUnit(unit: string): string | null {
    const u = unit.trim()
    if (!u || u === '1') return null
    return u
  }

  function formatDatapointTime(timestamp: bigint): string {
    return formatDateTimeMs(Number(timestamp / 1_000_000n), timeContext.tz)
      .dateTime
  }

  /** Whether the store trimmed this datapoint's exemplar list. The field is
   *  absent whenever it did not, which is almost always. */
  function withheld(dp: DataPoint): boolean {
    return (
      dp.exemplarCount !== undefined && dp.exemplarCount > dp.exemplars.length
    )
  }

  function exemplarSpanPatch(ex: Exemplar) {
    return ex.spanID ? { [SPAN_PARAM]: ex.spanID } : undefined
  }

  function goToExemplarTrace(e: MouseEvent, ex: Exemplar) {
    if (!ex.traceID) return
    e.preventDefault()
    navigateToItem('traces', ex.traceID, 'push', exemplarSpanPatch(ex))
  }

  function datapointValueParts(dp: DataPoint): {
    number: string
    unit: string | null
  } {
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

  function inspectButtonLabel(
    datapointID: string,
    formattedTime: string,
    valueParts: { number: string; unit: string | null }
  ): string {
    const value = valueParts.unit
      ? `${valueParts.number} ${valueParts.unit}`
      : valueParts.number
    return `Inspect datapoint at ${formattedTime}, value ${value}, ID ${datapointID}`
  }
</script>

{#snippet datapointRows(dp: DataPoint, datapointIndex: number)}
  {@const selected = ctx.selectedDatapointID === dp.id}
  {@const hasExtra = dp.flags > 0 || dp.exemplars.length > 0}
  {@const expanded = hasExtra && ctx.expandedDatapoints.has(dp.id)}
  {@const valueParts = datapointValueParts(dp)}
  {@const formattedTime = formatDatapointTime(dp.timestamp)}
  <tr
    class="dp-list__row"
    class:dp-list__row--selected={selected}
    data-dp-id={dp.id}
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
    <td class="dp-list__td dp-list__td--time tabular-nums">
      {formattedTime}
    </td>
    <td class="dp-list__td dp-list__td--value">
      <span class="dp-list__value-group">
        <span class="dp-list__value tabular-nums">{valueParts.number}</span>
        {#if valueParts.unit}
          <span class="dp-list__unit">{valueParts.unit}</span>
        {/if}
      </span>
    </td>
    <td class="dp-list__td dp-list__td--details">
      {#if dp.exemplars.length > 0}
        <span class="badge-count">
          {#if withheld(dp)}
            {dp.exemplars.length} of {dp.exemplarCount} ex
          {:else}
            {dp.exemplars.length} ex
          {/if}
        </span>
      {/if}
      {#if dp.flags > 0}
        <span class="badge badge-xs badge-soft badge-warning">flags</span>
      {/if}
    </td>
    <td class="dp-list__td dp-list__td--action">
      <button
        type="button"
        class="btn btn-ghost btn-xs"
        data-dp-index={datapointIndex}
        data-dp-id={dp.id}
        aria-label={inspectButtonLabel(dp.id, formattedTime, valueParts)}
        aria-pressed={selected}
        aria-expanded={hasExtra ? expanded : undefined}
        onclick={() => ctx.onDatapointClick(dp)}
        onkeydown={e => moveInspectFocus(e, datapointIndex)}
      >
        Inspect
      </button>
    </td>
  </tr>
  {#if expanded}
    <tr class="dp-list__expansion-row">
      <td colspan={showSwatch ? 5 : 4} class="dp-list__expansion-cell">
        <div class="dp-list__expansion">
          {#if dp.flags > 0}
            <div class="dp-list__detail">
              <span class="dp-list__detail-label">flags</span>
              <span class="dp-list__detail-value">{dp.flags}</span>
            </div>
          {/if}
          {#if withheld(dp)}
            <div class="dp-list__detail">
              <span class="dp-list__detail-label">exemplars</span>
              <span class="dp-list__detail-value">
                showing {dp.exemplars.length} of {dp.exemplarCount} — the store caps
                the list so one densely sampled stream cannot decide the size of every
                response
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
                    >trace: {ex.traceID}</a
                  >
                {/if}
                {#if ex.spanID && ex.traceID}
                  <a
                    class="dp-list__detail-value link link-primary font-mono"
                    href={itemHref('traces', ex.traceID, exemplarSpanPatch(ex))}
                    onclick={e => goToExemplarTrace(e, ex)}>span: {ex.spanID}</a
                  >
                {:else if ex.spanID}
                  <span class="dp-list__detail-value">span: {ex.spanID}</span>
                {/if}
                {#each dedupeAttributes(ex.filteredAttributes) as attr (attr.key)}
                  <span class="dp-list__detail-value"
                    >{attr.key}: {attr.value}</span
                  >
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
  <p class="dp-list__empty" class:dp-list__empty--flush={flush}>
    No datapoints
  </p>
{:else}
  <div
    bind:this={listRoot}
    class="dp-list__container"
    class:dp-list__container--flush={flush}
  >
    <div class="dp-list__table-wrap">
      <table
        class="dp-list"
        class:dp-list--flush={flush}
        aria-label="Datapoints"
      >
        <thead>
          <tr class="dp-list__header-row">
            {#if showSwatch}
              <th scope="col" class="dp-list__th dp-list__th--swatch">
                <span class="sr-only">Series</span>
              </th>
            {/if}
            <th scope="col" class="dp-list__th dp-list__th--time"> Time </th>
            <th scope="col" class="dp-list__th dp-list__th--value"> Value </th>
            <th scope="col" class="dp-list__th dp-list__th--details">
              Details
            </th>
            <th scope="col" class="dp-list__th dp-list__th--action">
              Action
            </th>
          </tr>
        </thead>
        <tbody>
          {#each visible as dp, offset (dp.id)}
            {@render datapointRows(dp, rangeStart + offset)}
          {/each}
        </tbody>
      </table>
    </div>

    <div class="dp-list__pagination">
      <div class="dp-list__pagination-summary">
        <span
          class="dp-list__range tabular-nums"
          aria-live="polite"
          aria-atomic="true"
        >
          {rangeStart + 1}-{rangeEnd} of {datapoints.length}
          {datapoints.length === 1 ? 'datapoint' : 'datapoints'}
        </span>
        <label class="dp-list__page-size">
          <span>Rows per page</span>
          <select
            class="select select-xs"
            aria-label="Rows per page"
            value={pageSize}
            onchange={changePageSize}
          >
            {#each PAGE_SIZES as size}
              <option value={size}>{size}</option>
            {/each}
          </select>
        </label>
      </div>
      <DetailNav
        index={pageIndex}
        total={pageCount}
        label="page"
        onFirst={() => setPage(0)}
        onPrev={() => setPage(pageIndex - 1)}
        onNext={() => setPage(pageIndex + 1)}
        onLast={() => setPage(pageCount - 1)}
      />
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .dp-list__empty--flush {
    @apply px-0;
  }

  .dp-list--flush .dp-list__td--swatch,
  .dp-list--flush .dp-list__th--swatch {
    @apply pl-0;
  }

  .dp-list--flush .dp-list__td--time,
  .dp-list--flush .dp-list__th--time {
    @apply pl-0;
  }

  .dp-list__empty {
    @apply m-0 px-3 py-2 text-center text-xs italic;
    color: var(--color-muted);
  }

  .dp-list__table-wrap {
    @apply overflow-x-auto;
  }

  .dp-list {
    @apply w-full text-xs;
    border-collapse: collapse;
  }

  .dp-list__header-row {
    border-bottom: 1px solid
      color-mix(in oklab, var(--color-base-300) 50%, transparent);
  }

  .dp-list__th {
    @apply px-2 py-1 text-left text-xs font-medium;
    color: var(--color-subtle);
  }

  .dp-list__th--value,
  .dp-list__th--action {
    @apply text-right;
  }

  .dp-list__row {
    @apply transition-colors hover:bg-base-300/30;
  }

  .dp-list__row--selected {
    background-color: color-mix(
      in oklab,
      var(--color-primary) 18%,
      transparent
    );
  }

  .dp-list__td {
    @apply px-2 py-1 align-middle;
  }

  .dp-list__td--swatch,
  .dp-list__th--swatch {
    @apply pl-3 pr-1;
    width: 1.25rem;
  }

  .dp-list__swatch {
    @apply inline-block rounded-full;
    width: 6px;
    height: 6px;
  }

  .dp-list__td--time {
    @apply whitespace-nowrap pl-3;
    color: var(--color-subtle);
  }

  .dp-list__value-group {
    @apply inline-flex min-w-0 items-baseline justify-end gap-1;
  }

  .dp-list__td--value,
  .dp-list__td--action {
    @apply text-right;
  }

  .dp-list__td--details {
    @apply whitespace-nowrap;
  }

  .dp-list__value {
    @apply font-mono;
    color: var(--color-base-content);
  }

  .dp-list__unit {
    @apply shrink-0;
    color: var(--color-subtle);
  }

  .dp-list__pagination {
    @apply flex flex-wrap items-center justify-between gap-2 px-3 py-2;
    border-top: 1px solid
      color-mix(in oklab, var(--color-base-300) 30%, transparent);
  }

  .dp-list__container--flush .dp-list__pagination {
    @apply px-0;
  }

  .dp-list__pagination-summary,
  .dp-list__page-size {
    @apply flex flex-wrap items-center gap-2;
  }

  .dp-list__range,
  .dp-list__page-size {
    @apply text-xs;
    color: var(--color-subtle);
  }

  .dp-list__page-size :global(.select) {
    @apply w-auto min-w-16;
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
