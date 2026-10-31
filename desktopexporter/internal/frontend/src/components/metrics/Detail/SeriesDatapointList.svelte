<script module lang="ts">
  import type { DataPoint, MetricType } from '@/types/api-types'

  export type DatapointOwner = {
    metricRef: string
    seriesRef: string
    unit: string
    metricType: MetricType
  }

  export type DatapointSeries = DatapointOwner & {
    datapoints: DataPoint[]
    count: number
  }
</script>

<script lang="ts">
  import ExemplarTraceReference from './ExemplarTraceReference.svelte'
  import DatapointFields from './DatapointFields.svelte'
  import { detailPanel } from '@/state/detail-panel.svelte'
  /*
   * Paginated datapoint rows for the selected timeseries. Selection and
   * histogram snapshot sync go through MetricViewContext.
   */
  import { tick } from 'svelte'
  import { formatDateTimeMs } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { formatMetricValuePlain } from '@/components/metrics/utils/format-metric-value'
  import { dedupeAttributes } from '@/components/metrics/utils/dedupe-attributes'
  import { attributeValueLabel } from '@/components/shared/attribute-label'
  import DetailNav from '@/components/shared/DetailNav.svelte'
  import type { Exemplar } from '@/types/api-types'

  type Props = {
    datapoints: DataPoint[]
    owner?: DatapointOwner
    /** The displayed metric still owns the requested page selection. */
    selectionCurrent?: boolean
  }

  let { datapoints, owner, selectionCurrent = true }: Props = $props()

  const ctx = getMetricViewContext()
  const timeContext = getTimeContext()

  const ownsSelection = $derived(
    selectionCurrent &&
      (!owner ||
        (owner.metricRef === ctx.metric?.metricRef &&
          owner.seriesRef === ctx.selectedSeriesKey &&
          ctx.seriesDatapoints(owner.seriesRef) === datapoints))
  )
  let selectedID = $state<string | null>(null)
  let relatedIDs = $state<ReadonlySet<string>>(new Set())
  let revealedID = $state<string | null>(null)
  $effect.pre(() => {
    if (!ownsSelection) return
    const selected = ctx.selectedDatapointID
    const related = new Set(ctx.chartRelatedDatapointIDs)
    selectedID = selected
    relatedIDs = related
    revealedID =
      ctx.selectionSource === 'chart'
        ? (related.values().next().value ?? null)
        : selected
  })
  function selectDatapoint(dp: DataPoint) {
    if (ownsSelection) {
      ctx.onDatapointClick(dp)
    } else {
      // Inspection belongs to the displayed rows even when the shared chart
      // and URL already belong to a replacement. Keep those actions local.
      selectedID = selectedID === dp.id ? null : dp.id
      revealedID = selectedID
    }
  }
  const isHistogramKind = $derived(
    owner
      ? owner.metricType === 'Histogram' ||
          owner.metricType === 'ExponentialHistogram'
      : ctx.isHistogramKind
  )
  let metricUnit = $derived(owner?.unit ?? ctx.metric?.unit ?? '')
  let valueUnit = $derived(displayUnit(metricUnit))
  let hasDetails = $derived(
    datapoints.some(dp => dp.flags > 0 || dp.exemplars.length > 0)
  )
  let columnCount = $derived((isHistogramKind ? 3 : 2) + (hasDetails ? 1 : 0))

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
  const revealDatapointID = $derived(revealedID)

  $effect(() => {
    const id = revealDatapointID
    if (!id || !detailPanel.open) return
    let cancelled = false
    void tick().then(() => {
      if (cancelled || listRoot?.closest('details')?.open === false) return
      listRoot
        ?.querySelector(`[data-dp-id="${CSS.escape(id)}"]`)
        ?.scrollIntoView?.({ block: 'nearest' })
    })
    return () => {
      cancelled = true
    }
  })

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

  function focusedDatapointRow(): {
    element: HTMLTableRowElement
    id: string
    index: number
  } | null {
    const active = document.activeElement
    if (
      !(active instanceof HTMLTableRowElement) ||
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

  async function restoreNearestDatapointFocus(
    preferredIndex: number,
    previous: HTMLTableRowElement
  ): Promise<void> {
    await tick()
    const active = document.activeElement
    // A pointer or keyboard action that focused something else after the data
    // change wins. Body means the focused row was removed from the DOM.
    if (active !== previous && active !== document.body) return

    let nearest: HTMLTableRowElement | undefined
    let nearestDistance = Number.POSITIVE_INFINITY
    for (const row of listRoot?.querySelectorAll<HTMLTableRowElement>(
      'tr[data-dp-index]'
    ) ?? []) {
      const distance = Math.abs(Number(row.dataset.dpIndex) - preferredIndex)
      if (distance < nearestDistance) {
        nearest = row
        nearestDistance = distance
      }
    }
    nearest?.focus()
  }

  // Reconcile before Svelte removes the old page so a focused datapoint row
  // can be identified. External focus is never moved; keyboard traversal owns
  // its own explicit target below.
  $effect.pre(() => {
    const points = datapoints
    const size = pageSize
    const selectedID = revealDatapointID
    const focused = focusedDatapointRow()
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
        void restoreNearestDatapointFocus(preferred, focused.element)
      }
    }

    if (nextPage !== pageIndex) pageIndex = nextPage
  })

  function setPage(next: number): void {
    pageIndex = Math.max(0, Math.min(next, pageCount - 1))
  }

  function changePageSize(
    e: Event & { currentTarget: HTMLSelectElement }
  ): void {
    const next = Number(e.currentTarget.value)
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
      ?.querySelector<HTMLTableRowElement>(`tr[data-dp-index="${target}"]`)
      ?.focus()
    if (keyboardFocusTarget === target) keyboardFocusTarget = null
  }

  function moveDatapointFocus(e: KeyboardEvent, index: number): void {
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

  function handleDatapointKeydown(
    e: KeyboardEvent,
    dp: DataPoint,
    index: number
  ): void {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      selectDatapoint(dp)
      return
    }
    moveDatapointFocus(e, index)
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

  function exemplarValue(ex: Exemplar): number | bigint | string {
    switch (ex.valueType) {
      case 'Double':
        return ex.doubleValue
      case 'Int':
        return ex.intValue
      case 'Empty':
        return '—'
    }
  }

  function datapointValueParts(dp: DataPoint) {
    const unit = displayUnit(metricUnit)
    if (dp.metricType === 'Gauge' || dp.metricType === 'Sum') {
      if (dp.doubleValue !== null) {
        return {
          number: formatMetricValuePlain(dp.doubleValue),
          unit,
          count: null,
        }
      }
      if (dp.intValue !== null) {
        return { number: dp.intValue.toString(), unit, count: null }
      }
      return { number: '—', unit: null, count: null }
    }
    if (
      dp.metricType === 'Histogram' ||
      dp.metricType === 'ExponentialHistogram'
    ) {
      return {
        number: dp.sum === null ? '—' : formatMetricValuePlain(dp.sum),
        unit,
        count: dp.count.toString(),
      }
    }
    return { number: '—', unit: null, count: null }
  }

  function datapointRowLabel(
    datapointID: string,
    formattedTime: string,
    valueParts: { number: string; unit: string | null; count: string | null }
  ): string {
    const number = valueParts.unit
      ? `${valueParts.number} ${valueParts.unit}`
      : valueParts.number
    const value =
      valueParts.count === null
        ? number
        : `count ${valueParts.count}, sum ${number}`
    return `Datapoint at ${formattedTime}, value ${value}, ID ${datapointID}`
  }
</script>

{#snippet datapointRows(dp: DataPoint, datapointIndex: number)}
  {@const selected = selectedID === dp.id}
  {@const expanded = selected}
  {@const chartRelated = relatedIDs.has(dp.id)}
  {@const valueParts = datapointValueParts(dp)}
  {@const formattedTime = formatDatapointTime(dp.timestamp)}
  <tr
    class="dp-list__row item-state"
    data-dp-id={dp.id}
    data-dp-index={datapointIndex}
    tabindex="0"
    aria-label={datapointRowLabel(dp.id, formattedTime, valueParts)}
    aria-selected={selected || chartRelated}
    aria-expanded={expanded}
    title={chartRelated ? 'In selected chart point or interval' : undefined}
    onclick={() => {
      if (!window.getSelection()?.toString()) selectDatapoint(dp)
    }}
    onkeydown={e => handleDatapointKeydown(e, dp, datapointIndex)}
  >
    <td class="dp-list__td dp-list__td--time tabular-nums">
      {formattedTime}
    </td>
    {#if isHistogramKind}
      <td class="dp-list__td dp-list__td--value">{valueParts.count}</td>
    {/if}
    <td class="dp-list__td dp-list__td--value">{valueParts.number}</td>
    {#if hasDetails}
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
          <span class="text-base-content">flags</span>
        {/if}
      </td>
    {/if}
  </tr>
  {#if expanded}
    <tr class="dp-list__expansion-row">
      <td colspan={columnCount} class="dp-list__expansion-cell">
        <div class="dp-list__expansion">
          <DatapointFields datapoint={dp} />
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
                <span class="dp-list__detail-value"
                  >value: {exemplarValue(ex)}</span
                >
                <span class="dp-list__detail-value tabular-nums">
                  time: {formatDatapointTime(ex.timestamp)}
                </span>
                <ExemplarTraceReference exemplar={ex} />
                {#each dedupeAttributes(ex.filteredAttributes) as attr, attributeIndex (`exemplar:${i}:${attr.id ?? 'unidentified'}:${attributeIndex}`)}
                  <span class="dp-list__detail-value"
                    >{attr.key}: {attributeValueLabel(attr.value)}</span
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
  <p class="dp-list__empty">No datapoints</p>
{:else}
  <div bind:this={listRoot} class="dp-list__container">
    <div class="dp-list__table-wrap">
      <table class="dp-list" aria-label="Datapoints">
        <tbody>
          {#if datapoints.length > 0}
            <tr class="dp-list__header-row">
              <th scope="col" class="dp-list__th dp-list__th--time"> Time </th>
              {#if isHistogramKind}
                <th scope="col" class="dp-list__th dp-list__th--value">Count</th
                >
              {/if}
              <th scope="col" class="dp-list__th dp-list__th--value">
                {isHistogramKind ? 'Sum' : 'Value'}{#if valueUnit}
                  <span class="dp-list__header-unit">{' '}({valueUnit})</span>
                {/if}
              </th>
              {#if hasDetails}
                <th scope="col" class="dp-list__th dp-list__th--details">
                  Details
                </th>
              {/if}
            </tr>
            {#each visible as dp, offset (dp.id)}
              {@render datapointRows(dp, rangeStart + offset)}
            {/each}
          {:else}
            <tr
              ><td colspan={columnCount} class="dp-list__empty"
                >No datapoints</td
              ></tr
            >
          {/if}
        </tbody>
      </table>
    </div>

    {#if datapoints.length >= PAGE_SIZES[0]}<div class="dp-list__pagination">
        <span
          class="dp-list__range tabular-nums"
          aria-live="polite"
          aria-atomic="true"
        >
          {rangeStart + 1}-{rangeEnd} of {datapoints.length}
          {datapoints.length === 1 ? 'datapoint' : 'datapoints'}
        </span>
        <div class="dp-list__page-nav">
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
        <label class="dp-list__page-size">
          <span>Rows per page</span>
          <select
            class="select page-size-select"
            aria-label="Rows per page"
            value={pageSize}
            onchange={changePageSize}
          >
            {#each PAGE_SIZES as size}
              <option value={size}>{size}</option>
            {/each}
          </select>
        </label>
      </div>{/if}
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .dp-list__empty {
    @apply m-0 px-3 py-2 text-center text-xs italic;
    color: var(--color-muted);
  }

  .dp-list__container {
    @apply flex min-h-0 min-w-0 flex-1 flex-col;
    container-type: inline-size;
  }

  .dp-list__table-wrap {
    @apply min-h-0 min-w-0 flex-1 overflow-auto;
  }

  .dp-list {
    @apply w-full text-xs;
  }

  .dp-list__header-row > .dp-list__th {
    border-bottom: 1px solid
      color-mix(in oklab, var(--color-base-300) 50%, transparent);
  }

  .dp-list__th {
    @apply px-2 py-1 text-left text-sm font-normal;
    color: var(--color-subtle);
  }

  .dp-list__th--value {
    @apply text-right;
  }

  .dp-list__row {
    @apply cursor-pointer;
  }

  .dp-list__td {
    @apply px-2 py-1 align-middle;
  }

  .dp-list__th:last-child,
  .dp-list__td:last-child {
    padding-inline-end: calc(var(--pane-inline-inset) + var(--header-gap));
  }

  .dp-list__td--time {
    @apply whitespace-nowrap pl-3;
    color: var(--color-subtle);
  }

  .dp-list__td--value {
    @apply whitespace-nowrap text-right font-sans tabular-nums;
    color: var(--color-base-content);
  }

  .dp-list__td--details {
    @apply whitespace-nowrap;
  }

  .dp-list__header-unit {
    @apply font-normal;
    color: var(--color-subtle);
  }

  @media (forced-colors: active) {
    .dp-list__row:focus-visible {
      outline: 2px solid Highlight;
      outline-offset: -2px;
      box-shadow: none;
    }
  }

  .dp-list__pagination {
    @apply grid shrink-0 items-center gap-2 bg-base-100 px-3 py-2;
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
    border-top: 1px solid
      color-mix(in oklab, var(--color-base-300) 30%, transparent);
  }

  .dp-list__page-size {
    @apply flex flex-wrap items-center gap-2;
  }

  .dp-list__page-nav {
    @apply justify-self-center;
  }

  .dp-list__page-size {
    @apply justify-self-end;
  }

  .dp-list__range,
  .dp-list__page-size {
    @apply text-xs;
    color: var(--color-subtle);
  }

  .dp-list__page-size :global(.select) {
    @apply w-auto min-w-16;
  }

  @container (max-width: 30rem) {
    .dp-list__pagination {
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .dp-list__range {
      grid-row: 1;
      grid-column: 1;
    }

    .dp-list__page-nav {
      grid-row: 2;
      grid-column: 1 / -1;
    }

    .dp-list__page-size {
      grid-row: 1;
      grid-column: 2;
    }
  }

  @container (max-width: 20rem) {
    .dp-list__pagination {
      grid-template-columns: minmax(0, 1fr);
    }

    .dp-list__range,
    .dp-list__page-nav,
    .dp-list__page-size {
      grid-column: 1;
    }

    .dp-list__range {
      grid-row: 1;
    }

    .dp-list__page-nav {
      grid-row: 2;
    }

    .dp-list__page-size {
      grid-row: 3;
    }
  }

  .dp-list__expansion-cell {
    @apply p-0;
  }

  .dp-list__expansion {
    @apply flex flex-col;
  }

  .dp-list__detail {
    @apply flex items-baseline gap-2 text-xs;
  }

  .dp-list__detail-label {
    @apply shrink-0 text-sm font-normal;
    color: var(--color-subtle);
  }

  .dp-list__detail-value {
    @apply font-sans text-xs;
    color: var(--color-base-content);
  }

  .dp-list__exemplar {
    @apply flex flex-col gap-0.5 px-3 py-2;
  }

  .dp-list__exemplar-fields {
    @apply flex flex-col gap-0.5 pl-3;
  }
</style>
